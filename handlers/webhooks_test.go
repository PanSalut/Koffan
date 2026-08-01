package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"shopping-list/db"
	"shopping-list/webhook"
	"testing"
	"time"
)

type capturedItemWebhook struct {
	Event string `json:"event"`
	Data  struct {
		Item    db.Item       `json:"item"`
		Section webhookEntity `json:"section"`
		List    webhookEntity `json:"list"`
	} `json:"data"`
}

func TestPreparedItemWebhookKeepsContextAfterSectionDeletion(t *testing.T) {
	initTestDatabase(t)

	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies <- body
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("WEBHOOK_URL", server.URL)
	t.Setenv("WEBHOOK_EVENTS", webhook.EventItemDeleted)
	t.Setenv("WEBHOOK_SECRET", "")
	if err := webhook.ConfigureFromEnv(); err != nil {
		t.Fatalf("configure webhook: %v", err)
	}
	t.Cleanup(func() {
		t.Setenv("WEBHOOK_URL", "")
		t.Setenv("WEBHOOK_EVENTS", "")
		_ = webhook.ConfigureFromEnv()
	})

	list, err := db.CreateList("Weekly groceries", "cart")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	section, err := db.CreateSectionForList(list.ID, "Dairy")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	item, err := db.CreateItem(section.ID, "Milk", "", 1)
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	prepared := PrepareItemWebhooks(webhook.EventItemDeleted, []db.Item{*item})
	if err := db.DeleteSection(section.ID); err != nil {
		t.Fatalf("delete section: %v", err)
	}
	NotifyPreparedItemWebhooks(webhook.EventItemDeleted, prepared)

	select {
	case body := <-bodies:
		var event capturedItemWebhook
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("decode webhook: %v", err)
		}
		if event.Event != webhook.EventItemDeleted {
			t.Errorf("event = %q, want %q", event.Event, webhook.EventItemDeleted)
		}
		if event.Data.Item.Name != "Milk" {
			t.Errorf("item name = %q, want Milk", event.Data.Item.Name)
		}
		if event.Data.Section.ID != section.ID || event.Data.Section.Name != "Dairy" {
			t.Errorf("section = %#v, want deleted Dairy section", event.Data.Section)
		}
		if event.Data.List.ID != list.ID || event.Data.List.Name != "Weekly groceries" {
			t.Errorf("list = %#v, want Weekly groceries list", event.Data.List)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook")
	}
}
