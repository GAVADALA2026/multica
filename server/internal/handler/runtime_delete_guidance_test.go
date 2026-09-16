package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
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

// createSystemFixtureAgent inserts a product-owned agent. kind and system_key
// are independent here on purpose: Mika is kind='user' with system_key='mika'
// (it must stay visible and assignable), while a builder carrier is
// kind='system'. The refusals key off system_key for exactly that reason.
func createSystemFixtureAgent(t *testing.T, ctx context.Context, runtimeID, name, kind, systemKey string) string {
	t.Helper()
	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, visibility, max_concurrent_tasks, owner_id, kind, system_key
		)
		VALUES ($1, $2, '', 'cloud', '{}'::jsonb, $3, 'private', 1, $4, $5, $6)
		RETURNING id
	`, testWorkspaceID, name, runtimeID, testUserID, kind, systemKey).Scan(&agentID); err != nil {
		t.Fatalf("insert system fixture agent (%s): %v", systemKey, err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})
	return agentID
}

// Mika cannot be archived — the archive endpoint rejects any agent carrying a
// system_key — and there is no supported way to move it to another runtime. So
// the refusal must not tell the user to reassign or archive it; saying so would
// be the same unactionable-advice defect this change set exists to remove.
func TestDeleteRuntimeProfile_MikaBlockerDoesNotSuggestArchiving(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Mika Held Profile", "codex", "mika-held")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "MIKA-HOST", "codex")
	createSystemFixtureAgent(t, ctx, runtimeID, "Mika (profile guard)", "user", "mika")

	body := deleteProfileExpectingConflict(t, ctx, profileID)
	msg := conflictMessage(t, body)

	if strings.Contains(msg, "can be reassigned or archived") || strings.Contains(msg, "Reassign or archive them first.") {
		t.Fatalf("Mika cannot be reassigned or archived; refusal must not say so: %s", msg)
	}
	if !strings.Contains(msg, "Mika is built into Multica") {
		t.Fatalf("refusal must explain Mika's status honestly, got: %s", msg)
	}
	if !strings.Contains(msg, "built into Multica)") {
		t.Fatalf("the listed blocker should be marked as product-owned, got: %s", msg)
	}

	agents, _ := body["active_agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(agents))
	}
	entry, _ := agents[0].(map[string]any)
	if got, _ := entry["system_key"].(string); got != "mika" {
		t.Fatalf("system_key = %q, want mika — clients need it to localize", got)
	}
	if got, _ := entry["kind"].(string); got != "user" {
		t.Fatalf("kind = %q; Mika is deliberately kind=user, so kind must not be the discriminator", got)
	}
}

// A builder carrier is hidden from the agent list entirely, so neither
// "reassign" nor "archive" is reachable. The way out is its Builder session.
func TestDeleteRuntimeProfile_BuilderCarrierBlockerPointsAtItsSession(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Builder Held Profile", "codex", "builder-held")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "BUILDER-HOST", "codex")
	createSystemFixtureAgent(t, ctx, runtimeID,
		".multica-agent-builder-flow1", "system", "agent_builder:flow1")

	body := deleteProfileExpectingConflict(t, ctx, profileID)
	msg := conflictMessage(t, body)

	if strings.Contains(msg, "can be reassigned or archived") || strings.Contains(msg, "Reassign or archive them first.") {
		t.Fatalf("a builder carrier is not in the agent list; refusal must not say so: %s", msg)
	}
	if !strings.Contains(msg, "Agent Builder session") {
		t.Fatalf("refusal must point at the Builder session, got: %s", msg)
	}

	agents, _ := body["active_agents"].([]any)
	entry, _ := agents[0].(map[string]any)
	if got, _ := entry["system_key"].(string); got != "agent_builder:flow1" {
		t.Fatalf("system_key = %q", got)
	}
}

// With both kinds present each needs its own clause: the user agent is
// actionable, the carrier is not, and collapsing them loses one or the other.
func TestDeleteRuntimeProfile_MixedBlockersGiveEachItsOwnRemedy(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Mixed Held Profile", "codex", "mixed-held")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "MIXED-HOST", "codex")
	_ = createCascadeFixtureAgent(t, ctx, runtimeID, "Ordinary Agent")
	createSystemFixtureAgent(t, ctx, runtimeID, "Mika (mixed guard)", "user", "mika")
	createSystemFixtureAgent(t, ctx, runtimeID,
		".multica-agent-builder-flow2", "system", "agent_builder:flow2")

	body := deleteProfileExpectingConflict(t, ctx, profileID)
	msg := conflictMessage(t, body)

	for _, want := range []string{
		"not marked as built into Multica can be reassigned or archived",
		"Agent Builder session",
		"Mika is built into Multica",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("mixed refusal missing %q, got: %s", want, msg)
		}
	}
	if got, _ := body["active_agent_count"].(float64); int(got) != 3 {
		t.Fatalf("active_agent_count = %v, want 3", got)
	}
}

// The response carries a bounded sample, not the profile's whole agent set:
// this query runs inside the delete transaction with rows locked, and the
// response is buffered whole before it is written.
func TestDeleteRuntimeProfile_ActiveAgentResponseIsBounded(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Crowded Bounded Profile", "codex", "crowded-bounded")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "CROWDED-HOST", "codex")
	total := maxReportedBlockingAgents + 7
	for i := 0; i < total; i++ {
		_ = createCascadeFixtureAgent(t, ctx, runtimeID, fmt.Sprintf("Crowd Agent %03d", i))
	}

	body := deleteProfileExpectingConflict(t, ctx, profileID)

	agents, _ := body["active_agents"].([]any)
	if len(agents) != maxReportedBlockingAgents {
		t.Fatalf("active_agents = %d entries, want the cap %d", len(agents), maxReportedBlockingAgents)
	}
	// The exact size still has to reach the caller, or the cap would silently
	// understate how much is bound to the profile.
	if got, _ := body["active_agent_count"].(float64); int(got) != total {
		t.Fatalf("active_agent_count = %v, want the true total %d", got, total)
	}
	if truncated, _ := body["active_agents_truncated"].(bool); !truncated {
		t.Fatal("active_agents_truncated should be true when the sample is capped")
	}
	if msg := conflictMessage(t, body); !strings.Contains(msg,
		fmt.Sprintf("and %d more", total-maxNamedBlockingAgents)) {
		t.Fatalf("the sentence must count from the true total, got: %s", msg)
	}
}

// An offline instance held by Mika alone hits the same trap as the profile
// refusal: there is nothing the user can reassign or archive.
func TestDeleteAgentRuntime_OfflineInstanceHeldByMikaDoesNotSuggestArchiving(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Mika Held Machine")
	if _, err := testPool.Exec(ctx,
		`UPDATE agent_runtime SET status = 'offline' WHERE id = $1`, runtimeID); err != nil {
		t.Fatalf("mark runtime offline: %v", err)
	}
	createSystemFixtureAgent(t, ctx, runtimeID, "Mika (instance guard)", "user", "mika")

	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/runtimes/"+runtimeID, nil)
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.DeleteAgentRuntime(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	msg := conflictMessage(t, decodeConflict(t, w))

	if strings.Contains(msg, "can be reassigned or archived") || strings.Contains(msg, "Reassign or archive them first.") {
		t.Fatalf("Mika cannot be reassigned or archived: %s", msg)
	}
	if !strings.Contains(msg, "Mika is built into Multica") {
		t.Fatalf("instance refusal must explain Mika honestly too, got: %s", msg)
	}
	if strings.Contains(msg, "without any action from you") {
		t.Fatalf("must not promise cleanup while Mika holds the runtime: %s", msg)
	}
}

func deleteProfileExpectingConflict(t *testing.T, ctx context.Context, profileID string) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest("DELETE", "/api/workspaces/"+testWorkspaceID+"/runtime-profiles/"+profileID, nil)
	req = withURLParams(req, "id", testWorkspaceID, "profileId", profileID)
	h := *testHandler
	h.DaemonRuntimeGone = &recordingRuntimeGoneNotifier{}
	h.DeleteRuntimeProfile(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	return decodeConflict(t, w)
}

// The remedies must come from the whole blocker set, not the sample. Sorting
// puts the sample's contents outside the caller's control, so a class can fall
// entirely past the cap: twenty ordinary agents that sort first push Mika to
// position 21, and a message built from the sample would then tell the user to
// archive all 21 — the exact unactionable instruction this change removes, just
// deferred until they have worked through the first twenty.
func TestDeleteRuntimeProfile_RemedyCoversBlockersBeyondTheSample(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Beyond Sample Profile", "codex", "beyond-sample")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "BEYOND-HOST", "codex")
	// "Agent NNN" sorts before "Mika ...", filling the whole sample.
	for i := 0; i < maxReportedBlockingAgents; i++ {
		_ = createCascadeFixtureAgent(t, ctx, runtimeID, fmt.Sprintf("Agent %03d", i))
	}
	createSystemFixtureAgent(t, ctx, runtimeID, "Mika (beyond sample)", "user", "mika")

	body := deleteProfileExpectingConflict(t, ctx, profileID)
	msg := conflictMessage(t, body)

	if !strings.Contains(msg, "Mika is built into Multica") {
		t.Fatalf("Mika is blocker #21 and must still be reported, got: %s", msg)
	}
	if strings.Contains(msg, "Reassign or archive them first.") {
		t.Fatalf("the blanket remedy must not cover a Mika the sample never showed: %s", msg)
	}
	if got, _ := body["active_agent_count"].(float64); int(got) != maxReportedBlockingAgents+1 {
		t.Fatalf("active_agent_count = %v", got)
	}
}

// Same boundary for a builder carrier: its remedy is a different surface
// entirely, so losing it past the cap strands the user just as badly.
func TestDeleteRuntimeProfile_BuilderCarrierBeyondTheSampleStillReported(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Beyond Sample Builder", "codex", "beyond-builder")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "BEYOND-BUILD-HOST", "codex")
	for i := 0; i < maxReportedBlockingAgents; i++ {
		_ = createCascadeFixtureAgent(t, ctx, runtimeID, fmt.Sprintf("Agent %03d", i))
	}
	// "zzz-" sorts last, so the carrier lands past the cap.
	createSystemFixtureAgent(t, ctx, runtimeID,
		"zzz-multica-agent-builder-flow9", "system", "agent_builder:flow9")

	msg := conflictMessage(t, deleteProfileExpectingConflict(t, ctx, profileID))
	if !strings.Contains(msg, "Agent Builder session") {
		t.Fatalf("builder carrier is blocker #21 and must still be reported, got: %s", msg)
	}
}

// The class mapping exists twice — the SQL CASE in ListActiveAgentsByProfile,
// because per-class counts have to cover rows the LIMIT excludes, and
// classifyBlockingAgent in Go for the instance path, which reads plain agent
// rows. This pins them to each other: if a future system_key is added to one
// and not the other, the refusal would name a remedy for the wrong blocker.
func TestBlockingAgentClassMatchesSQLClassification(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	profileID := insertRuntimeProfileFixture(t, ctx, "Classifier Pin Profile", "codex", "classifier-pin")
	runtimeID := insertProfileRuntimeFixture(t, ctx, profileID, "CLASSIFIER-HOST", "codex")

	_ = createCascadeFixtureAgent(t, ctx, runtimeID, "Pin Ordinary")
	createSystemFixtureAgent(t, ctx, runtimeID, "Pin Mika", "user", "mika")
	createSystemFixtureAgent(t, ctx, runtimeID, "Pin Builder", "system", "agent_builder:pinflow")
	createSystemFixtureAgent(t, ctx, runtimeID, "Pin Future", "system", "some_future_facility")
	// Near misses for the builder prefix. '_' is a single-character wildcard in
	// SQL LIKE, so a LIKE-based CASE classifies both of these as carriers while
	// Go's HasPrefix does not — and the refusal would then send the user to an
	// Agent Builder session that does not exist.
	createSystemFixtureAgent(t, ctx, runtimeID, "Pin Dash", "system", "agent-builder:not-a-carrier")
	createSystemFixtureAgent(t, ctx, runtimeID, "Pin Wildcard", "system", "agentXbuilder:not-a-carrier")

	rows, err := testHandler.Queries.ListActiveAgentsByProfile(ctx, db.ListActiveAgentsByProfileParams{
		ProfileID:   parseUUID(profileID),
		WorkspaceID: parseUUID(testWorkspaceID),
		MaxRows:     maxReportedBlockingAgents,
	})
	if err != nil {
		t.Fatalf("list blockers: %v", err)
	}
	if len(rows) != 6 {
		t.Fatalf("expected 6 blockers, got %d", len(rows))
	}

	for _, row := range rows {
		fromSQL := blockingAgentClassFromKey(row.BlockerClass)
		fromGo := classifyBlockingAgent(row.SystemKey)
		if fromSQL != fromGo {
			t.Fatalf("%q (system_key=%q): SQL says %q -> %v, Go says %v",
				row.Name, row.SystemKey.String, row.BlockerClass, fromSQL, fromGo)
		}
	}

	// And the per-class counts have to agree with the rows they summarise.
	summary := rows[0]
	for _, tc := range []struct {
		name  string
		got   int64
		class blockingAgentClass
	}{
		{"user", summary.UserCount, blockingAgentUser},
		{"mika", summary.MikaCount, blockingAgentMika},
		{"agent_builder", summary.AgentBuilderCount, blockingAgentBuilderCarrier},
		{"other_system", summary.OtherSystemCount, blockingAgentOtherSystem},
	} {
		var want int64
		for _, row := range rows {
			if classifyBlockingAgent(row.SystemKey) == tc.class {
				want++
			}
		}
		if tc.got != want {
			t.Fatalf("%s_count = %d, rows of that class = %d", tc.name, tc.got, want)
		}
	}
}

// Retention GC needs both no bound agents AND no unfinished task. An agent
// rebound to another machine can leave a deferred run pinned to the old
// runtime, so "no agents bound" alone is not a cleanup promise this server can
// keep. Regression contributed by review.
func TestDeleteAgentRuntime_OfflineInstanceWithUnfinishedTaskDoesNotPromiseCleanup(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID, _ := createProfileBackedRuntime(t, ctx, "Review Retired Host")
	agentID := createCascadeFixtureAgent(t, ctx, runtimeID, "Review Moved Agent")
	issueID := dbfx.Issue(t, "Review deferred task")
	taskID := dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID, "issue_id": issueID, "status": "deferred",
		"fire_at": testutil.Raw("now() + interval '30 days'"),
	})
	targetID := newTestRuntime(t, "Review New Host", "online")
	w := httptest.NewRecorder()
	testHandler.UpdateAgent(w, withURLParam(newRequest("PATCH", "/api/agents/"+agentID,
		map[string]any{"runtime_id": targetID}), "id", agentID))
	if w.Code != http.StatusOK {
		t.Fatalf("rebind: %d %s", w.Code, w.Body.String())
	}
	dbfx.Exec(t, `UPDATE agent_runtime SET status='offline', last_seen_at=now()-interval '8 days' WHERE id=$1`, runtimeID)
	var taskRuntime, taskStatus string
	dbfx.QueryRow(t, `SELECT runtime_id::text, status FROM agent_task_queue WHERE id=$1`, taskID).Scan(&taskRuntime, &taskStatus)
	if taskRuntime != runtimeID || taskStatus != "deferred" {
		t.Fatalf("unexpected task after rebind: %s %s", taskRuntime, taskStatus)
	}
	rows, err := testHandler.Queries.ListStaleOfflineRuntimeGCCandidates(ctx, db.ListStaleOfflineRuntimeGCCandidatesParams{
		StaleSeconds: service.OfflineRuntimeTTLSeconds, MaxPerTick: 10000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range rows {
		if uuidToString(id) == runtimeID {
			t.Fatal("GC unexpectedly allows runtime with deferred task")
		}
	}
	for _, cascade := range []bool{false, true} {
		w = httptest.NewRecorder()
		if cascade {
			r := newRequest("POST", "/api/runtimes/"+runtimeID+"/unbind-agents-and-delete", map[string]any{"expected_active_agent_ids": []string{}})
			testHandler.UnbindAgentsAndDeleteRuntime(w, withURLParam(r, "runtimeId", runtimeID))
		} else {
			testHandler.DeleteAgentRuntime(w, withURLParam(newRequest("DELETE", "/api/runtimes/"+runtimeID, nil), "runtimeId", runtimeID))
		}
		if w.Code != http.StatusConflict {
			t.Fatalf("delete: %d %s", w.Code, w.Body.String())
		}
		body := decodeConflict(t, w)
		msg := conflictMessage(t, body)
		if body["active_agent_count"] != float64(0) {
			t.Fatalf("expected no agent bindings: %#v", body)
		}
		if strings.Contains(msg, "without any action from you") {
			t.Errorf("cascade=%v: GC excludes this 8-day-old runtime with a deferred task, but guidance promises cleanup: %s", cascade, msg)
		}
	}
}

// Builder sessions are creator-scoped reads, so an admin who is not the
// creator gets 403 from list, switch and discard alike. Telling that admin to
// reopen the session is another instruction they cannot carry out.
// Regression contributed by review.
func TestDeleteRuntimeProfile_BuilderRemedyAddressesTheSessionCreator(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	creatorID := dbfx.User(t, "Review Builder Owner", "review-builder-owner@multica.ai")
	dbfx.Insert(t, "member", testutil.Cols{"workspace_id": testWorkspaceID, "user_id": creatorID, "role": "member"})
	runtimeID, profileID := createProfileBackedRuntime(t, ctx, "Review Shared Builder Host")
	dbfx.Exec(t, `UPDATE agent_runtime SET visibility='public' WHERE id=$1`, runtimeID)
	w := httptest.NewRecorder()
	testHandler.CreateAgentBuilderSession(w, newRequestAs(creatorID, "POST", "/api/agent-builder/sessions", map[string]any{"runtime_id": runtimeID}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create builder: %d %s", w.Code, w.Body.String())
	}
	var session CreateAgentBuilderSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	dbfx.Exec(t, `INSERT INTO agent_builder_draft (chat_session_id, workspace_id, draft) VALUES ($1, $2, '{"name":"Saved draft"}'::jsonb)`, session.SessionID, testWorkspaceID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_builder_draft WHERE chat_session_id=$1`, session.SessionID)
	})
	msg := conflictMessage(t, deleteProfileExpectingConflict(t, ctx, profileID))
	w = httptest.NewRecorder()
	testHandler.ListAgentBuilderSessions(w, newRequest("GET", "/api/agent-builder/sessions", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), session.SessionID) {
		t.Fatal("other member's private session unexpectedly visible")
	}
	w = switchBuilderRuntime(t, session.SessionID, runtimeID)
	if w.Code != http.StatusForbidden {
		t.Fatalf("switch: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r := withURLParam(newRequest("DELETE", "/api/chat/sessions/"+session.SessionID, nil), "sessionId", session.SessionID)
	testHandler.DeleteChatSession(w, withChatTestWorkspaceCtx(t, r))
	if w.Code != http.StatusForbidden {
		t.Fatalf("discard: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(msg, "reopen the session") && !strings.Contains(msg, "creator") && !strings.Contains(msg, "owner") {
		t.Fatalf("admin cannot see, switch, or discard the member's Builder, but guidance tells the admin to reopen it: %s", msg)
	}
}
