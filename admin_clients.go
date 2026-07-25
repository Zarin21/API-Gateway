package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// registerClientRoutes mounts /admin/clients, including nested API key
// management at /admin/clients/{id}/keys. Called from registerAdminRoutes,
// already inside the adminAuthMiddleware-protected group.
func registerClientRoutes(admin chi.Router) {
	admin.Route("/clients", func(clients chi.Router) {
		clients.Get("/", listClientsHandler)
		clients.Post("/", createClientHandler)
		clients.Route("/{id}", func(client chi.Router) {
			client.Put("/", updateClientHandler)
			client.Delete("/", deleteClientHandler)

			client.Route("/keys", func(keys chi.Router) {
				keys.Get("/", listKeysHandler)
				keys.Post("/", createKeyHandler)
				keys.Delete("/{keyID}", revokeKeyHandler)
			})
		})
	})
}

type clientRecord struct {
	ID                 int       `json:"id"`
	Name               string    `json:"name"`
	RateLimitPerMinute int       `json:"rate_limit_per_minute"`
	CreatedAt          time.Time `json:"created_at"`
}

func listClientsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(r.Context(),
		`SELECT id, name, rate_limit_per_minute, created_at FROM clients ORDER BY id`)
	if err != nil {
		http.Error(w, "failed to list clients", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := []clientRecord{}
	for rows.Next() {
		var rec clientRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.RateLimitPerMinute, &rec.CreatedAt); err != nil {
			http.Error(w, "failed to read clients", http.StatusInternalServerError)
			return
		}
		records = append(records, rec)
	}

	writeJSON(w, http.StatusOK, records)
}

func createClientHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name               string `json:"name"`
		RateLimitPerMinute int    `json:"rate_limit_per_minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if input.RateLimitPerMinute <= 0 {
		input.RateLimitPerMinute = 60
	}

	var rec clientRecord
	err := db.QueryRow(r.Context(),
		`INSERT INTO clients (name, rate_limit_per_minute) VALUES ($1, $2)
		 RETURNING id, name, rate_limit_per_minute, created_at`,
		input.Name, input.RateLimitPerMinute,
	).Scan(&rec.ID, &rec.Name, &rec.RateLimitPerMinute, &rec.CreatedAt)
	if err != nil {
		http.Error(w, "failed to create client", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, rec)
}

func updateClientHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		Name               string `json:"name"`
		RateLimitPerMinute int    `json:"rate_limit_per_minute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Name == "" || input.RateLimitPerMinute <= 0 {
		http.Error(w, "name and a positive rate_limit_per_minute are required", http.StatusBadRequest)
		return
	}

	var rec clientRecord
	err := db.QueryRow(r.Context(),
		`UPDATE clients SET name = $1, rate_limit_per_minute = $2 WHERE id = $3
		 RETURNING id, name, rate_limit_per_minute, created_at`,
		input.Name, input.RateLimitPerMinute, id,
	).Scan(&rec.ID, &rec.Name, &rec.RateLimitPerMinute, &rec.CreatedAt)
	if err != nil {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func deleteClientHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := db.Exec(r.Context(), `DELETE FROM clients WHERE id = $1`, id)
	if err != nil {
		// Most likely a foreign key violation because this client still
		// has API keys pointing at it — a deliberate guard against
		// orphaning keys, not a bug.
		http.Error(w, "failed to delete client (it may still have API keys — revoke or delete those first)", http.StatusConflict)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// keyRecord is what we return for key listings — never the hash or
// plaintext, since neither should be retrievable after creation.
type keyRecord struct {
	ID        int        `json:"id"`
	ClientID  int        `json:"client_id"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

func listKeysHandler(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")

	rows, err := db.Query(r.Context(),
		`SELECT id, client_id, created_at, revoked_at FROM api_keys WHERE client_id = $1 ORDER BY id`,
		clientID,
	)
	if err != nil {
		http.Error(w, "failed to list keys", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := []keyRecord{}
	for rows.Next() {
		var rec keyRecord
		if err := rows.Scan(&rec.ID, &rec.ClientID, &rec.CreatedAt, &rec.RevokedAt); err != nil {
			http.Error(w, "failed to read keys", http.StatusInternalServerError)
			return
		}
		records = append(records, rec)
	}

	writeJSON(w, http.StatusOK, records)
}

// createKeyHandler generates a new random API key, stores only its hash,
// and returns the plaintext key exactly once. There is no way to recover
// it afterward — the caller must save it now.
func createKeyHandler(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")

	rawKey, err := generateAPIKey()
	if err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}

	var rec struct {
		ID        int       `json:"id"`
		ClientID  int       `json:"client_id"`
		Key       string    `json:"key"`
		CreatedAt time.Time `json:"created_at"`
	}
	err = db.QueryRow(r.Context(),
		`INSERT INTO api_keys (client_id, key_hash) VALUES ($1, $2)
		 RETURNING id, client_id, created_at`,
		clientID, hashKey(rawKey),
	).Scan(&rec.ID, &rec.ClientID, &rec.CreatedAt)
	if err != nil {
		http.Error(w, "failed to create key (client may not exist)", http.StatusBadRequest)
		return
	}
	rec.Key = rawKey

	writeJSON(w, http.StatusCreated, rec)
}

// generateAPIKey returns a URL-safe, base64-encoded string built from 32
// bytes of crypto/rand output — cryptographically random, unlike the
// hardcoded test-key-123 fixture seeded in step 5.
func generateAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func revokeKeyHandler(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "id")
	keyID := chi.URLParam(r, "keyID")

	tag, err := db.Exec(r.Context(),
		`UPDATE api_keys SET revoked_at = now()
		 WHERE id = $1 AND client_id = $2 AND revoked_at IS NULL`,
		keyID, clientID,
	)
	if err != nil {
		http.Error(w, "failed to revoke key", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "key not found or already revoked", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
