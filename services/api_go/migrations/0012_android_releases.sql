CREATE TABLE android_release_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    payload TEXT NOT NULL
);

CREATE TABLE android_release_apks (
    id TEXT PRIMARY KEY,
    metadata TEXT NOT NULL,
    published_at INTEGER
);
