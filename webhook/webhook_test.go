package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type capturedRequest struct {
	method string
	header http.Header
	body   []byte
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func initTestOutbox(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatalf("open outbox database: %v", err)
	}
	if _, err := database.Exec(`
		CREATE TABLE webhook_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event TEXT NOT NULL,
			payload BLOB NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			available_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			last_error TEXT DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)
	`); err != nil {
		database.Close()
		t.Fatalf("create outbox table: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func closeDispatcher(t *testing.T, dispatcher *Dispatcher) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := dispatcher.Close(ctx); err != nil {
		t.Fatalf("close dispatcher: %v", err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "invalid URL", config: Config{URL: "localhost/webhook"}},
		{name: "unsupported scheme", config: Config{URL: "ftp://example.com/webhook"}},
		{name: "unsupported event", config: Config{URL: "https://example.com/webhook", Events: "item.created,list.created"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.config); err == nil {
				t.Fatal("New() error = nil, want configuration error")
			}
		})
	}
}

func TestDisabledDispatcherDoesNotQueueEvents(t *testing.T) {
	dispatcher, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if dispatcher.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	if dispatcher.Notify(EventItemCreated, map[string]string{"name": "Milk"}) {
		t.Fatal("Notify() = true for disabled dispatcher, want false")
	}
}

func TestDispatcherPostsSignedEvent(t *testing.T) {
	database := initTestOutbox(t)
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- capturedRequest{method: request.Method, header: request.Header.Clone(), body: body}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := New(Config{URL: server.URL, Secret: "test-secret", DB: database})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeDispatcher(t, dispatcher)
	if !dispatcher.Notify(EventItemCreated, map[string]interface{}{
		"item": map[string]interface{}{"id": float64(42), "name": "Milk"},
	}) {
		t.Fatal("Notify() = false, want true")
	}

	select {
	case request := <-requests:
		if request.method != http.MethodPost {
			t.Errorf("method = %q, want POST", request.method)
		}
		if contentType := request.header.Get("Content-Type"); contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}
		if event := request.header.Get("X-Koffan-Event"); event != EventItemCreated {
			t.Errorf("X-Koffan-Event = %q, want %q", event, EventItemCreated)
		}

		mac := hmac.New(sha256.New, []byte("test-secret"))
		_, _ = mac.Write(request.body)
		wantSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if signature := request.header.Get("X-Koffan-Signature-256"); !hmac.Equal([]byte(signature), []byte(wantSignature)) {
			t.Errorf("signature = %q, want %q", signature, wantSignature)
		}

		var event Event
		if err := json.Unmarshal(request.body, &event); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if event.Event != EventItemCreated {
			t.Errorf("event = %q, want %q", event.Event, EventItemCreated)
		}
		if event.ID == "" {
			t.Error("event ID is empty")
		}
		if event.Timestamp.IsZero() {
			t.Error("timestamp is zero")
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("data type = %T, want object", event.Data)
		}
		item, ok := data["item"].(map[string]interface{})
		if !ok || item["name"] != "Milk" {
			t.Errorf("item = %#v, want Milk", data["item"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook request")
	}
}

func TestDispatcherFiltersEvents(t *testing.T) {
	database := initTestOutbox(t)
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- struct{}{}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := New(Config{URL: server.URL, Events: EventItemDeleted, DB: database})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeDispatcher(t, dispatcher)
	if dispatcher.Notify(EventItemCreated, nil) {
		t.Fatal("Notify(created) = true, want filtered event")
	}
	if !dispatcher.Notify(EventItemDeleted, nil) {
		t.Fatal("Notify(deleted) = false, want queued event")
	}

	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for enabled event")
	}
}

func TestDispatcherRetriesPersistedEventAfterRestart(t *testing.T) {
	database := initTestOutbox(t)
	failedBodies := make(chan []byte, 1)
	failingClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		select {
		case failedBodies <- body:
		default:
		}
		return nil, errors.New("temporary failure")
	})}

	first, err := New(Config{
		URL:          "http://127.0.0.1:1/webhook",
		DB:           database,
		HTTPClient:   failingClient,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create first dispatcher: %v", err)
	}
	if !first.Notify(EventItemCreated, map[string]string{"name": "Milk"}) {
		t.Fatal("Notify() = false, want persisted event")
	}
	var firstBody []byte
	select {
	case firstBody = <-failedBodies:
	case <-time.After(2 * time.Second):
		t.Fatal("first delivery was not attempted")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		var attempts int
		if err := database.QueryRow("SELECT attempts FROM webhook_outbox LIMIT 1").Scan(&attempts); err == nil && attempts > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event was not scheduled for retry")
		}
		time.Sleep(10 * time.Millisecond)
	}
	closeDispatcher(t, first)

	deliveries := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		deliveries <- body
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	second, err := New(Config{URL: server.URL, DB: database, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("create restarted dispatcher: %v", err)
	}
	defer closeDispatcher(t, second)

	var restartedBody []byte
	select {
	case restartedBody = <-deliveries:
	case <-time.After(3 * time.Second):
		t.Fatal("persisted event was not delivered after restart")
	}
	if !bytes.Equal(restartedBody, firstBody) {
		t.Fatalf("retried payload changed after restart:\nfirst: %s\nretry: %s", firstBody, restartedBody)
	}
	var event Event
	if err := json.Unmarshal(restartedBody, &event); err != nil {
		t.Fatalf("decode restarted event: %v", err)
	}
	if event.ID == "" {
		t.Fatal("restarted event ID is empty")
	}

	deadline = time.Now().Add(time.Second)
	for {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM webhook_outbox").Scan(&count); err != nil {
			t.Fatalf("count outbox rows: %v", err)
		}
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("outbox rows = %d, want 0", count)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
