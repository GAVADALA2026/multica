package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTerminalReportStoreRoundTripAndPermissions(t *testing.T) {
	store := newTerminalReportStore(Config{
		WorkspacesRoot: t.TempDir(),
		ServerBaseURL:  "https://api.example.test",
		Profile:        "work",
		DaemonID:       "daemon-1",
	})
	report := terminalTaskReport{
		kind:                  terminalTaskReportComplete,
		taskID:                "task-private",
		output:                "private final answer",
		branchName:            "agent/private",
		sessionID:             "session-private",
		workDir:               "/private/workdir",
		durableWorkDir:        "/private/project",
		sessionRolloutMissing: true,
		retiredSessionID:      "retired-private",
	}
	if err := store.enqueue(report); err != nil {
		t.Fatalf("enqueue terminal report: %v", err)
	}
	items, err := store.list()
	if err != nil {
		t.Fatalf("list terminal reports: %v", err)
	}
	if len(items) != 1 || items[0].report != report {
		t.Fatalf("round trip = %+v, want %+v", items, report)
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(store.dir); err != nil {
			t.Fatalf("stat queue directory: %v", err)
		} else if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("queue directory mode = %o, want 700", got)
		}
		path := store.dir + string(os.PathSeparator) + items[0].fileName
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("stat queue file: %v", err)
		} else if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("queue file mode = %o, want 600", got)
		}
	}

	conflict := report
	conflict.output = "replacement must not overwrite the original"
	if err := store.enqueue(conflict); err == nil || !strings.Contains(err.Error(), "conflicts with the original") {
		t.Fatalf("conflicting enqueue error = %v, want original-payload conflict", err)
	}
	items, err = store.list()
	if err != nil {
		t.Fatalf("list after conflict: %v", err)
	}
	if len(items) != 1 || items[0].report.output != report.output {
		t.Fatalf("conflicting enqueue changed original payload: %+v", items)
	}
}

func TestTerminalReportStoreRecoversFlushedTempFileAfterCrash(t *testing.T) {
	store := newTerminalReportStore(Config{
		WorkspacesRoot: t.TempDir(),
		ServerBaseURL:  "https://api.example.test",
		DaemonID:       "daemon-crash",
	})
	if err := store.ensureDir(); err != nil {
		t.Fatalf("prepare store: %v", err)
	}
	report := terminalTaskReport{kind: terminalTaskReportComplete, taskID: "task-crash", output: "durable answer"}
	record, err := persistedTerminalReport(report, time.Now())
	if err != nil {
		t.Fatalf("build persisted report: %v", err)
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	targetName := terminalReportFileName(report.taskID)
	temp, err := os.CreateTemp(store.dir, "."+strings.TrimSuffix(targetName, ".json")+"-*.tmp")
	if err != nil {
		t.Fatalf("create interrupted temp: %v", err)
	}
	tempName := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		t.Fatalf("chmod interrupted temp: %v", err)
	}
	if _, err := temp.Write(body); err != nil {
		t.Fatalf("write interrupted temp: %v", err)
	}
	if err := temp.Sync(); err != nil {
		t.Fatalf("sync interrupted temp: %v", err)
	}
	if err := temp.Close(); err != nil {
		t.Fatalf("close interrupted temp: %v", err)
	}

	items, err := store.list()
	if err != nil {
		t.Fatalf("recover interrupted report: %v", err)
	}
	if len(items) != 1 || items[0].report != report {
		t.Fatalf("recovered reports = %+v, want %+v", items, report)
	}
	if _, err := os.Stat(tempName); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interrupted temp still exists after recovery: %v", err)
	}
}

func TestTerminalReportReplaysAfterClientRetryWindow(t *testing.T) {
	defer noSleepRetry(t)()
	previousSchedule := defaultTerminalRetrySchedule
	defaultTerminalRetrySchedule = []time.Duration{time.Nanosecond, time.Nanosecond}
	t.Cleanup(func() { defaultTerminalRetrySchedule = previousSchedule })

	var online atomic.Bool
	var calls atomic.Int32
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/complete") {
			t.Errorf("unexpected request path %q", req.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		calls.Add(1)
		if !online.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := New(Config{
		ServerBaseURL:  srv.URL,
		WorkspacesRoot: t.TempDir(),
		DaemonID:       "daemon-retry",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	report := terminalTaskReport{
		kind:           terminalTaskReportComplete,
		taskID:         "task-retry-window",
		output:         "the original answer",
		branchName:     "agent/recovered",
		sessionID:      "session-1",
		workDir:        "/tmp/work",
		durableWorkDir: "/tmp/project",
	}
	if err := d.reportTerminalTask(context.Background(), report); err == nil {
		t.Fatal("terminal report unexpectedly succeeded while server was offline")
	}
	if got, want := calls.Load(), int32(len(defaultTerminalRetrySchedule)+1); got != want {
		t.Fatalf("initial callback attempts = %d, want exhausted window of %d", got, want)
	}
	if items, err := d.terminalReports.list(); err != nil || len(items) != 1 {
		t.Fatalf("pending reports after exhausted retries = %d, %v; want 1", len(items), err)
	}

	online.Store(true)
	pending, delivered := d.replayPendingTerminalReports(context.Background())
	if pending != 0 || delivered != 1 {
		t.Fatalf("replay result pending=%d delivered=%d, want 0/1", pending, delivered)
	}
	if got := calls.Load(); got != int32(len(defaultTerminalRetrySchedule)+2) {
		t.Fatalf("calls after recovery = %d, want one replay after retry exhaustion", got)
	}
	mu.Lock()
	defer mu.Unlock()
	for i, body := range bodies {
		if body["output"] != report.output || body["branch_name"] != report.branchName || body["durable_work_dir"] != report.durableWorkDir {
			t.Fatalf("attempt %d payload = %#v, want original report", i+1, body)
		}
	}
}

func TestTerminalReportReplaysAfterDaemonRestart(t *testing.T) {
	cfg := Config{
		ServerBaseURL:  "https://api.example.test",
		WorkspacesRoot: t.TempDir(),
		Profile:        "restart",
		DaemonID:       "daemon-restart",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	report := terminalTaskReport{
		kind:          terminalTaskReportFail,
		taskID:        "task-restart",
		errorMessage:  "provider failed after doing useful work",
		branchName:    "agent/partial-work",
		failureReason: "agent_error.process_failure",
	}

	beforeRestart := New(cfg, logger)
	beforeRestart.terminalReportSend = func(context.Context, terminalTaskReport, []time.Duration) error {
		return errors.New("network unavailable")
	}
	if err := beforeRestart.reportTerminalTask(context.Background(), report); err == nil {
		t.Fatal("terminal report unexpectedly succeeded before restart")
	}

	afterRestart := New(cfg, logger)
	var replayed terminalTaskReport
	afterRestart.terminalReportSend = func(_ context.Context, got terminalTaskReport, schedule []time.Duration) error {
		if schedule != nil {
			t.Fatalf("replay schedule = %v, want one HTTP attempt", schedule)
		}
		replayed = got
		return nil
	}
	pending, delivered := afterRestart.replayPendingTerminalReports(context.Background())
	if pending != 0 || delivered != 1 {
		t.Fatalf("restart replay pending=%d delivered=%d, want 0/1", pending, delivered)
	}
	if replayed != report {
		t.Fatalf("restart replay = %+v, want %+v", replayed, report)
	}
}

func TestRunBatchPollerReleasesSlotAfterTerminalRetryExhaustion(t *testing.T) {
	defer noSleepRetry(t)()
	previousSchedule := defaultTerminalRetrySchedule
	defaultTerminalRetrySchedule = []time.Duration{time.Nanosecond, time.Nanosecond}
	t.Cleanup(func() { defaultTerminalRetrySchedule = previousSchedule })

	var completeAttempts atomic.Int32
	var secondServed atomic.Bool
	secondStarted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(req.URL.Path, "/api/daemon/tasks/claim"):
			switch {
			case completeAttempts.Load() == 0:
				_, _ = w.Write([]byte(`{"tasks":[{"id":"t1","runtime_id":"rt-1","issue_id":"i1"}]}`))
			case secondServed.CompareAndSwap(false, true):
				_, _ = w.Write([]byte(`{"tasks":[{"id":"t2","runtime_id":"rt-1","issue_id":"i2"}]}`))
			default:
				_, _ = w.Write([]byte(`{"tasks":[]}`))
			}
		case strings.HasSuffix(req.URL.Path, "/tasks/t1/complete"):
			completeAttempts.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		case strings.HasSuffix(req.URL.Path, "/tasks/t2/complete"):
			w.WriteHeader(http.StatusOK)
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	d := New(Config{
		ServerBaseURL:      srv.URL,
		WorkspacesRoot:     t.TempDir(),
		DaemonID:           "daemon-slot",
		PollInterval:       time.Hour,
		MaxConcurrentTasks: 1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.workspaces["ws-1"] = &workspaceState{workspaceID: "ws-1", runtimeIDs: []string{"rt-1"}}
	d.runtimeIndex["rt-1"] = Runtime{ID: "rt-1"}
	d.cancelPollInterval = time.Hour
	d.taskSlotWait = 20 * time.Millisecond
	d.runner = taskRunnerFunc(func(_ context.Context, task Task, _ string, _ int, _ *slog.Logger) (TaskResult, error) {
		if task.ID == "t2" {
			close(secondStarted)
		}
		return TaskResult{Status: "completed"}, nil
	})

	sem := newTaskSlotSemaphore(1)
	wakeup := make(chan struct{}, 1)
	var taskWG sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		d.runBatchPoller(ctx, ctx, sem, wakeup, &taskWG)
	}()

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		cancel()
		<-pollDone
		t.Fatalf("second task did not start after first report exhausted retries; attempts=%d", completeAttempts.Load())
	}
	if got, want := completeAttempts.Load(), int32(len(defaultTerminalRetrySchedule)+1); got != want {
		t.Fatalf("first terminal callback attempts = %d, want %d", got, want)
	}
	cancel()
	<-pollDone
	taskWG.Wait()
	items, err := d.terminalReports.list()
	if err != nil {
		t.Fatalf("list pending reports: %v", err)
	}
	if len(items) != 1 || items[0].report.taskID != "t1" {
		t.Fatalf("pending reports = %+v, want only exhausted task t1", items)
	}
}
