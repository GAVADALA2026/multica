package handler

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// agentBuilderSystemKeyPrefix marks the hidden execution carrier behind an
// unfinished AI agent-creation flow. The suffix is the flow id.
const agentBuilderSystemKeyPrefix = "agent_builder:"

// A runtime or profile delete is refused while agents are still bound. Telling
// the user to "reassign or archive them" is only true for agents they can
// actually reach: the archive endpoint rejects anything carrying a system_key
// outright, and a builder carrier is not in the agent list at all. Naming one
// remedy for every blocker therefore hands some users an instruction that
// cannot be carried out — the same class of defect this whole change set exists
// to remove, so the refusals classify their blockers instead.
//
// system_key, not kind, is the discriminator. Mika is deliberately kind='user'
// (it must stay visible and assignable) while still being product-owned and
// unarchivable; builder carriers are kind='system'.
type blockingAgentClass int

const (
	// blockingAgentUser is an ordinary workspace agent: rebind or archive it.
	blockingAgentUser blockingAgentClass = iota
	// blockingAgentMika is the workspace's built-in Mika. It cannot be archived
	// (agent.go rejects any system_key) and there is no supported rebind path,
	// so no instruction can honestly be offered for it today.
	blockingAgentMika
	// blockingAgentBuilderCarrier is the hidden carrier behind an unfinished
	// Agent Builder flow. It is released through that session, not the agent list.
	blockingAgentBuilderCarrier
	// blockingAgentOtherSystem is any future product-owned agent. Unarchivable
	// like the rest, with no remedy this code can name specifically.
	blockingAgentOtherSystem
)

func classifyBlockingAgent(systemKey pgtype.Text) blockingAgentClass {
	key := strings.TrimSpace(systemKey.String)
	if !systemKey.Valid || key == "" {
		return blockingAgentUser
	}
	switch {
	case key == service.MikaSystemKey:
		return blockingAgentMika
	case strings.HasPrefix(key, agentBuilderSystemKeyPrefix):
		return blockingAgentBuilderCarrier
	default:
		return blockingAgentOtherSystem
	}
}

// blockingAgentLabel renders one blocker for a refusal sentence. Product-owned
// agents are marked so the reader can tell at a glance why the plain remedy
// does not apply to them.
func blockingAgentLabel(name, runtimeName, runtimeStatus string, class blockingAgentClass) string {
	switch class {
	case blockingAgentBuilderCarrier:
		return fmt.Sprintf("an unfinished Agent Builder session on %q (%s)", runtimeName, runtimeStatus)
	case blockingAgentMika, blockingAgentOtherSystem:
		return fmt.Sprintf("%q on %q (%s, built into Multica)", name, runtimeName, runtimeStatus)
	default:
		return fmt.Sprintf("%q on %q (%s)", name, runtimeName, runtimeStatus)
	}
}

// blockingAgentRemedies returns one clause per distinct recovery path present,
// in a fixed order so the sentence is stable. An empty result means every
// blocker was product-owned and the caller should say so rather than suggest
// an action.
func blockingAgentRemedies(classes map[blockingAgentClass]bool) []string {
	// "Reassign or archive them" must not appear to cover the product-owned
	// blockers listed beside them, which is exactly the instruction they cannot
	// follow. When both are present the clause names which ones it applies to.
	mixed := classes[blockingAgentMika] ||
		classes[blockingAgentBuilderCarrier] ||
		classes[blockingAgentOtherSystem]

	var out []string
	if classes[blockingAgentUser] && mixed {
		out = append(out, "The agents above that are not marked as built into Multica can be reassigned or archived.")
	} else if classes[blockingAgentUser] {
		out = append(out, "Reassign or archive them first.")
	}
	if classes[blockingAgentBuilderCarrier] {
		out = append(out, "The unfinished Agent Builder session(s) here are hidden from the agent list — reopen the session to pick another runtime, or discard it, to release its runtime.")
	}
	if classes[blockingAgentMika] {
		out = append(out, "Mika is built into Multica: it cannot be archived, and there is no supported way to move it to another runtime yet, so this cannot be cleared from here while Mika is bound.")
	}
	if classes[blockingAgentOtherSystem] {
		out = append(out, "Some blockers are agents built into Multica and cannot be archived.")
	}
	return out
}

// blockingAgentClassesFromAgents collects the classes present on a plain agent
// set, for the callers that already hold db.Agent rows.
func blockingAgentClassesFromAgents(agents []db.Agent) map[blockingAgentClass]bool {
	classes := make(map[blockingAgentClass]bool, 2)
	for _, a := range agents {
		classes[classifyBlockingAgent(a.SystemKey)] = true
	}
	return classes
}
