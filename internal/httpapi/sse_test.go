package httpapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
)

func TestHubDeliversEventToSubscriber(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	waitForSubscribers(t, hub, 1)

	idea := ideas.Idea{ID: "i1", Title: "Crispy chili eggs"}
	hub.Broadcast(Event{Type: "idea.updated", Idea: &idea})

	got := readEvent(t, resp.Body)
	if got.Type != "idea.updated" {
		t.Errorf("Type = %q", got.Type)
	}
	if got.Idea == nil || got.Idea.Title != "Crispy chili eggs" {
		t.Errorf("payload did not survive the wire: %+v", got.Idea)
	}
}

func TestHubFansOutToMultipleSubscribers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	const n = 3
	bodies := make([]*http.Response, n)
	for i := range bodies {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("connect %d: %v", i, err)
		}
		defer resp.Body.Close()
		bodies[i] = resp
	}
	waitForSubscribers(t, hub, n)

	hub.Broadcast(Event{Type: "idea.deleted", ID: "gone"})

	for i, resp := range bodies {
		if got := readEvent(t, resp.Body); got.ID != "gone" {
			t.Errorf("subscriber %d got %+v", i, got)
		}
	}
}

func TestHubDropsSubscriberOnDisconnect(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	waitForSubscribers(t, hub, 1)

	resp.Body.Close()
	waitForSubscribers(t, hub, 0)
}

// A slow or wedged reader must not block the enrichment goroutine that is
// broadcasting. This is the failure mode that would turn one stuck browser
// tab into a stalled backend.
func TestBroadcastDoesNotBlockOnSlowSubscriber(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	waitForSubscribers(t, hub, 1)

	done := make(chan struct{})
	go func() {
		// Far more events than any per-subscriber buffer.
		for i := 0; i < 5000; i++ {
			hub.Broadcast(Event{Type: "idea.updated", ID: "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Broadcast blocked on a subscriber that stopped reading")
	}
}

func TestBroadcastIsRaceFree(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	srv := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer srv.Close()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				hub.Broadcast(Event{Type: "idea.updated", ID: "x"})
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL)
			if err != nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
			resp.Body.Close()
		}()
	}
	wg.Wait()
}

func waitForSubscribers(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("SubscriberCount = %d, want %d", hub.SubscriberCount(), want)
}

// readEvent reads one SSE frame, skipping the retry hint and any comments.
func readEvent(t *testing.T, r interface{ Read([]byte) (int, error) }) Event {
	t.Helper()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		return ev
	}
	t.Fatal("stream closed before an event arrived")
	return Event{}
}
