package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	terminalReportRecordVersion = 1
	terminalReportReplayWorkers = 4

	terminalReportReplayInitialBackoff = 5 * time.Second
	terminalReportReplayMaxBackoff     = 5 * time.Minute
)

// persistedTerminalTaskReport is the versioned on-disk form of one terminal
// callback. It deliberately contains no auth token: replay always uses the
// daemon's current credential, while the potentially sensitive agent output is
// protected by the queue's owner-only directory and file modes.
type persistedTerminalTaskReport struct {
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"created_at"`
	Kind                  string    `json:"kind"`
	TaskID                string    `json:"task_id"`
	Output                string    `json:"output,omitempty"`
	BranchName            string    `json:"branch_name,omitempty"`
	ErrorMessage          string    `json:"error,omitempty"`
	SessionID             string    `json:"session_id,omitempty"`
	WorkDir               string    `json:"work_dir,omitempty"`
	DurableWorkDir        string    `json:"durable_work_dir,omitempty"`
	FailureReason         string    `json:"failure_reason,omitempty"`
	SessionRolloutMissing bool      `json:"session_rollout_missing,omitempty"`
	RetiredSessionID      string    `json:"retired_session_id,omitempty"`
}

type pendingTerminalTaskReport struct {
	fileName string
	report   terminalTaskReport
}

// terminalReportStore is a file-backed outbox. One file per task keeps each
// acknowledgement independent, and hashing task IDs prevents a malformed or
// tampered task ID from becoming a path traversal primitive.
type terminalReportStore struct {
	dir string
	mu  sync.Mutex
}

func newTerminalReportStore(cfg Config) *terminalReportStore {
	if strings.TrimSpace(cfg.WorkspacesRoot) == "" {
		return nil
	}
	identity := strings.TrimRight(cfg.ServerBaseURL, "/") + "\x00" + cfg.Profile + "\x00" + cfg.DaemonID
	sum := sha256.Sum256([]byte(identity))
	namespace := hex.EncodeToString(sum[:16])
	return &terminalReportStore{dir: filepath.Join(cfg.WorkspacesRoot, ".pending-terminal-reports", "v1", namespace)}
}

func terminalReportFileName(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return hex.EncodeToString(sum[:]) + ".json"
}

func terminalReportKindName(kind terminalTaskReportKind) (string, error) {
	switch kind {
	case terminalTaskReportComplete:
		return "complete", nil
	case terminalTaskReportFail:
		return "fail", nil
	default:
		return "", fmt.Errorf("unsupported terminal task report kind %d", kind)
	}
}

func persistedTerminalReport(report terminalTaskReport, createdAt time.Time) (persistedTerminalTaskReport, error) {
	if strings.TrimSpace(report.taskID) == "" {
		return persistedTerminalTaskReport{}, errors.New("terminal task report has no task id")
	}
	kind, err := terminalReportKindName(report.kind)
	if err != nil {
		return persistedTerminalTaskReport{}, err
	}
	return persistedTerminalTaskReport{
		Version:               terminalReportRecordVersion,
		CreatedAt:             createdAt.UTC(),
		Kind:                  kind,
		TaskID:                report.taskID,
		Output:                report.output,
		BranchName:            report.branchName,
		ErrorMessage:          report.errorMessage,
		SessionID:             report.sessionID,
		WorkDir:               report.workDir,
		DurableWorkDir:        report.durableWorkDir,
		FailureReason:         report.failureReason,
		SessionRolloutMissing: report.sessionRolloutMissing,
		RetiredSessionID:      report.retiredSessionID,
	}, nil
}

func (record persistedTerminalTaskReport) terminalReport() (terminalTaskReport, error) {
	if record.Version != terminalReportRecordVersion {
		return terminalTaskReport{}, fmt.Errorf("unsupported terminal report version %d", record.Version)
	}
	if strings.TrimSpace(record.TaskID) == "" {
		return terminalTaskReport{}, errors.New("terminal report has no task id")
	}
	var kind terminalTaskReportKind
	switch record.Kind {
	case "complete":
		kind = terminalTaskReportComplete
	case "fail":
		kind = terminalTaskReportFail
	default:
		return terminalTaskReport{}, fmt.Errorf("unsupported terminal report kind %q", record.Kind)
	}
	return terminalTaskReport{
		kind:                  kind,
		taskID:                record.TaskID,
		output:                record.Output,
		branchName:            record.BranchName,
		errorMessage:          record.ErrorMessage,
		sessionID:             record.SessionID,
		workDir:               record.WorkDir,
		durableWorkDir:        record.DurableWorkDir,
		failureReason:         record.FailureReason,
		sessionRolloutMissing: record.SessionRolloutMissing,
		retiredSessionID:      record.RetiredSessionID,
	}, nil
}

func (s *terminalReportStore) ensureDir() error {
	if s == nil {
		return errors.New("terminal report store is not configured")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create terminal report queue: %w", err)
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return fmt.Errorf("inspect terminal report queue: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("terminal report queue is not a real directory: %s", s.dir)
	}
	// Tighten an existing directory as well as a newly-created one. Terminal
	// payloads may include private prompts, results, paths, and error details.
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("secure terminal report queue: %w", err)
	}
	return nil
}

func (s *terminalReportStore) enqueue(report terminalTaskReport) error {
	record, err := persistedTerminalReport(report, time.Now())
	if err != nil {
		return err
	}
	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode terminal report: %w", err)
	}
	body = append(body, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir(); err != nil {
		return err
	}
	name := terminalReportFileName(report.taskID)
	path := filepath.Join(s.dir, name)
	if existingBody, readErr := os.ReadFile(path); readErr == nil {
		existing, decodeErr := decodePersistedTerminalReport(existingBody)
		if decodeErr != nil {
			return fmt.Errorf("existing terminal report %s is unreadable: %w", name, decodeErr)
		}
		existingReport, decodeErr := existing.terminalReport()
		if decodeErr != nil {
			return fmt.Errorf("existing terminal report %s is invalid: %w", name, decodeErr)
		}
		if existingReport != report {
			return fmt.Errorf("terminal report for task %s conflicts with the original pending payload", report.taskID)
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read existing terminal report %s: %w", name, readErr)
	}

	tmp, err := os.CreateTemp(s.dir, "."+strings.TrimSuffix(name, ".json")+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create terminal report temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("secure terminal report temp file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		cleanup()
		return fmt.Errorf("write terminal report temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync terminal report temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close terminal report temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publish terminal report: %w", err)
	}
	if err := syncTerminalReportDir(s.dir); err != nil {
		return fmt.Errorf("sync terminal report queue after enqueue: %w", err)
	}
	return nil
}

func decodePersistedTerminalReport(body []byte) (persistedTerminalTaskReport, error) {
	var record persistedTerminalTaskReport
	if err := json.Unmarshal(body, &record); err != nil {
		return persistedTerminalTaskReport{}, err
	}
	return record, nil
}

func (s *terminalReportStore) list() ([]pendingTerminalTaskReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("read terminal report queue: %w", err)
	}
	recoveryErr := s.recoverTempFiles(entries)
	entries, err = os.ReadDir(s.dir)
	if err != nil {
		return nil, errors.Join(recoveryErr, fmt.Errorf("reread terminal report queue: %w", err))
	}
	items := make([]pendingTerminalTaskReport, 0, len(entries))
	var errs []error
	if recoveryErr != nil {
		errs = append(errs, recoveryErr)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if readErr != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", entry.Name(), readErr))
			continue
		}
		record, decodeErr := decodePersistedTerminalReport(body)
		if decodeErr != nil {
			errs = append(errs, fmt.Errorf("decode %s: %w", entry.Name(), decodeErr))
			continue
		}
		report, decodeErr := record.terminalReport()
		if decodeErr != nil {
			errs = append(errs, fmt.Errorf("validate %s: %w", entry.Name(), decodeErr))
			continue
		}
		if want := terminalReportFileName(report.taskID); entry.Name() != want {
			errs = append(errs, fmt.Errorf("terminal report %s does not match task id", entry.Name()))
			continue
		}
		items = append(items, pendingTerminalTaskReport{fileName: entry.Name(), report: report})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].fileName < items[j].fileName })
	return items, errors.Join(errs...)
}

// recoverTempFiles closes the atomic-write crash window after the temp file is
// fully flushed but before its rename. A partial/corrupt temp file is retained
// for forensic recovery and reported as an error; silently deleting it could
// discard the only copy of a terminal payload.
func (s *terminalReportStore) recoverTempFiles(entries []os.DirEntry) error {
	var errs []error
	changed := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		tempPath := filepath.Join(s.dir, name)
		body, err := os.ReadFile(tempPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("read interrupted terminal report %s: %w", name, err))
			continue
		}
		record, err := decodePersistedTerminalReport(body)
		if err != nil {
			errs = append(errs, fmt.Errorf("decode interrupted terminal report %s: %w", name, err))
			continue
		}
		report, err := record.terminalReport()
		if err != nil {
			errs = append(errs, fmt.Errorf("validate interrupted terminal report %s: %w", name, err))
			continue
		}
		targetName := terminalReportFileName(report.taskID)
		wantPrefix := "." + strings.TrimSuffix(targetName, ".json") + "-"
		if !strings.HasPrefix(name, wantPrefix) {
			errs = append(errs, fmt.Errorf("interrupted terminal report %s does not match task id", name))
			continue
		}
		targetPath := filepath.Join(s.dir, targetName)
		if existingBody, readErr := os.ReadFile(targetPath); readErr == nil {
			existing, decodeErr := decodePersistedTerminalReport(existingBody)
			if decodeErr != nil {
				errs = append(errs, fmt.Errorf("decode terminal report while recovering %s: %w", targetName, decodeErr))
				continue
			}
			existingReport, decodeErr := existing.terminalReport()
			if decodeErr != nil || existingReport != report {
				errs = append(errs, fmt.Errorf("interrupted terminal report %s conflicts with existing payload", name))
				continue
			}
			if err := os.Remove(tempPath); err != nil {
				errs = append(errs, fmt.Errorf("remove duplicate interrupted terminal report %s: %w", name, err))
				continue
			}
			changed = true
			continue
		} else if !errors.Is(readErr, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("inspect terminal report while recovering %s: %w", targetName, readErr))
			continue
		}
		if err := os.Chmod(tempPath, 0o600); err != nil {
			errs = append(errs, fmt.Errorf("secure interrupted terminal report %s: %w", name, err))
			continue
		}
		if err := os.Rename(tempPath, targetPath); err != nil {
			errs = append(errs, fmt.Errorf("recover interrupted terminal report %s: %w", name, err))
			continue
		}
		changed = true
	}
	if changed {
		if err := syncTerminalReportDir(s.dir); err != nil {
			errs = append(errs, fmt.Errorf("sync terminal report queue after recovery: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (s *terminalReportStore) acknowledge(item pendingTerminalTaskReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.fileName != terminalReportFileName(item.report.taskID) {
		return errors.New("terminal report acknowledgement does not match task id")
	}
	path := filepath.Join(s.dir, item.fileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove acknowledged terminal report: %w", err)
	}
	if err := syncTerminalReportDir(s.dir); err != nil {
		return fmt.Errorf("sync terminal report queue after acknowledgement: %w", err)
	}
	return nil
}

func (d *Daemon) signalTerminalReportReplay() {
	if d.terminalReportWakeup == nil {
		return
	}
	select {
	case d.terminalReportWakeup <- struct{}{}:
	default:
	}
}

// replayPendingTerminalReports makes one delivery attempt per queued report.
// A small fixed worker pool avoids one dead endpoint blocking every later task
// for the HTTP client's full timeout while still bounding reconnect pressure.
// The caller owns the outer backoff; each item gets exactly one HTTP attempt.
func (d *Daemon) replayPendingTerminalReports(ctx context.Context) (pending, delivered int) {
	if d.terminalReports == nil {
		return 0, 0
	}
	items, err := d.terminalReports.list()
	if err != nil {
		d.logger.Error("load pending terminal reports", "error", err)
	}
	if len(items) == 0 {
		return 0, 0
	}

	workers := terminalReportReplayWorkers
	if len(items) < workers {
		workers = len(items)
	}
	jobs := make(chan pendingTerminalTaskReport)
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	remaining := len(items)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if ctx.Err() != nil {
					continue
				}
				err := d.sendTerminalTaskReport(ctx, item.report, nil)
				if err == nil {
					err = d.terminalReports.acknowledge(item)
				}
				resultMu.Lock()
				if err == nil {
					remaining--
					delivered++
				} else {
					d.logger.Warn("pending terminal report remains queued",
						"task", item.report.taskID,
						"kind", item.report.kind,
						"error", err,
					)
				}
				resultMu.Unlock()
			}
		}()
	}
	for _, item := range items {
		select {
		case jobs <- item:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return remaining, delivered
		}
	}
	close(jobs)
	wg.Wait()
	return remaining, delivered
}

func (d *Daemon) terminalReportReplayLoop(ctx context.Context) {
	backoff := terminalReportReplayInitialBackoff
	var timer *time.Timer
	resetTimer := func(delay time.Duration) <-chan time.Time {
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(delay)
		}
		return timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	// Startup is itself a replay trigger. The queue is loaded only after auth
	// preflight, so recovered reports use the daemon's current credential.
	timerCh := resetTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.terminalReportWakeup:
			backoff = terminalReportReplayInitialBackoff
			timerCh = resetTimer(0)
		case <-timerCh:
			pending, delivered := d.replayPendingTerminalReports(ctx)
			if delivered > 0 {
				d.logger.Info("replayed pending terminal reports", "delivered", delivered, "remaining", pending)
			}
			if pending == 0 {
				timerCh = nil
				backoff = terminalReportReplayInitialBackoff
				continue
			}
			timerCh = resetTimer(backoff)
			backoff *= 2
			if backoff > terminalReportReplayMaxBackoff {
				backoff = terminalReportReplayMaxBackoff
			}
		}
	}
}
