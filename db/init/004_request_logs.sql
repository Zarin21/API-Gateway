CREATE TABLE request_logs (
    id BIGSERIAL PRIMARY KEY,
    client_id INTEGER REFERENCES clients(id),
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    latency_ms INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- client_id is nullable: a request that never resolved a valid API key
-- (missing/invalid) still gets logged, just with no known client.
