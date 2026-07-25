package main

import (
	"sync"
	"time"
)

const (
	circuitFailureThreshold = 3
	circuitOpenDuration     = 5 * time.Second
)

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

// circuitBreaker tracks one backend's health. This is deliberately
// in-memory, per gateway process, and not shared via Redis/Postgres:
// unlike routes or rate limits, "is this specific backend currently
// failing" is transient runtime state, not configuration.
type circuitBreaker struct {
	mu           sync.Mutex
	state        circuitState
	failureCount int
	openedAt     time.Time
}

// allow reports whether a request should be let through to the backend.
// It also performs the Open -> Half-Open transition once the cooldown has
// elapsed, letting exactly one trial request through; further requests
// are rejected until that trial resolves via recordSuccess/recordFailure.
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateOpen:
		if time.Since(cb.openedAt) >= circuitOpenDuration {
			cb.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		return false
	default:
		return true
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = stateClosed
	cb.failureCount = 0
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateHalfOpen {
		// The trial request failed — reopen immediately, don't wait for
		// the failure count to build back up.
		cb.state = stateOpen
		cb.openedAt = time.Now()
		return
	}

	cb.failureCount++
	if cb.failureCount >= circuitFailureThreshold {
		cb.state = stateOpen
		cb.openedAt = time.Now()
	}
}

var (
	circuitBreakersMu sync.Mutex
	circuitBreakers   = map[string]*circuitBreaker{}
)

// getCircuitBreaker returns the breaker for a backend URL, creating one
// on first use.
func getCircuitBreaker(backend string) *circuitBreaker {
	circuitBreakersMu.Lock()
	defer circuitBreakersMu.Unlock()

	cb, ok := circuitBreakers[backend]
	if !ok {
		cb = &circuitBreaker{}
		circuitBreakers[backend] = cb
	}
	return cb
}
