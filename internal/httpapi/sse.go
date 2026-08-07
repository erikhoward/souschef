package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

// Event is what the browser receives. Idea is nil for deletions, where only
// the ID is meaningful.
type Event struct {
	Type string      `json:"type"`
	Idea *ideas.Idea `json:"idea,omitempty"`
	ID   string      `json:"id,omitempty"`
}

// subscriberBuffer is how many events a single client can fall behind before
// we start dropping its updates. Dropping is the correct trade: the client
// refetches on reconnect, whereas blocking would stall the enrichment
// goroutine doing the broadcast.
const subscriberBuffer = 64

// keepAliveInterval keeps intermediaries from closing an idle stream.
const keepAliveInterval = 25 * time.Second

type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan Event]struct{})}
}

func (h *Hub) subscribe() chan Event {
	ch := make(chan Event, subscriberBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return ch
	}
	h.subscribers[ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
}

// Broadcast sends to every subscriber without blocking. A client that has
// stopped reading loses events rather than holding up the sender.
func (h *Hub) Broadcast(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subscribers {
		select {
		case ch <- ev:
		default:
			// Buffer full: drop. The client resynchronises on reconnect.
		}
	}
}

func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subscribers {
		delete(h.subscribers, ch)
		close(ch)
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Vite's dev proxy and any intermediary must not buffer this.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Tell the browser how quickly to come back. EventSource reconnects on
	// its own, so no client-side retry code is needed.
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case ev, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				log.Printf("httpapi: marshal event: %v", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()

		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
