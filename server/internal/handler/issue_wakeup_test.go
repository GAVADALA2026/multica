package handler

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestIssueWakeupAPIAndTrustedOrigin(t *testing.T) {
	issue := dbfx.Issue(t, "wake api")
	agent := dbfx.Agent(t, "wake api", testRuntimeID)
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup WHERE issue_id=$1", issue)
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup_receipt WHERE wakeup_id IN(SELECT id FROM issue_wakeup WHERE issue_id=$1)", issue)
	body := map[string]any{"agent_id": agent, "kind": "at", "after_seconds": 600, "instruction": "check deployment"}
	req := withURLParam(newRequest("POST", "/api/issues/"+issue+"/wakeups", body), "id", issue)
	// An untrusted task header must not get stamped as delegation provenance.
	forged := dbfx.Task(t, agent, testutil.Cols{"runtime_id": testRuntimeID, "issue_id": issue})
	req.Header.Set("X-Task-ID", forged)
	rec := httptest.NewRecorder()
	testHandler.CreateIssueWakeup(rec, req)
	if rec.Code != 201 {
		t.Fatalf("create %d: %s", rec.Code, rec.Body.String())
	}
	var result db.IssueWakeup
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.SourceTaskID.Valid || uuidToString(result.CreatedBy) != testUserID {
		t.Fatal("untrusted source identity")
	}
	req = withURLParam(newRequest("GET", "/", nil), "id", issue)
	rec = httptest.NewRecorder()
	testHandler.ListIssueWakeups(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	outsider := dbfx.User(t, "wake outsider", "wake-outside@multica.test")
	dbfx.Member(t, testWorkspaceID, outsider, "member")
	// A task-scoped caller must use the initiating human's rights, never the
	// runtime owner's rights, even when registering a wakeup for itself.
	dbfx.Exec(t, "UPDATE agent_task_queue SET originator_user_id=$2,accountable_user_id=$2,status='running',started_at=now() WHERE id=$1", forged, outsider)
	req = withURLParam(newRequest("POST", "/", map[string]any{"kind": "at", "after_seconds": 600, "instruction": "check"}), "id", issue)
	req.Header.Set("X-Agent-ID", agent)
	req.Header.Set("X-Task-ID", forged)
	req.Header.Set("X-Actor-Source", "task_token")
	rec = httptest.NewRecorder()
	testHandler.CreateIssueWakeup(rec, req)
	if rec.Code != 403 {
		t.Fatalf("borrowed runtime owner permission: %d %s", rec.Code, rec.Body.String())
	}
	svc := service.IssueWakeupService{Tasks: testHandler.TaskService}
	if _, err := svc.Disable(context.Background(), parseUUID(issue), result.ID, parseUUID(outsider)); err != service.ErrWakeupForbidden {
		t.Fatalf("other member disabled: %v", err)
	}
	if _, err := svc.Disable(context.Background(), parseUUID(issue), result.ID, parseUUID(testUserID)); err != nil {
		t.Fatal(err)
	}
}

func TestIssueWakeupMutationTrustedActor(t *testing.T) {
	issue := dbfx.Issue(t, "wake mutation actor")
	agent := dbfx.Agent(t, "wake mutation actor", testRuntimeID)
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup WHERE issue_id=$1", issue)
	dbfx.Cleanup(t, "DELETE FROM issue_wakeup_receipt WHERE wakeup_id IN(SELECT id FROM issue_wakeup WHERE issue_id=$1)", issue)
	comment := dbfx.Comment(t, issue, "agent original", testutil.Cols{"author_type": "agent", "author_id": agent})
	svc := service.IssueWakeupService{Tasks: testHandler.TaskService}
	w, err := svc.Create(context.Background(), parseUUID(issue), parseUUID(testUserID), pgtype.UUID{}, service.WakeupInput{AgentID: agent, Kind: "event", Mode: "continuous", EventTypes: []string{"comment.updated", "issue.metadata_changed", "reaction.added", "reaction.removed"}, Instruction: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	run := dbfx.Task(t, agent, testutil.Cols{"runtime_id": testRuntimeID, "issue_id": issue, "status": "running", "started_at": testutil.Raw("now()"), "originator_user_id": testUserID, "accountable_user_id": testUserID})
	call := func(handler http.HandlerFunc, req *http.Request, trusted bool) {
		t.Helper()
		req.Header.Set("X-Task-ID", run)
		if trusted {
			req.Header.Set("X-Agent-ID", agent)
			req.Header.Set("X-Actor-Source", "task_token")
		}
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code < 200 || rec.Code >= 300 {
			t.Fatalf("mutation %d: %s", rec.Code, rec.Body.String())
		}
	}
	// Admin editing another agent's comment: author stays agent; actor is member,
	// and a forged task header cannot suppress the event as the wakeup's own run.
	dbfx.Exec(t, "UPDATE agent_task_queue SET context=jsonb_build_object('wakeup_id',$2::text) WHERE id=$1", run, uuidToString(w.ID))
	call(testHandler.UpdateComment, withURLParam(newRequest("PATCH", "/", map[string]any{"content": "admin edit"}), "commentId", comment), false)
	var actorType, actorID string
	var source *string
	dbfx.QueryRow(t, "SELECT payload->>'actor_type',payload->>'actor_id',payload->>'source_task_id' FROM issue_wakeup_receipt WHERE wakeup_id=$1", w.ID).Scan(&actorType, &actorID, &source)
	if actorType != "member" || actorID != testUserID || source != nil {
		t.Fatalf("wrong editor identity %s %s %v", actorType, actorID, source)
	}
	count := func() int { return dbfx.Count(t, "SELECT count(*) FROM issue_wakeup_receipt WHERE wakeup_id=$1", w.ID) }
	call(testHandler.UpdateComment, withURLParam(newRequest("PATCH", "/", map[string]any{"content": "own wakeup edit"}), "commentId", comment), true)
	req := withURLParams(newRequest("PUT", "/", map[string]any{"value": "secret"}), "id", issue, "key", "test")
	call(testHandler.SetIssueMetadataKey, req, true)
	call(testHandler.AddIssueReaction, withURLParam(newRequest("POST", "/", map[string]any{"emoji": "👍"}), "id", issue), true)
	call(testHandler.RemoveIssueReaction, withURLParam(newRequest("DELETE", "/", map[string]any{"emoji": "👍"}), "id", issue), true)
	if count() != 1 {
		t.Fatalf("own run caused wakeup: %d", count())
	}
	dbfx.Exec(t, "UPDATE agent_task_queue SET context='{}' WHERE id=$1", run)
	req = withURLParams(newRequest("PUT", "/", map[string]any{"value": "changed"}), "id", issue, "key", "test")
	call(testHandler.SetIssueMetadataKey, req, true)
	if count() != 2 {
		t.Fatalf("external run not captured: %d", count())
	}
	dbfx.QueryRow(t, "SELECT payload->>'actor_type',payload->>'actor_id',payload->>'source_task_id' FROM issue_wakeup_receipt WHERE wakeup_id=$1 AND event_type='issue.metadata_changed'", w.ID).Scan(&actorType, &actorID, &source)
	if actorType != "agent" || actorID != agent || source == nil || *source != run {
		t.Fatalf("wrong source identity %s %s %v", actorType, actorID, source)
	}
}
