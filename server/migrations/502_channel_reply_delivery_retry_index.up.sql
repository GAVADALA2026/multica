CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_channel_reply_delivery_retry ON channel_reply_delivery (binding_id, updated_at DESC) WHERE phase = 'awaiting_retry';
