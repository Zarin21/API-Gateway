ALTER TABLE clients ADD COLUMN rate_limit_per_minute INTEGER NOT NULL DEFAULT 60;

-- Lower the seeded test client's limit so hitting it is easy to
-- demonstrate without firing 60 requests.
UPDATE clients SET rate_limit_per_minute = 3 WHERE id = 1;
