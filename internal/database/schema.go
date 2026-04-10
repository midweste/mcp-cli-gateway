package database

// SQL schema constants for the gateway SQLite database.

const SchemaSQL = `
CREATE TABLE IF NOT EXISTS requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model TEXT NOT NULL,
    status TEXT NOT NULL,
    label TEXT,
    prompt_hash TEXT NOT NULL,
    prompt_text TEXT,
    pid INTEGER,
    cwd TEXT NOT NULL,
    created_at REAL NOT NULL,
    started_at REAL,
    finished_at REAL,
    exit_code INTEGER,
    retry_count INTEGER NOT NULL DEFAULT 0,
    error TEXT,
    tokens_in INTEGER,
    tokens_out INTEGER,
    tokens_cached INTEGER,
    tokens_thoughts INTEGER,
    tool_calls INTEGER,
    api_latency_ms INTEGER,
    batch_id TEXT,
    response_text TEXT
);

CREATE INDEX IF NOT EXISTS idx_requests_active
    ON requests(model, status)
    WHERE status IN ('waiting', 'running', 'retrying');

CREATE TABLE IF NOT EXISTS pacing (
    model TEXT PRIMARY KEY,
    min_gap_ms INTEGER NOT NULL,
    last_request_at REAL NOT NULL DEFAULT 0,
    backoff_ms INTEGER NOT NULL DEFAULT 0,
    consecutive_ok INTEGER NOT NULL DEFAULT 0,
    total_ok INTEGER NOT NULL DEFAULT 0,
    total_rate_limited INTEGER NOT NULL DEFAULT 0
);
`
