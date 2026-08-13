CREATE TABLE runtime_settings (
    name TEXT PRIMARY KEY,
    encrypted_value BLOB NOT NULL,
    updated_at INTEGER NOT NULL,
    updated_by TEXT REFERENCES users(id)
);
