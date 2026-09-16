package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// issueSnapshotVersion is the shape version of the JSON stored in
// agent_task_queue.issue_snapshot. Bump it whenever the compared field SET
// changes — never when only a value's formatting changes, because a stored
// snapshot is compared against a freshly built one and a formatting drift
// inside the same version would read as a spurious "changed".
//
// A snapshot carrying any other version is treated as UNKNOWN rather than
// coerced: the two runs did not compare the same things, so neither
// "unchanged" nor a changed-field list would be true. Unknown degrades to the
// unconditional issue read, which is the behaviour that predates this column.
//
// This is version 1 — the first shape the column has ever held. The set below
// was narrowed during review, before any of this shipped, so no stored row has
// ever carried a wider one and there is nothing for a bump to protect. The
// version earns its place from here on, not retroactively.
const issueSnapshotVersion = 1

// Compared field names, in the fixed order they are reported.
//
// The set answers exactly one question — "must the agent run `issue get`
// again?" — so a field earns a place here only if changing it would alter what
// the agent does AND the per-turn message does not already carry its current
// value:
//
//   - title, description: the task itself, and reachable only by reading the
//     issue. Both must be compared.
//   - status: also shipped as a current value, so comparing it is strictly
//     redundant for "what is it now". It stays because "changed: status" is the
//     clearest available signal that somebody intervened between the runs — a
//     push back from in_review to todo means the delivery was rejected — and
//     carrying it costs nothing.
//   - assignee: the agent only needs "is this mine now", which the current
//     value answers outright. Not compared; still shipped.
//   - priority: reachable only by reading, but it does not change what the
//     agent does. Not compared.
//
// The agent is told exactly this set was compared, so a field absent here must
// never be implied to have been checked: assignee, priority, labels, parent,
// due date, stage, project and metadata are all out of scope, and an issue
// whose ONLY change is one of them is reported as unchanged.
const (
	issueFieldTitle       = "title"
	issueFieldDescription = "description"
	issueFieldStatus      = "status"
)

// issueStateSnapshot is the comparison key for one claim's view of an issue.
//
// Title and description are stored as SHA-256 hex, not as text: the column
// exists to answer "did this move", and a second copy of every issue body in
// the task queue would be both a storage cost and a place for issue text to
// leak from. Status is a short key, so it is stored raw.
//
// The claim's CURRENT status and assignee reach the agent as their own response
// fields, read straight off the issue row — they are not sourced from here, and
// narrowing this struct does not affect them.
type issueStateSnapshot struct {
	Version           int    `json:"v"`
	Status            string `json:"status"`
	TitleSHA256       string `json:"title_sha256"`
	DescriptionSHA256 string `json:"description_sha256"`
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// buildIssueStateSnapshot captures the compared fields of an issue as of now.
func buildIssueStateSnapshot(issue db.Issue) issueStateSnapshot {
	return issueStateSnapshot{
		Version:           issueSnapshotVersion,
		Status:            issue.Status,
		TitleSHA256:       sha256Hex(issue.Title),
		DescriptionSHA256: sha256Hex(issue.Description.String),
	}
}

// changedFieldsSince reports which compared fields differ from prev, in the
// fixed order above. An empty result means every compared field matched — and
// says nothing about the fields outside the set.
func (s issueStateSnapshot) changedFieldsSince(prev issueStateSnapshot) []string {
	var changed []string
	if s.TitleSHA256 != prev.TitleSHA256 {
		changed = append(changed, issueFieldTitle)
	}
	if s.DescriptionSHA256 != prev.DescriptionSHA256 {
		changed = append(changed, issueFieldDescription)
	}
	if s.Status != prev.Status {
		changed = append(changed, issueFieldStatus)
	}
	return changed
}

// decodeIssueStateSnapshot parses a stored snapshot. It returns ok=false for
// every state that means "this claim cannot say what changed": no prior run, a
// row written before the column existed, a snapshot whose write lost its CAS,
// malformed JSON, or a different shape version. Callers must map ok=false to
// "not compared" and never to "unchanged" — the whole point of the flag is that
// those two are not the same answer, and only one of them may replace a read.
func decodeIssueStateSnapshot(raw []byte) (issueStateSnapshot, bool) {
	if len(raw) == 0 {
		return issueStateSnapshot{}, false
	}
	var snap issueStateSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return issueStateSnapshot{}, false
	}
	if snap.Version != issueSnapshotVersion {
		return issueStateSnapshot{}, false
	}
	return snap, true
}

// taskStatusCompleted is the only terminal status that PROVES a run delivered
// its prompt to the provider. The distinction matters because the row is a
// delta anchor, not just a session pointer — see resumedRunAnchor.
const taskStatusCompleted = "completed"

// resumedRunAnchor is the prior run whose provider session THIS claim hands
// back, and therefore the only run either of a claim's two deltas may be
// measured from.
//
// The distinction that makes this type necessary: "the run that started most
// recently" and "the run whose session we resume" are not the same row.
// GetLastTaskSession skips poisoned and retired sessions, and a manual rerun
// resumes an operator-chosen source, so both legitimately hand back an OLDER
// run. Measuring a delta against the newest run while resuming an older one
// reports "unchanged" to an agent whose resumed memory predates the change —
// the one failure mode this whole mechanism exists to avoid, and one with no
// symptom at runtime (MUL-7344, found in review).
//
// A nil *resumedRunAnchor means this claim resumes nothing it can date, so
// neither delta is computed and the daemon falls back to the reads it has
// always performed.
type resumedRunAnchor struct {
	// StartedAt dates the comment delta.
	StartedAt pgtype.Timestamptz
	// IssueSnapshot is the issue state that run was handed at ITS claim. Empty
	// for a run that predates the column.
	IssueSnapshot []byte
}

// newResumedRunAnchor returns the anchor for a resume source, or nil when that
// source cannot date a delta.
//
// A snapshot is written at CLAIM time, so its existence proves only that the
// server built a payload — not that the agent ever saw it. Between those two
// points the daemon writes started_at (TaskService.StartTask) and only then
// launches the provider, so a run can carry a snapshot AND a started_at and
// still have died before the prompt reached the session: the daemon exits
// during prepare, the runtime drops, and orphan recovery marks the row failed.
// Its snapshot then describes an issue that session never saw, and comparing
// against it reports "unchanged" across a real edit — plus a comment anchor
// that hides every comment older than it.
//
// Nothing on the row distinguishes "failed after the provider ran" from "failed
// before it ran": session_id is inherited by CreateRetryTask at insert, and
// started_at precedes the launch. The only terminal status that PROVES delivery
// is 'completed' — a run cannot finish a turn it never started.
//
// So failed and cancelled sources date nothing. Their SESSION is still resumed
// (GetLastTaskSession accepts them on purpose, and that behaviour is unchanged);
// the run simply performs the context reads it always did. That is the cheap
// side of the trade: those paths are the ones already recovering from a
// problem, and the optimization's main case — an ordinary follow-up after a
// healthy run — is untouched.
func newResumedRunAnchor(status string, startedAt pgtype.Timestamptz, snapshot []byte) *resumedRunAnchor {
	if status != taskStatusCompleted || !startedAt.Valid {
		return nil
	}
	return &resumedRunAnchor{StartedAt: startedAt, IssueSnapshot: snapshot}
}

// commentCountScope carries the issue/workspace/trigger identity the comment
// delta needs, captured while the trigger comment is loaded and consumed once
// the resume anchor is known.
type commentCountScope struct {
	AnchorID    pgtype.UUID
	IssueID     pgtype.UUID
	WorkspaceID pgtype.UUID
}
