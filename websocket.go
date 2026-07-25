package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// wsUpgrader turns an HTTP request into a WebSocket connection. Browsers'
// native WebSocket API can't set custom headers on the handshake request,
// so unlike the rest of the admin API this endpoint checks its token as a
// query parameter instead, and CheckOrigin substitutes for the header
// check other admin routes rely on for cross-origin protection.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || origin == dashboardOrigin
	},
}

// websocketHandler upgrades the connection, then streams every traffic
// event published to Redis straight through to the client as JSON text
// frames, until either side disconnects.
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	pubsub := rdb.Subscribe(context.Background(), trafficChannel)
	defer pubsub.Close()

	// We don't expect the dashboard to send anything, but a read loop is
	// still required: it's what processes the close handshake and lets us
	// notice the client disconnecting.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	events := pubsub.Channel()
	for {
		select {
		case msg, ok := <-events:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		case <-closed:
			return
		}
	}
}
