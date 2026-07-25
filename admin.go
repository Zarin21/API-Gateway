package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// adminToken is the shared secret required on every /admin request, via
// the X-Admin-Token header. This is a completely separate credential
// space from client API keys: "operating the gateway" and "being a
// client proxied through it" are different trust levels and shouldn't
// share a mechanism.
var adminToken string

// registerAdminRoutes mounts the admin API under /admin, protected end
// to end by adminAuthMiddleware.
func registerAdminRoutes(r chi.Router) {
	r.Route("/admin", func(admin chi.Router) {
		// corsMiddleware must run before adminAuthMiddleware: browsers send
		// preflight OPTIONS requests with no X-Admin-Token, so the token
		// check would reject them before CORS ever got a chance to answer.
		admin.Use(corsMiddleware, adminAuthMiddleware)

		admin.Route("/routes", func(routesGroup chi.Router) {
			routesGroup.Get("/", listRoutesHandler)
			routesGroup.Post("/", createRouteHandler)
			routesGroup.Route("/{id}", func(routeGroup chi.Router) {
				routeGroup.Put("/", updateRouteHandler)
				routeGroup.Delete("/", deleteRouteHandler)
			})
		})

		registerClientRoutes(admin)
	})
}

// corsMiddleware allows the dashboard's origin (a different port, hence a
// different origin per browser rules) to call the admin API. It answers
// preflight OPTIONS requests directly rather than passing them on, since
// those never carry the app's own headers/body.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", dashboardOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// adminAuthMiddleware requires a valid X-Admin-Token header matching the
// server's configured admin secret. Uses a constant-time comparison so
// the check doesn't leak how many leading bytes of a guess were correct
// via response timing.
func adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// routeRecord mirrors a row in the routes table for JSON responses.
type routeRecord struct {
	ID         int       `json:"id"`
	PathPrefix string    `json:"path_prefix"`
	BackendURL string    `json:"backend_url"`
	CreatedAt  time.Time `json:"created_at"`
}

func listRoutesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(),
		`SELECT id, path_prefix, backend_url, created_at FROM routes ORDER BY id`)
	if err != nil {
		http.Error(w, "failed to list routes", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := []routeRecord{}
	for rows.Next() {
		var rec routeRecord
		if err := rows.Scan(&rec.ID, &rec.PathPrefix, &rec.BackendURL, &rec.CreatedAt); err != nil {
			http.Error(w, "failed to read routes", http.StatusInternalServerError)
			return
		}
		records = append(records, rec)
	}

	writeJSON(w, http.StatusOK, records)
}

func createRouteHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PathPrefix string `json:"path_prefix"`
		BackendURL string `json:"backend_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.PathPrefix == "" || input.BackendURL == "" {
		http.Error(w, "path_prefix and backend_url are required", http.StatusBadRequest)
		return
	}

	var rec routeRecord
	err := db.QueryRow(r.Context(),
		`INSERT INTO routes (path_prefix, backend_url) VALUES ($1, $2)
		 RETURNING id, path_prefix, backend_url, created_at`,
		input.PathPrefix, input.BackendURL,
	).Scan(&rec.ID, &rec.PathPrefix, &rec.BackendURL, &rec.CreatedAt)
	if err != nil {
		http.Error(w, "failed to create route (path_prefix may already exist)", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func updateRouteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		PathPrefix string `json:"path_prefix"`
		BackendURL string `json:"backend_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.PathPrefix == "" || input.BackendURL == "" {
		http.Error(w, "path_prefix and backend_url are required", http.StatusBadRequest)
		return
	}

	var rec routeRecord
	err := db.QueryRow(r.Context(),
		`UPDATE routes SET path_prefix = $1, backend_url = $2 WHERE id = $3
		 RETURNING id, path_prefix, backend_url, created_at`,
		input.PathPrefix, input.BackendURL, id,
	).Scan(&rec.ID, &rec.PathPrefix, &rec.BackendURL, &rec.CreatedAt)
	if err != nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func deleteRouteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := db.Exec(r.Context(), `DELETE FROM routes WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "failed to delete route", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
