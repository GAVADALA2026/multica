package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// seedPriorRunWithSnapshot creates the terminal prior task this agent's next
// claim will RESUME, carrying the snapshot that run was handed. Passing an
// empty snapshot models a run recorded before the column existed.
//
// The session_id and matching runtime_id are load-bearing, not decoration: a
// claim reports a delta only when it actually hands a resumed session back, and
// that session has to come from THIS run for the delta to describe the context
// the agent will continue from (MUL-7344). A prior run with no resumable
// session is a cold start, and a cold start has no delta to report.
func seedPriorRunWithSnapshot(t *testing.T, agentID, runtimeID, issueID, snapshot string) {
	t.Helper()
	cols := testutil.Cols{
		"runtime_id":   runtimeID,
		"issue_id":     issueID,
		"status":       "completed",
		"session_id":   "prior-run-session-" + issueID,
		"work_dir":     "/tmp/prior-run-workdir",
		"started_at":   testutil.Raw("now() - interval '1 hour'"),
		"completed_at": testutil.Raw("now() - interval '50 minutes'"),
	}
	if snapshot != "" {
		cols["issue_snapshot"] = snapshot
	}
	dbfx.Task(t, agentID, cols)
}

// issueSnapshotJSON renders a stored snapshot for the issue fixture, letting a
// caller override individual fields to model what the previous run saw.
// createClaimReclaimAgentAndIssue always creates an unassigned issue in
// status in_progress with priority none, and titles it "<name> issue".
func issueSnapshotJSON(t *testing.T, version int, title, description, status string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"v":                  version,
		"status":             status,
		"title_sha256":       sha256Hex(title),
		"description_sha256": sha256Hex(description),
	})
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return string(raw)
}

// TestClaimTaskByRuntime_IssueUnchangedReportsEmptyDelta: the previous run saw
// the issue exactly as it is now, so the claim reports "compared, nothing
// changed" — the one answer a daemon may use to skip workflow step 1's read.
func TestClaimTaskByRuntime_IssueUnchangedReportsEmptyDelta(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Issue snapshot unchanged runtime")
	const name = "Issue snapshot unchanged agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID,
		issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"))
	createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

	resp := claimCommentTask(t, runtimeID, "issue-snapshot-unchanged")
	if !resp.Task.IssueStateDeltaKnown {
		t.Fatalf("issue_state_delta_known must be true when the comparison ran")
	}
	if len(resp.Task.IssueChangedFields) != 0 {
		t.Errorf("issue_changed_fields = %v, want empty for an unchanged issue", resp.Task.IssueChangedFields)
	}
	if resp.Task.IssueStatus != "in_progress" {
		t.Errorf("issue_status = %q, want in_progress", resp.Task.IssueStatus)
	}
}

// TestClaimTaskByRuntime_IssueChangedNamesFields: the comparison reports which
// of the compared fields moved, in the fixed order, so the daemon can name them
// instead of telling the agent to diff the whole record.
func TestClaimTaskByRuntime_IssueChangedNamesFields(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Issue snapshot changed runtime")
	const name = "Issue snapshot changed agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// The previous run saw a different description and a different status;
	// title and priority are unchanged and must NOT be reported.
	seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID,
		issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "an older description", "todo"))
	createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

	resp := claimCommentTask(t, runtimeID, "issue-snapshot-changed")
	if !resp.Task.IssueStateDeltaKnown {
		t.Fatalf("issue_state_delta_known must be true when the comparison ran")
	}
	if got := strings.Join(resp.Task.IssueChangedFields, ","); got != "description,status" {
		t.Errorf("issue_changed_fields = %q, want \"description,status\"", got)
	}
}

// TestClaimTaskByRuntime_NoPriorSnapshotIsNotUnchanged pins the ambiguity this
// flag exists to remove. A first run on an issue, and a prior run recorded
// before the column existed, both produce an empty changed-field list — the
// same bytes as a genuine "nothing changed". Only the flag separates them, and
// getting it wrong means an agent continues from stale context.
//
// The current status and assignee ship regardless: a run that must read the
// issue anyway loses nothing, and workflow step 3 needs them either way.
func TestClaimTaskByRuntime_NoPriorSnapshotIsNotUnchanged(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	cases := []struct {
		label    string
		priorRun bool
	}{
		// First run on the issue: the anchor query finds no row at all.
		{label: "no prior run at all", priorRun: false},
		// A prior run exists but predates the column, so its snapshot is NULL.
		{label: "prior run, no snapshot", priorRun: true},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			ctx := context.Background()
			runtimeID := createClaimReclaimRuntime(t, ctx, "Issue snapshot unknown runtime "+tc.label)
			agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, "Issue snapshot unknown agent "+tc.label)
			if tc.priorRun {
				seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID, "")
			}
			createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

			resp := claimCommentTask(t, runtimeID, "issue-snapshot-unknown")
			if resp.Task.IssueStateDeltaKnown {
				t.Errorf("issue_state_delta_known must be false when nothing was compared")
			}
			if len(resp.Task.IssueChangedFields) != 0 {
				t.Errorf("issue_changed_fields = %v, want empty", resp.Task.IssueChangedFields)
			}
			if resp.Task.IssueStatus != "in_progress" {
				t.Errorf("issue_status = %q, want in_progress even when the delta is unknown", resp.Task.IssueStatus)
			}
		})
	}
}

// TestClaimTaskByRuntime_ForeignSnapshotVersionIsUnknown: a snapshot written by
// a different shape version compared a different set of fields, so neither
// "unchanged" nor a changed-field list would be a true statement about it.
func TestClaimTaskByRuntime_ForeignSnapshotVersionIsUnknown(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Issue snapshot version runtime")
	const name = "Issue snapshot version agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// Byte-identical to the unchanged case apart from `v`.
	seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID,
		issueSnapshotJSON(t, issueSnapshotVersion+1, name+" issue", "", "in_progress"))
	createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

	resp := claimCommentTask(t, runtimeID, "issue-snapshot-version")
	if resp.Task.IssueStateDeltaKnown {
		t.Errorf("a snapshot from another shape version must read as not compared, not as unchanged")
	}
}

// TestClaimTaskByRuntime_RecordsItsOwnIssueSnapshot closes the loop: the
// comparison above is only ever true if each claim persists what it saw. This
// is the write half, and it must happen for an ASSIGNMENT claim too — a run
// that records nothing leaves the next one with no baseline.
func TestClaimTaskByRuntime_RecordsItsOwnIssueSnapshot(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Issue snapshot write runtime")
	const name = "Issue snapshot write agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// No trigger comment: this is the assignment path.
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID,
		"issue_id":   issueID,
	})

	req := newDaemonTokenRequest("POST", "/api/daemon/runtimes/"+runtimeID+"/tasks/claim", nil, testWorkspaceID, "issue-snapshot-write")
	req = withURLParam(req, "runtimeId", runtimeID)
	testutil.Call(t, testHandler.ClaimTaskByRuntime, req).Want(http.StatusOK)

	var raw []byte
	if err := testPool.QueryRow(ctx, `SELECT issue_snapshot FROM agent_task_queue WHERE id = $1`, taskID).Scan(&raw); err != nil {
		t.Fatalf("read back issue_snapshot: %v", err)
	}
	stored, ok := decodeIssueStateSnapshot(raw)
	if !ok {
		t.Fatalf("claim did not persist a decodable issue snapshot, got %q", string(raw))
	}
	if stored.Status != "in_progress" {
		t.Errorf("stored snapshot = %+v, want the issue's status", stored)
	}
	if stored.TitleSHA256 != sha256Hex(name+" issue") {
		t.Errorf("stored title hash does not match the claimed issue's title")
	}
	if strings.Contains(string(raw), name+" issue") {
		t.Errorf("snapshot must store a hash, never the issue text: %s", string(raw))
	}
}

// TestClaimTaskByRuntime_BothDeltasShareOneAnchor pins that the comment delta
// and the issue-state delta are resolved from the SAME row — the resumed run's
// — so "since your last run" is one fact on a claim rather than two
// independently-resolved ones that could disagree. Both now ride the row
// GetLastTaskSession already returns, so a claim reads no extra anchor at all.
func TestClaimTaskByRuntime_BothDeltasShareOneAnchor(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "Shared anchor runtime")
	const name = "Shared anchor agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// One prior run carries both halves of the anchor: its started_at dates the
	// comment delta, its snapshot dates the issue-state delta.
	seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID,
		issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"))

	// A member comment after that anchor, so the comment delta is non-zero and
	// the two deltas cannot both be trivially empty.
	dbfx.Comment(t, issueID, "something happened while you were away")
	createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

	resp := claimCommentTask(t, runtimeID, "shared-anchor-claim")
	if !resp.Task.DeltaKnown {
		t.Errorf("new_comments_delta_known must be true — the resumed run's row carries started_at")
	}
	if resp.Task.NewCommentCount != 1 {
		t.Errorf("new_comment_count = %d, want 1", resp.Task.NewCommentCount)
	}
	if !resp.Task.IssueStateDeltaKnown {
		t.Errorf("issue_state_delta_known must be true — the same row carries the snapshot")
	}
	if len(resp.Task.IssueChangedFields) != 0 {
		t.Errorf("issue_changed_fields = %v, want empty", resp.Task.IssueChangedFields)
	}
	// Both anchors describe the same prior run, so the comment anchor must be
	// that run's started_at, not this claim's own timestamp.
	if resp.Task.NewCommentsSince == "" {
		t.Errorf("new_comments_since must carry the shared anchor")
	}
}

// TestClaimTaskByRuntime_DeltasDateFromTheResumedRun is the regression Niko's
// review found, and it is the invariant the whole mechanism rests on: a claim
// may only report a delta measured from the run whose session it hands back.
//
// "Newest run" and "resumed run" are different rows more often than they look.
// GetLastTaskSession skips poisoned sessions, so a newer run that died on an
// unprocessable conversation is passed over for an older healthy one; a manual
// rerun resumes whatever source the operator clicked. In both cases the newest
// run's snapshot describes an issue the resumed session never saw — so
// comparing against it can report "unchanged" to an agent whose memory predates
// the edit, which is the exact failure this mechanism exists to prevent, and
// the only one with no symptom at runtime.
//
// Both sub-cases put the CURRENT issue in the state the newer run recorded, so
// a claim that anchors on the newest run finds nothing changed and says so.
func TestClaimTaskByRuntime_DeltasDateFromTheResumedRun(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, manualRerun := range []bool{false, true} {
		label := "older healthy session after a poisoned newer run"
		if manualRerun {
			label = "manual rerun of an older run"
		}
		t.Run(label, func(t *testing.T) {
			ctx := context.Background()
			runtimeID := createClaimReclaimRuntime(t, ctx, "resumed anchor runtime "+label)
			name := "resumed anchor agent " + label
			agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

			// The run we will resume. Its snapshot says the description was
			// "older instructions" — which is NOT what the issue says now.
			olderID := dbfx.Task(t, agentID, testutil.Cols{
				"runtime_id":     runtimeID,
				"issue_id":       issueID,
				"status":         "failed",
				"failure_reason": "agent_error.unknown",
				"session_id":     "older-healthy-session",
				"work_dir":       "/tmp/resumed-anchor-workdir",
				"started_at":     testutil.Raw("now() - interval '2 hours'"),
				"completed_at":   testutil.Raw("now() - interval '110 minutes'"),
				"issue_snapshot": issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "older instructions", "in_progress"),
			})
			// A NEWER run that saw the issue as it is now. Anchoring here is the
			// bug: its session is not the one being resumed.
			newer := testutil.Cols{
				"runtime_id":     runtimeID,
				"issue_id":       issueID,
				"status":         "failed",
				"failure_reason": "api_invalid_request", // poisons the session
				"session_id":     "newer-unresumable-session",
				"work_dir":       "/tmp/resumed-anchor-other-workdir",
				"started_at":     testutil.Raw("now() - interval '1 hour'"),
				"completed_at":   testutil.Raw("now() - interval '50 minutes'"),
				"issue_snapshot": issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"),
			}
			if manualRerun {
				// Nothing wrong with the newer run here; the operator simply
				// chose to resume the older one.
				newer["status"] = "completed"
				delete(newer, "failure_reason")
			}
			dbfx.Task(t, agentID, newer)

			dbfx.Comment(t, issueID, "context between the two runs", testutil.Cols{
				"created_at": testutil.Raw("now() - interval '90 minutes'"),
			})
			triggerID := dbfx.Comment(t, issueID, "continue")
			current := testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID, "trigger_comment_id": triggerID}
			if manualRerun {
				current["rerun_of_task_id"] = olderID
				current["force_fresh_session"] = true
			}
			dbfx.Task(t, agentID, current)

			resp := claimCommentTask(t, runtimeID, "resumed-anchor-claim")
			if resp.Task.PriorSessionID != "older-healthy-session" {
				t.Fatalf("fixture did not exercise the older-session resume path: prior_session_id = %q", resp.Task.PriorSessionID)
			}
			// The resumed run's snapshot says "older instructions"; the issue
			// says something else now. That IS a change, and the claim must say
			// so rather than comparing against the run it is not resuming.
			if !resp.Task.IssueStateDeltaKnown {
				t.Fatalf("the resumed run carries a snapshot, so the comparison must run")
			}
			if got := strings.Join(resp.Task.IssueChangedFields, ","); got != "description" {
				t.Errorf("issue_changed_fields = %q, want \"description\" — measured from the RESUMED run, not the newest one", got)
			}
			// Same rule for the comment anchor: dating it from the newest run
			// would hide comments the resumed session never saw.
			if !resp.Task.DeltaKnown || resp.Task.NewCommentCount != 1 || resp.Task.NewCommentsSince == "" {
				t.Fatalf("comment delta must include the intervening comment: known=%v count=%d since=%q", resp.Task.DeltaKnown, resp.Task.NewCommentCount, resp.Task.NewCommentsSince)
			}
			if resp.Task.NewCommentsSince != "" {
				since, err := time.Parse(time.RFC3339, resp.Task.NewCommentsSince)
				if err != nil {
					t.Fatalf("new_comments_since is not RFC3339: %v", err)
				}
				if time.Since(since) < 90*time.Minute {
					t.Errorf("new_comments_since = %s — that is the newer run's start, not the resumed run's", resp.Task.NewCommentsSince)
				}
			}
		})
	}
}

// TestClaimTaskByRuntime_NeverStartedAnchorRowReportsNoIssueDelta closes the
// gap between "the row whose session we resume" and "the run whose memory we
// resume". GetLastTaskSession returns the session's latest TERMINAL row, and a
// retry child that inherited the session can be claimed — writing its snapshot
// — and then fail before the agent ever ran (runtime offline during prepare).
// Its snapshot describes the issue as of ITS claim, which the session's memory
// never saw. Anchoring on it would report "unchanged" across an edit made after
// the last run that actually executed: the silent failure this mechanism must
// never produce. A never-started anchor row dates neither delta.
func TestClaimTaskByRuntime_NeverStartedAnchorRowReportsNoIssueDelta(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "never-started anchor runtime")
	const name = "never-started anchor agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// The run that really executed in session S. Its memory — and its
	// snapshot — say the description was "older instructions".
	dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID, "issue_id": issueID, "status": "completed",
		"session_id": "never-started-anchor-session", "work_dir": "/tmp/never-started-anchor",
		"started_at":     testutil.Raw("now() - interval '2 hours'"),
		"completed_at":   testutil.Raw("now() - interval '110 minutes'"),
		"issue_snapshot": issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "older instructions", "in_progress"),
	})
	// A retry child that inherited session S: claimed after the description
	// was edited to what it is now (so its snapshot matches the current issue),
	// then the runtime went offline before it started. started_at stays NULL.
	dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID, "issue_id": issueID, "status": "failed",
		"failure_reason": "runtime_offline", "error": "runtime went offline",
		"session_id": "never-started-anchor-session", "work_dir": "/tmp/never-started-anchor",
		"dispatched_at":  testutil.Raw("now() - interval '1 hour'"),
		"completed_at":   testutil.Raw("now() - interval '50 minutes'"),
		"issue_snapshot": issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"),
	})
	triggerID := dbfx.Comment(t, issueID, "continue")
	dbfx.Task(t, agentID, testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID, "trigger_comment_id": triggerID})

	resp := claimCommentTask(t, runtimeID, "never-started-anchor-claim")
	if resp.Task.PriorSessionID != "never-started-anchor-session" {
		t.Fatalf("fixture did not resume the session through its never-started row: prior_session_id = %q", resp.Task.PriorSessionID)
	}
	// The resumed memory predates the edit; the only true answers are "not
	// compared" or "description changed". Never "unchanged".
	if resp.Task.IssueStateDeltaKnown && len(resp.Task.IssueChangedFields) == 0 {
		t.Errorf("issue reported unchanged against a snapshot the resumed memory never saw")
	}
	if resp.Task.IssueStateDeltaKnown {
		t.Errorf("a never-started anchor row must not date the issue delta; got known=true changed=%q", strings.Join(resp.Task.IssueChangedFields, ","))
	}
	if resp.Task.DeltaKnown {
		t.Errorf("a never-started anchor row must not date the comment delta either")
	}
}

// TestClaimTaskByRuntime_NoAdoptedSessionHasNoDeltas pins the other side of the
// resumed-run rule: a delta exists only when this claim actually hands a
// resumed session back. Each mode below leaves the agent with no continued
// context, so there is nothing for a delta to be measured against, and the
// daemon must perform the reads it always has.
//
// Each fixture seeds a prior run WITH a usable snapshot and an unread comment,
// so a claim that reported a delta anyway would have something to report — the
// assertion fails on a real regression, not on an empty fixture.
func TestClaimTaskByRuntime_NoAdoptedSessionHasNoDeltas(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, mode := range []string{"missing_session", "different_runtime", "force_fresh", "poisoned_rerun"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			runtimeID := createClaimReclaimRuntime(t, ctx, "no-adopted-session runtime "+mode)
			name := "no-adopted-session agent " + mode
			agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)
			prior := testutil.Cols{
				"runtime_id":     runtimeID,
				"issue_id":       issueID,
				"status":         "completed",
				"session_id":     "no-adopted-session-prior",
				"started_at":     testutil.Raw("now() - interval '1 hour'"),
				"completed_at":   testutil.Raw("now() - interval '50 minutes'"),
				"issue_snapshot": issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"),
			}
			switch mode {
			case "missing_session":
				delete(prior, "session_id")
			case "different_runtime":
				prior["runtime_id"] = createClaimReclaimRuntime(t, ctx, "no-adopted-session older runtime")
			case "poisoned_rerun":
				prior["status"] = "failed"
				prior["failure_reason"] = "api_invalid_request"
			}
			priorID := dbfx.Task(t, agentID, prior)
			dbfx.Comment(t, issueID, "an unseen comment")
			triggerID := dbfx.Comment(t, issueID, "continue")
			current := testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID, "trigger_comment_id": triggerID}
			if mode == "force_fresh" || mode == "poisoned_rerun" {
				current["force_fresh_session"] = true
			}
			if mode == "poisoned_rerun" {
				current["rerun_of_task_id"] = priorID
			}
			dbfx.Task(t, agentID, current)
			got := claimCommentTask(t, runtimeID, "no-adopted-session").Task
			if got.PriorSessionID != "" || got.IssueStateDeltaKnown || got.DeltaKnown || got.NewCommentsSince != "" || got.NewCommentCount != 0 {
				t.Fatalf("no adopted session must keep both deltas unknown: %+v", got)
			}
		})
	}
}

// TestClaimTaskByRuntime_AssigneeAndPriorityAreOutOfScope pins the v2 narrowing.
// Reassigning an issue or changing its priority does not change what the agent
// has to read, so neither is compared — an issue whose ONLY change is one of
// them is reported as unchanged, and the prompt says which three fields were
// checked so that report cannot be read as "nothing about this issue moved".
//
// The current assignee still ships on every claim; the agent answers "is this
// mine now" from that value, not from the comparison.
func TestClaimTaskByRuntime_AssigneeAndPriorityAreOutOfScope(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID := createClaimReclaimRuntime(t, ctx, "narrowed scope runtime")
	const name = "narrowed scope agent"
	agentID, issueID := createClaimReclaimAgentAndIssue(t, ctx, runtimeID, name)

	// The prior run saw the three compared fields exactly as they are now.
	seedPriorRunWithSnapshot(t, agentID, runtimeID, issueID,
		issueSnapshotJSON(t, issueSnapshotVersion, name+" issue", "", "in_progress"))
	// Since that run the issue was reassigned and re-prioritised. Neither is in
	// the compared set.
	dbfx.Exec(t, `UPDATE issue SET assignee_type = 'agent', assignee_id = $1, priority = 'urgent' WHERE id = $2`,
		agentID, issueID)
	createCommentTriggeredClaimTask(t, ctx, agentID, runtimeID, issueID, nil)

	resp := claimCommentTask(t, runtimeID, "narrowed-scope-claim")
	if !resp.Task.IssueStateDeltaKnown {
		t.Fatalf("the comparison must still run")
	}
	if len(resp.Task.IssueChangedFields) != 0 {
		t.Errorf("issue_changed_fields = %v, want empty — assignee and priority are out of scope in v2",
			resp.Task.IssueChangedFields)
	}
	// The value the agent actually needs is still delivered.
	if resp.Task.IssueAssigneeType != "agent" || resp.Task.IssueAssigneeID != agentID {
		t.Errorf("current assignee must still ship: type=%q id=%q, want agent/%s",
			resp.Task.IssueAssigneeType, resp.Task.IssueAssigneeID, agentID)
	}
}
