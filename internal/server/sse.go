package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// SSEBroker manages Server-Sent Events connections.
// Clients subscribe via GET /api/events and receive JSON events as they occur.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func newSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[chan []byte]struct{}),
	}
}

// Subscribe adds a new client channel and returns it.
func (b *SSEBroker) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 64)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a client channel.
func (b *SSEBroker) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, ch)
	close(ch)
}

// Publish sends an event to all connected clients.
func (b *SSEBroker) Publish(event SSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[sse] marshal error: %v", err)
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// Client too slow, drop this event for them
		}
	}
}

// SSEHandler handles GET /api/events — the SSE endpoint.
func (s *Server) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.sse.Subscribe()
	defer s.sse.Unsubscribe(ch)

	// Send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
