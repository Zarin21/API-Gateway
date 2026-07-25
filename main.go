package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"  // reverse proxy
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool" // pgx connection pool
	"github.com/redis/go-redis/v9"
)

var db *pgxpool.Pool
var rdb *redis.Client

// dashboardOrigin is the one browser origin allowed to open the live
// traffic WebSocket and call CORS-protected admin endpoints — the Vite
// dev server's default address unless overridden.
var dashboardOrigin string

// trafficChannel is the Redis pub/sub channel loggingMiddleware publishes
// every request's outcome to, and the WebSocket handler subscribes to.
const trafficChannel = "gateway:traffic"

// authInfo is what authMiddleware learns about the caller. loggingMiddleware
// (the outermost wrapper) creates one empty authInfo per request and puts a
// *pointer* to it in context; authMiddleware fills in the fields through
// that same pointer. A plain value wouldn't work here: context.WithValue
// creates a new, layered context that's only visible to handlers called
// after it, not to the outer middleware's own request variable once
// next.ServeHTTP returns. Mutating through a shared pointer is what lets
// loggingMiddleware see the resolved client even though auth runs "inside"
// it.
type authInfo struct {
	clientID  int
	rateLimit int
	resolved  bool // true once authMiddleware has identified a valid client
}

type ctxKey int

const ctxKeyAuth ctxKey = iota

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("connecting to postgres: %v", err)
	}
	defer pool.Close()
	db = pool

	rdb = redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("connecting to redis: %v", err)
	}
	defer rdb.Close()

	adminToken = os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN must be set")
	}

	dashboardOrigin = os.Getenv("DASHBOARD_ORIGIN")
	if dashboardOrigin == "" {
		dashboardOrigin = "http://localhost:5173"
	}

	r := chi.NewRouter() // creates a new router

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("gateway alive"))
	})

	r.Get("/admin/ws", websocketHandler)

	registerAdminRoutes(r)

	r.With(loggingMiddleware, authMiddleware, rateLimitMiddleware).Handle("/*", http.HandlerFunc(proxyHandler))

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// proxyHandler looks up which backend owns this request's path, then
// forwards the request there unchanged — unless that backend's circuit
// breaker is open, in which case it fails fast without contacting it.
func proxyHandler(w http.ResponseWriter, r *http.Request) {
	backend, err := lookupBackend(r.Context(), r.URL.Path)
	if err != nil {
		log.Printf("route lookup failed for %s: %v", r.URL.Path, err)
		http.Error(w, "no route for this path", http.StatusBadGateway)
		return
	}

	cb := getCircuitBreaker(backend.String())
	if !cb.allow() {
		http.Error(w, "backend unavailable (circuit open)", http.StatusServiceUnavailable)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(backend)

	// Wrapped so we can read back the status actually written, whether it
	// came from the backend itself or from ReverseProxy's default error
	// handler (which turns connection failures/timeouts into a 502).
	ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
	proxy.ServeHTTP(ww, r)

	if ww.Status() >= http.StatusInternalServerError {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

// authMiddleware rejects any request that doesn't carry a valid,
// non-revoked API key in the X-API-Key header, and fills in the shared
// authInfo (put in context by loggingMiddleware) with the resolved
// client's id and rate limit.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			http.Error(w, "missing X-API-Key header", http.StatusUnauthorized)
			return
		}

		info := r.Context().Value(ctxKeyAuth).(*authInfo)
		err := db.QueryRow(r.Context(),
			`SELECT c.id, c.rate_limit_per_minute
			 FROM api_keys k
			 JOIN clients c ON c.id = k.client_id
			 WHERE k.key_hash = $1 AND k.revoked_at IS NULL`,
			hashKey(key),
		).Scan(&info.clientID, &info.rateLimit)
		if err != nil {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}
		info.resolved = true

		next.ServeHTTP(w, r)
	})
}

// hashKey returns the hex-encoded SHA-256 hash of key, matching how keys
// are stored in the api_keys table.
func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// loggingMiddleware is the outermost wrapper in the chain, so it sees
// every request that reaches the gateway, including ones auth or rate
// limiting later reject. It creates the shared authInfo for this request,
// times the whole chain, captures the status code actually written (via
// chi's response writer wrapper, since the standard http.ResponseWriter
// doesn't expose that after the fact), and logs the outcome.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		info := &authInfo{}
		r = r.WithContext(context.WithValue(r.Context(), ctxKeyAuth, info))

		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		logRequest(requestLog{
			clientID:   info.clientID,
			hasClient:  info.resolved,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: ww.Status(),
			LatencyMs:  time.Since(start).Milliseconds(),
		})
	})
}

// requestLog is one row destined for the request_logs table, and also
// the payload broadcast to the live traffic dashboard.
type requestLog struct {
	ClientID   *int   `json:"client_id"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Timestamp  string `json:"timestamp"`

	clientID  int
	hasClient bool
}

// logRequest writes entry to Postgres and publishes it to Redis, both in
// the background so the gateway's response doesn't wait on either. It
// deliberately uses context.Background() rather than the original
// request's context: by the time this goroutine runs, the response has
// already been sent and the request's context may already be canceled.
func logRequest(entry requestLog) {
	go func() {
		var clientID any
		if entry.hasClient {
			clientID = entry.clientID
			entry.ClientID = &entry.clientID
		}
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)

		if payload, err := json.Marshal(entry); err != nil {
			log.Printf("failed to marshal traffic event: %v", err)
		} else if err := rdb.Publish(context.Background(), trafficChannel, payload).Err(); err != nil {
			log.Printf("failed to publish traffic event: %v", err)
		}

		_, err := db.Exec(context.Background(),
			`INSERT INTO request_logs (client_id, method, path, status_code, latency_ms)
			 VALUES ($1, $2, $3, $4, $5)`,
			clientID, entry.Method, entry.Path, entry.StatusCode, entry.LatencyMs,
		)
		if err != nil {
			log.Printf("failed to write request log: %v", err)
		}
	}()
}

// rateLimitMiddleware enforces the caller's per-minute request budget
// using a token bucket stored in Redis. Must run after authMiddleware,
// which populates the authInfo this reads from the request context.
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := r.Context().Value(ctxKeyAuth).(*authInfo)

		allowed, err := checkRateLimit(r.Context(), info.clientID, info.rateLimit)
		if err != nil {
			log.Printf("rate limit check failed for client %d: %v", info.clientID, err)
			http.Error(w, "rate limit check failed", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// tokenBucketScript atomically refills a client's bucket based on time
// elapsed since its last request, then consumes one token if available.
// Runs inside Redis via EVAL so concurrent requests from the same client
// can't both read the same token count and both be let through.
const tokenBucketScript = `
local tokens_key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local bucket = redis.call('HMGET', tokens_key, 'tokens', 'ts')
local tokens = tonumber(bucket[1])
local ts = tonumber(bucket[2])

if tokens == nil then
	tokens = capacity
	ts = now
end

local elapsed = math.max(0, now - ts)
tokens = math.min(capacity, tokens + elapsed * refill_rate)

local allowed = 0
if tokens >= 1 then
	allowed = 1
	tokens = tokens - 1
end

redis.call('HMSET', tokens_key, 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', tokens_key, 60)

return allowed
`

// checkRateLimit consumes one token from clientID's bucket, sized to
// limitPerMinute requests with a matching per-second refill rate.
// Returns false once the bucket is empty.
func checkRateLimit(ctx context.Context, clientID, limitPerMinute int) (bool, error) {
	key := fmt.Sprintf("ratelimit:%d", clientID)
	now := float64(time.Now().UnixNano()) / 1e9
	refillRate := float64(limitPerMinute) / 60.0

	result, err := rdb.Eval(ctx, tokenBucketScript, []string{key}, limitPerMinute, refillRate, now).Result()
	if err != nil {
		return false, err
	}

	allowed, _ := result.(int64)
	return allowed == 1, nil
}

// lookupBackend finds the most specific route whose path_prefix matches
// path ("/" always matches as the fallback) and returns its backend_url.
func lookupBackend(ctx context.Context, path string) (*url.URL, error) {
	var rawURL string
	err := db.QueryRow(ctx,
		`SELECT backend_url FROM routes
		 WHERE path_prefix = '/'
		    OR path_prefix = $1
		    OR $1 LIKE path_prefix || '/%'
		 ORDER BY length(path_prefix) DESC
		 LIMIT 1`,
		path,
	).Scan(&rawURL)
	if err != nil {
		return nil, err
	}
	return url.Parse(rawURL)
}
