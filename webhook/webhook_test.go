package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type capturedRequest struct {
	method string
	header http.Header
	body   []byte
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
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- capturedRequest{method: request.Method, header: request.Header.Clone(), body: body}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := New(Config{URL: server.URL, Secret: "test-secret"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- struct{}{}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := New(Config{URL: server.URL, Events: EventItemDeleted})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
