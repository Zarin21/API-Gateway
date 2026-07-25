CREATE TABLE routes (
    id SERIAL PRIMARY KEY,
    path_prefix TEXT NOT NULL UNIQUE,
    backend_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed data: "/" is the catch-all default route, "/v2" is a second
-- prefix pointing at a different backend, so we can prove routing
-- actually depends on the DB rather than always hitting one backend.
INSERT INTO routes (path_prefix, backend_url) VALUES
    ('/', 'http://backend:9001'),
    ('/v2', 'http://backend2:9002');
