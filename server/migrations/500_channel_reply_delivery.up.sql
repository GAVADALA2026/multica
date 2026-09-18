-- One row per agent reply delivered into a chat channel: who owns the reply,
-- what the provider already accepted, and how far the send got.
--
-- The streamed placeholder, the terminal answer and the failure notice are
-- produced by three different code paths, and in a multi-replica deployment by
-- three different processes — the text report and the completion callback are
-- independent HTTP requests that land wherever the load balancer sends them.
-- An in-process map cannot make those agree, so the second process posts its
-- own copy of an answer the first one already sent (GH #8049, #7750).
-- This table is the shared owner: every path claims it before touching the
-- provider, and records the outcome against it.
CREATE TABLE IF NOT EXISTS channel_reply_delivery (
    task_id UUID NOT NULL,
    binding_id UUID NOT NULL,
    installation_id UUID NOT NULL,
    channel_type TEXT NOT NULL,
    chat_id TEXT NOT NULL,
    -- streaming: the placeholder is the live reply and may still be edited.
    -- terminal:  the final answer has taken over; no new placeholder may open.
    -- settled:   delivery is over; nothing may send or edit for this task.
    -- awaiting_retry: the attempt failed and will be retried automatically —
    --            the next task of the same user turn inherits this placeholder
    --            instead of opening a second one.
    phase TEXT NOT NULL CHECK (phase IN ('streaming', 'terminal', 'settled', 'awaiting_retry')),
    -- none:      nothing was handed to the provider yet.
    -- in_flight: a send is out; the provider may already have accepted it.
    -- known:     accepted, and message_id below identifies it.
    -- unknown:   accepted or not — the response was lost. Telegram's
    --            sendMessage has no caller-supplied idempotency key, so
    --            re-sending here is the duplicate we are fixing. Delivery
    --            stops and the row keeps the evidence instead.
    send_state TEXT NOT NULL CHECK (send_state IN ('none', 'in_flight', 'known', 'unknown')),
    message_id TEXT NOT NULL DEFAULT '',
    chunks_sent INTEGER NOT NULL DEFAULT 0 CHECK (chunks_sent >= 0),
    settled_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
