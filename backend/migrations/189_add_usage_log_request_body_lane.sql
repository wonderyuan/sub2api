ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_body_lane VARCHAR(16);

COMMENT ON COLUMN usage_logs.request_body_lane IS
    'Request-body admission lane snapshot: normal, heavy, or recovery';
