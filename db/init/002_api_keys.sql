CREATE TABLE clients (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id SERIAL PRIMARY KEY,
    client_id INTEGER NOT NULL REFERENCES clients(id),
    key_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

-- Seed one test client + key so auth can be exercised before the admin
-- API (step 8) exists to create these properly.
-- Plaintext key: test-key-123 (hash below is sha256("test-key-123")).
INSERT INTO clients (name) VALUES ('test-client');
INSERT INTO api_keys (client_id, key_hash) VALUES
    (1, '625faa3fbbc3d2bd9d6ee7678d04cc5339cb33dc68d9b58451853d60046e226a');
