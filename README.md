# API Gateway

A reverse proxy gateway written from scratch in Go: authentication, per-client
rate limiting, dynamic routing, request logging, and a circuit breaker per
backend — all configured through Postgres and Redis instead of static config
files, with a React dashboard that streams traffic live.

## What it does

A request hits the gateway and passes through a middleware chain:

```
auth -> rate limit -> route match -> forward -> log
```

- **Auth** — every request needs a valid `X-API-Key` header. Keys are
  generated server-side and stored only as a SHA-256 hash.
- **Rate limiting** — each client has a per-minute budget enforced with a
  token bucket stored in Redis, refilling continuously rather than resetting
  all at once.
- **Routing** — which backend a request goes to is a row in Postgres
  (`routes`), not a hardcoded value. Changing it takes effect on the very
  next request, no redeploy.
- **Circuit breaking** — if a backend starts failing, its circuit opens and
  the gateway fails fast instead of hammering it, then automatically probes
  it and recovers once it's healthy again.
- **Logging** — every request (allowed, rejected, or rate-limited) is
  written to Postgres and published to Redis pub/sub, which the dashboard
  subscribes to over WebSocket for a live traffic feed.
- **Admin API** — a separate, separately-authenticated set of routes under
  `/admin` for managing routes, clients, and API keys via CRUD, so the
  gateway can be reconfigured over HTTP instead of raw SQL.

## Stack

| Piece | Role |
|---|---|
| Go (`net/http` + [chi](https://github.com/go-chi/chi)) | Gateway + admin API |
| PostgreSQL | System of record: routes, clients, API keys, request logs |
| Redis | Rate limit counters, live traffic pub/sub |
| React + Vite | Admin dashboard (separate origin, CORS-protected) |
| Docker Compose | Gateway + Postgres + Redis + two mock backends |

## Project layout

```
main.go              gateway core: proxy, auth, rate limit, logging middleware
admin.go              admin API routing, auth, CORS, routes CRUD
admin_clients.go       clients + API key CRUD
circuitbreaker.go      per-backend circuit breaker
websocket.go            live traffic WebSocket endpoint
db/init/                Postgres schema + seed data (auto-run on first startup)
dashboard/              React + Vite admin dashboard
docker-compose.yml       gateway + postgres + redis + two mock backends
```

## Running it

**1. Configure secrets**

```
cp .env.example .env
```

Edit `.env` and set `ADMIN_TOKEN` to a real random value (this protects the
`/admin` API). The example value is a placeholder, not something to use
as-is.

**2. Start the backend**

```
docker compose up -d --build
```

This brings up the gateway (`:8080`), Postgres, Redis, and two mock
backends. On first startup, Postgres runs the SQL files in `db/init/`,
which create the schema and seed one test client:

- API key `test-key-123` (client `test-client`, rate limit 3/min) — enough
  to try the gateway immediately:

  ```
  curl -H "X-API-Key: test-key-123" http://localhost:8080/
  ```

**3. Start the dashboard**

```
cd dashboard
npm install
npm run dev
```

Open `http://localhost:5173` and enter the `ADMIN_TOKEN` from your `.env`
to connect. You'll see current routes and clients, plus a live traffic feed
that updates in real time as requests hit the gateway.

## Admin API

All endpoints below require an `X-Admin-Token` header matching `ADMIN_TOKEN`.

```
GET    /admin/routes/
POST   /admin/routes/
PUT    /admin/routes/{id}
DELETE /admin/routes/{id}

GET    /admin/clients/
POST   /admin/clients/
PUT    /admin/clients/{id}
DELETE /admin/clients/{id}

GET    /admin/clients/{id}/keys/
POST   /admin/clients/{id}/keys/        # returns the plaintext key once
DELETE /admin/clients/{id}/keys/{keyID} # revokes, doesn't delete
```

`GET /admin/ws?token=<ADMIN_TOKEN>` is the live traffic WebSocket (token as
a query param, since browsers can't set custom headers on a WS handshake).

## Notes

- The `ADMIN_TOKEN` in `.env` is fine for local development but isn't real
  secret management — don't reuse this setup as-is for anything deployed
  publicly.
- Rate limits and routes are just Postgres rows — `psql` into the `gateway`
  database (`postgres://gateway:gateway@localhost:5432/gateway`) works too
  if you want to inspect or edit them directly.
