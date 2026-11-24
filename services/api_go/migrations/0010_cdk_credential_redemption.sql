ALTER TABLE login_cdks ADD COLUMN code_redeemed_at INTEGER;

UPDATE login_cdks SET code_redeemed_at = redeemed_at WHERE redeemed_at IS NOT NULL;
