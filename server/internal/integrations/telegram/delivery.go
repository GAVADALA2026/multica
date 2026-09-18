package telegram

// delivery.go — reply-delivery ownership.
//
// A task's reply reaches Telegram through three paths: the streamed
// placeholder, the final answer, and the failure notice. In a multi-replica
// deployment they also run in three different processes — the daemon's
// transcript report and its completion callback are independent HTTP requests
// that land wherever the load balancer sends them, and the event bus is
// in-process. A process that cannot see the placeholder posts its own copy of
// the same answer, which is the duplicate users report (GH #8049, #7750).
//
// channel_reply_delivery is the shared owner. Every path claims it before
// touching Telegram and records the outcome against it, so "has this reply
// already been sent, and as which message?" has one answer for all of them.

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	deliveryPhaseStreaming = "streaming"
	deliveryPhaseTerminal  = "terminal"
	deliveryPhaseSettled   = "settled"

	deliverySendNone     = "none"
	deliverySendInFlight = "in_flight"
	deliverySendKnown    = "known"
	deliverySendUnknown  = "unknown"
)

// maxDeliveryClaimAttempts bounds how long a reply waits for its ownership
// row. Availability wins after that: losing the answer entirely is worse for
// the user than the duplicate risk this table exists to remove, so delivery
// proceeds unowned and says so in the log.
const maxDeliveryClaimAttempts = 3

// deliveryMessageID is the Telegram message this reply owns, or 0 when the
// provider has not given one. An id is only usable once the send that produced
// it came back: an in-flight or lost send has no id to edit.
func deliveryMessageID(row db.ChannelReplyDelivery) int64 {
	if row.SendState != deliverySendKnown || row.MessageID == "" {
		return 0
	}
	id, err := strconv.ParseInt(row.MessageID, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// openStreamDelivery claims the streaming half of a task's reply and reports
// the ownership row a frame must respect. ok is false when the frame must not
// touch Telegram at all.
//
// tryAdopt asks whether this reply might be inheriting a previous attempt's
// placeholder. Only the first frame of a task needs to ask: adoption can only
// win before the task has a row of its own, and the query is an update with a
// subselect on a path that runs for every text frame of every reply.
func (o *Outbound) openStreamDelivery(ctx context.Context, target *replyTarget, tryAdopt bool) (db.ChannelReplyDelivery, bool) {
	if tryAdopt {
		// An automatic retry inherits the placeholder its previous attempt
		// left in the chat, so one user turn keeps one answer instead of
		// gaining a second beside it. Scoped to a row a retry actually parked:
		// two ordinary turns still get one reply each, even when they happen
		// to say the same thing.
		row, err := o.q.AdoptChannelReplyDeliveryForRetry(ctx, db.AdoptChannelReplyDeliveryForRetryParams{
			TaskID:    target.taskID,
			BindingID: target.bindingID,
		})
		if err == nil {
			return row, true
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			o.logger.WarnContext(ctx, "telegram outbound: retry placeholder lookup failed", "error", err)
			return db.ChannelReplyDelivery{}, false
		}
	}

	row, err := o.q.EnsureChannelReplyDelivery(ctx, db.EnsureChannelReplyDeliveryParams{
		TaskID:         target.taskID,
		BindingID:      target.bindingID,
		InstallationID: target.installationID,
		ChannelType:    string(TypeTelegram),
		ChatID:         target.channelChatID,
	})
	if err != nil {
		// Fail closed. A stream frame is decoration — the final answer still
		// lands — so skipping it costs nothing, while sending without knowing
		// whether a placeholder already exists is exactly the duplicate.
		o.logger.WarnContext(ctx, "telegram outbound: reply ownership unavailable; skipping stream frame", "error", err)
		return db.ChannelReplyDelivery{}, false
	}
	return row, true
}

// claimStreamSend takes the single placeholder send for this reply. Only one
// caller can win, across every process: the loser edits the winner's message
// once its id lands, or waits, but never opens a second one.
func (o *Outbound) claimStreamSend(ctx context.Context, taskID pgtype.UUID) bool {
	rows, err := o.q.MarkChannelReplyDeliverySending(ctx, taskID)
	if err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: placeholder claim failed", "error", err)
		return false
	}
	return rows == 1
}

// recordStreamSend writes what Telegram did with the placeholder.
//
// The distinction that matters is refused vs unknown. A parsed API rejection
// means nothing is in the chat and the reply may be attempted again. A
// transport failure means Telegram may well have posted the message and the
// response was lost on the way back — and sendMessage has no caller-supplied
// idempotency key, so a second attempt cannot be deduplicated by the provider.
// That case stops delivery and keeps the evidence instead of guessing.
func (o *Outbound) recordStreamSend(ctx context.Context, taskID pgtype.UUID, messageID int64, sendErr error) {
	var query func() (int64, error)
	switch {
	case sendErr == nil:
		query = func() (int64, error) {
			return o.q.RecordChannelReplyDeliveryMessage(ctx, db.RecordChannelReplyDeliveryMessageParams{
				TaskID:    taskID,
				MessageID: strconv.FormatInt(messageID, 10),
			})
		}
	case isDefiniteRejection(sendErr):
		query = func() (int64, error) { return o.q.ResetChannelReplyDeliverySend(ctx, taskID) }
	default:
		o.logger.WarnContext(ctx, "telegram outbound: placeholder send result unknown; delivery stops rather than risk a duplicate",
			"task_id", uuidText(taskID), "error", sendErr)
		query = func() (int64, error) { return o.q.MarkChannelReplyDeliverySendUnknown(ctx, taskID) }
	}
	if _, err := query(); err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: recording placeholder send failed", "error", err)
	}
}

// isDefiniteRejection reports whether Telegram answered and refused. Anything
// else — transport failure, timeout, an unreadable response — leaves the
// outcome genuinely unknown.
func isDefiniteRejection(err error) bool {
	var ae *apiError
	return errors.As(err, &ae)
}

// claimTerminalDelivery hands the reply to terminal delivery. It succeeds from
// any phase but settled, so an interrupted delivery resumes from the chunk it
// reached, while a second completion event for a reply that already finished
// gets nothing and is dropped.
func (o *Outbound) claimTerminalDelivery(ctx context.Context, target *replyTarget) (db.ChannelReplyDelivery, bool, error) {
	row, err := o.q.ClaimChannelReplyDeliveryTerminal(ctx, db.ClaimChannelReplyDeliveryTerminalParams{
		TaskID:         target.taskID,
		BindingID:      target.bindingID,
		InstallationID: target.installationID,
		ChannelType:    string(TypeTelegram),
		ChatID:         target.channelChatID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ChannelReplyDelivery{}, false, nil
	}
	if err != nil {
		return db.ChannelReplyDelivery{}, false, err
	}
	return row, true, nil
}

// advanceDeliveryChunks records how much of a multi-part answer is in the chat,
// after each part rather than at the end, so a delivery resumed elsewhere
// continues after the last part instead of repeating it.
func (o *Outbound) advanceDeliveryChunks(ctx context.Context, taskID pgtype.UUID, sent int) {
	if _, err := o.q.AdvanceChannelReplyDeliveryChunks(ctx, db.AdvanceChannelReplyDeliveryChunksParams{
		TaskID:     taskID,
		ChunksSent: int32(sent),
	}); err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: recording chunk progress failed", "error", err)
	}
}

// settleDelivery closes a task's reply. Afterwards no path sends or edits for
// it — including a text frame that was still in flight when the task ended.
func (o *Outbound) settleDelivery(ctx context.Context, taskID pgtype.UUID, reason string) {
	if _, err := o.q.SettleChannelReplyDelivery(ctx, db.SettleChannelReplyDeliveryParams{
		TaskID:        taskID,
		SettledReason: reason,
	}); err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: settling reply delivery failed", "error", err, "reason", reason)
	}
}

// parkDeliveryForRetry keeps a failed attempt's placeholder owned so the
// automatic retry can finish it. Without this the retry runs under a new task
// id, finds nothing, and posts its answer beside the abandoned one.
func (o *Outbound) parkDeliveryForRetry(ctx context.Context, taskID pgtype.UUID) {
	if _, err := o.q.MarkChannelReplyDeliveryAwaitingRetry(ctx, taskID); err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: parking reply for retry failed", "error", err)
	}
}

func uuidText(id pgtype.UUID) string {
	text, err := id.Value()
	if err != nil || text == nil {
		return ""
	}
	s, _ := text.(string)
	return s
}
