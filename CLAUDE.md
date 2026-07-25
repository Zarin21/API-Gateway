# Personal API Gateway

A reverse proxy that sits in front of backend services and handles
authentication (API keys), rate limiting, routing, request logging, and
(later) circuit breaking. A live dashboard shows traffic in real time.

## Stack

- **Go** (`net/http` + chi router) — gateway and admin API
- **PostgreSQL** — system of record: routes, API keys, clients, request logs
- **Redis** — rate limit counters; later pub/sub for the live dashboard
- **React + Vite** — admin dashboard, on a different origin (CORS is in scope)
- **Docker Compose** — gateway + postgres + redis + 2 mock backends

## Architecture rules

- The gateway is a reverse proxy: it receives requests, runs them through a
  middleware chain (auth → rate limit → route match → forward), and relays
  the response.
- The admin API is a separate set of routes on the same Go server, prefixed
  `/admin`.
- All config (routes, keys, limits) lives in Postgres, not config files.
- Rate limit counters live in Redis.
- The dashboard connects via WebSocket for live traffic streaming.

## How to work with me

- Always explain what you're about to build and why **before** writing any
  code.
- Build one piece at a time — never scaffold the entire project in one shot.
- After each piece, tell me what we just built, what it does, and what's
  next.
- I'm learning architecture — treat every decision as a teaching moment.
- Ask me before making any major design choice.

## Build order

Follow this sequence; don't jump ahead.

1. Go module + basic HTTP server + chi router (hello world)
2. Reverse proxy to a single hardcoded backend
3. Docker Compose: gateway + one mock backend
4. PostgreSQL + route table + dynamic routing from DB
5. API key table + auth middleware
6. Redis + rate limit middleware
7. Request logging to Postgres
8. Admin API (CRUD for routes, keys, limits)
9. WebSocket endpoint for live traffic
10. React dashboard (Vite, on different port — CORS middleware)
11. Circuit breaker (stretch goal)
