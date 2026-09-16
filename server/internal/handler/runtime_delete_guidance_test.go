package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
)

// The refusals these tests cover are the ones a user acts on. A profile spans
// every machine that registered it, so "delete its runtime profile instead" —
// the advice the instance refusal used to give — points at a workspace-wide
// delete that takes healthy machines' runtimes with it, and that a bound agent
// refuses anyway. GH #8456 is a report of exactly that dead end. What both
// messages must now carry is asserted here rather than left to review, because
// the damage is done by the wording, not by the status code.

func decodeConflict(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func conflictMessage(t *testing.T, body map[string]any) string {
	t.Helper()
	msg, _ := body["error"].(string)
	if strings.TrimSpace(msg) == "" {
		t.Fatalf("expected a human-readable error message, got %#v", body)
	}
	return msg
}

// An offline profile-backed instance is the GH #8456 shape: the row the user
// wants gone, on a machine that is never coming back. The refusal has to tell
// them it is reclaimed on its own, because that is the whole answer — every
// other route they could take from here is destructive.
func TestDeleteAgentRuntime_OfflineProfileInstanceRefusalPointsAtAutoCleanup(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Retired Test Machine")
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'offline' WHERE id = $1`, runtimeID); err != nil {
		t.Fatalf("mark runtime offline: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/runtimes/"+runtimeID, nil)
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.DeleteAgentRuntime(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)

	if got, _ := body["code"].(string); got != "runtime_profile_instance_delete_unsupported" {
		t.Fatalf("code changed, installed clients branch on it: %q", got)
	}
	if !strings.Contains(msg, "automatically") {
		t.Fatalf("refusal must say the row is reclaimed automatically, got: %s", msg)
	}
	if !strings.Contains(msg, "Retired Test Machine Profile") {
		t.Fatalf("refusal must name the owning profile, got: %s", msg)
	}
	// The old advice. Sending a user to a workspace-wide delete to clean up one
	// machine is the defect; a message may mention the profile's blast radius,
	// but must not recommend it as the fix.
	if strings.Contains(msg, "delete its runtime profile instead") {
		t.Fatalf("refusal still recommends deleting the shared profile: %s", msg)
	}

	if got, _ := body["auto_cleanup_after_days"].(float64); int(got) != service.OfflineRuntimeTTLDays() {
		t.Fatalf("auto_cleanup_after_days = %v, want %d", got, service.OfflineRuntimeTTLDays())
	}
	if got, _ := body["runtime_status"].(string); got != "offline" {
		t.Fatalf("runtime_status = %q, want offline", got)
	}
	if got, _ := body["profile_name"].(string); got != "Retired Test Machine Profile" {
		t.Fatalf("profile_name = %q", got)
	}
}

// Online is the one case where waiting is not the answer — the daemon would
// re-register the row immediately. The refusal has to say so instead of
// promising a cleanup that will not happen.
func TestDeleteAgentRuntime_OnlineProfileInstanceRefusalSaysStopTheDaemon(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Live Machine")

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/runtimes/"+runtimeID, nil)
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.DeleteAgentRuntime(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)

	if !strings.Contains(msg, "still online") {
		t.Fatalf("online refusal must explain the daemon would re-register, got: %s", msg)
	}
	if got, _ := body["runtime_status"].(string); got != "online" {
		t.Fatalf("runtime_status = %q, want online", got)
	}
}

// The cascade endpoint shares the guard, so it must share the guidance —
// `multica runtime delete --cascade` is the retry a blocked user reaches for.
func TestUnbindAgentsAndDeleteRuntime_ProfileInstanceRefusalCarriesGuidance(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Cascade Guidance Machine")
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'offline' WHERE id = $1`, runtimeID); err != nil {
		t.Fatalf("mark runtime offline: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/runtimes/"+runtimeID+"/unbind-agents-and-delete",
		strings.NewReader(`{"expected_active_agent_ids":[]}`))
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.UnbindAgentsAndDeleteRuntime(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)
	if !strings.Contains(msg, "automatically") {
		t.Fatalf("cascade refusal must carry the same guidance, got: %s", msg)
	}
	if got, _ := body["auto_cleanup_after_days"].(float64); int(got) != service.OfflineRuntimeTTLDays() {
		t.Fatalf("auto_cleanup_after_days = %v", got)
	}
}

// The profile refusal's job is to show that the agents blocking the delete sit
// on a *different* machine than the stale one the user was cleaning up. A bare
// count cannot do that, and the user's reasonable next move — unbinding agents
// that were working fine — is the damage this message exists to prevent.
func TestDeleteRuntimeProfile_ActiveAgentConflictNamesAgentsAndMachines(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Shared Devin Profile", "codex", "shared-devin")
	healthyRuntimeID := insertProfileRuntimeFixture(t, ctx, profileID, "HEALTHY-DESKTOP", "codex")
	staleRuntimeID := insertProfileRuntimeFixture(t, ctx, profileID, "RETIRED-LAPTOP", "codex")
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'offline' WHERE id = $1`, staleRuntimeID); err != nil {
		t.Fatalf("mark stale runtime offline: %v", err)
	}
	// The blocker is bound to the healthy machine, not the one being cleaned up.
	_ = createCascadeFixtureAgent(t, ctx, healthyRuntimeID, "Production Agent")

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/runtime-profiles/"+profileID, nil)
	req = withURLParams(req, "id", testWorkspaceID, "profileId", profileID)
	notifier := &recordingRuntimeGoneNotifier{}
	h := *testHandler
	h.DaemonRuntimeGone = notifier
	h.DeleteRuntimeProfile(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)

	if got, _ := body["code"].(string); got != "runtime_profile_has_active_agents" {
		t.Fatalf("code = %q", got)
	}
	if !strings.Contains(msg, "Production Agent") {
		t.Fatalf("refusal must name the blocking agent, got: %s", msg)
	}
	if !strings.Contains(msg, "HEALTHY-DESKTOP") {
		t.Fatalf("refusal must name the machine the blocker is on, got: %s", msg)
	}
	if !strings.Contains(msg, "Shared Devin Profile") {
		t.Fatalf("refusal must name the profile, got: %s", msg)
	}

	agents, _ := body["active_agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("expected 1 blocking agent on the response, got %d", len(agents))
	}
	entry, _ := agents[0].(map[string]any)
	if got, _ := entry["runtime_name"].(string); got != "HEALTHY-DESKTOP" {
		t.Fatalf("active_agents[0].runtime_name = %q", got)
	}
	if got, _ := entry["name"].(string); got != "Production Agent" {
		t.Fatalf("active_agents[0].name = %q", got)
	}

	// The guard itself must not have moved: both rows and the profile survive.
	var profileRows, rtRows int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM runtime_profile WHERE id = $1`, profileID).Scan(&profileRows); err != nil {
		t.Fatalf("count profile rows: %v", err)
	}
	if profileRows != 1 {
		t.Fatalf("profile should survive the refusal, found %d", profileRows)
	}
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM agent_runtime WHERE profile_id = $1`, profileID).Scan(&rtRows); err != nil {
		t.Fatalf("count runtime rows: %v", err)
	}
	if rtRows != 2 {
		t.Fatalf("both runtimes should survive the refusal, found %d", rtRows)
	}
}

// Above maxNamedBlockingAgents the message must stay readable without hiding
// the scale of what the user is about to unbind.
func TestDeleteRuntimeProfile_ActiveAgentConflictTruncatesLongAgentLists(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Crowded Profile", "codex", "crowded-codex")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "CROWDED-HOST", "codex")
	total := maxNamedBlockingAgents + 3
	for i := 0; i < total; i++ {
		_ = createCascadeFixtureAgent(t, ctx, runtimeID, "Crowd Agent "+string(rune('A'+i)))
	}

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/runtime-profiles/"+profileID, nil)
	req = withURLParams(req, "id", testWorkspaceID, "profileId", profileID)
	notifier := &recordingRuntimeGoneNotifier{}
	h := *testHandler
	h.DaemonRuntimeGone = notifier
	h.DeleteRuntimeProfile(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)

	if !strings.Contains(msg, "and 3 more") {
		t.Fatalf("expected a truncation tail for %d agents, got: %s", total, msg)
	}
	// The full set still reaches a client that wants to render it.
	agents, _ := body["active_agents"].([]any)
	if len(agents) != total {
		t.Fatalf("active_agents should carry every blocker, got %d want %d", len(agents), total)
	}
}

// Retention GC skips a runtime that still has a bound agent, so an offline
// instance in that state must NOT be told to sit and wait — that would be a
// fresh piece of wrong advice replacing the old one.
func TestDeleteAgentRuntime_OfflineProfileInstanceWithBoundAgentsDoesNotPromiseCleanup(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Held Machine")
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'offline' WHERE id = $1`, runtimeID); err != nil {
		t.Fatalf("mark runtime offline: %v", err)
	}
	_ = createCascadeFixtureAgent(t, ctx, runtimeID, "Still Bound Agent")

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/runtimes/"+runtimeID, nil)
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.DeleteAgentRuntime(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	body := decodeConflict(t, w)
	msg := conflictMessage(t, body)

	if strings.Contains(msg, "without any action from you") {
		t.Fatalf("must not promise cleanup while an agent holds the runtime: %s", msg)
	}
	if !strings.Contains(msg, "still bound to it") {
		t.Fatalf("refusal should say the bound agents hold it in place, got: %s", msg)
	}
	if got, _ := body["active_agent_count"].(float64); int(got) != 1 {
		t.Fatalf("active_agent_count = %v, want 1", got)
	}
}
