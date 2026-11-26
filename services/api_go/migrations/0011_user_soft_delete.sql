ALTER TABLE users ADD COLUMN deleted_at INTEGER
    CHECK (deleted_at IS NULL OR (is_active = 0 AND is_cdk = 0));
