CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    disabled      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS samples (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    sample_id       TEXT    NOT NULL UNIQUE,
    recipient_email TEXT    NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid               TEXT    NOT NULL UNIQUE,
    sample_id          TEXT    NOT NULL,
    user_id            INTEGER NOT NULL,
    status             TEXT    NOT NULL,
    encrypted_path     TEXT,
    password_encrypted BLOB,
    created_at         TEXT    NOT NULL,
    sent_at            TEXT,
    expires_at         TEXT    NOT NULL,
    FOREIGN KEY (sample_id) REFERENCES samples (sample_id),
    FOREIGN KEY (user_id)   REFERENCES users (id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_uuid       ON jobs (uuid);
CREATE INDEX IF NOT EXISTS idx_jobs_user       ON jobs (user_id);
CREATE INDEX IF NOT EXISTS idx_jobs_expires    ON jobs (expires_at);

CREATE TABLE IF NOT EXISTS audit_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT    NOT NULL,
    user_id   INTEGER,
    action    TEXT    NOT NULL,
    sample_id TEXT,
    detail    TEXT
);
