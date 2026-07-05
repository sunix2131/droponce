PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;

CREATE TABLE IF NOT EXISTS transfers (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    source_file_name TEXT NOT NULL,
    source_path TEXT,
    source_size_bytes INTEGER NOT NULL,
    source_modified_at TEXT NOT NULL,
    source_sha256 TEXT,
    bind_ip TEXT,
    port INTEGER,
    token_hash TEXT NOT NULL,
    max_downloads INTEGER NOT NULL,
    completed_downloads INTEGER NOT NULL DEFAULT 0,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    activated_at TEXT,
    completed_at TEXT,
    cancelled_at TEXT,
    stopped_at TEXT,
    last_error_code TEXT,
    last_error_message TEXT
);

CREATE TABLE IF NOT EXISTS download_attempts (
    id TEXT PRIMARY KEY,
    transfer_id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    bytes_sent INTEGER NOT NULL DEFAULT 0,
    error_code TEXT,
    error_message TEXT,
    FOREIGN KEY (transfer_id) REFERENCES transfers(id)
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_expires_at ON transfers(expires_at);
CREATE INDEX IF NOT EXISTS idx_transfers_token_hash ON transfers(token_hash);
CREATE INDEX IF NOT EXISTS idx_download_attempts_transfer_id ON download_attempts(transfer_id);
CREATE INDEX IF NOT EXISTS idx_download_attempts_status ON download_attempts(status);
