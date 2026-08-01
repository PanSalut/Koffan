package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"shopping-list/db"
	"shopping-list/webhook"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

type capturedItemWebhook struct {
	ID    string `json:"id"`
	Event string `json:"event"`
	Data  struct {
		Item    db.Item       `json:"item"`
		Section webhookEntity `json:"section"`
		List    webhookEntity `json:"list"`
	} `json:"data"`
}

func configureTestWebhook(t *testing.T, endpoint, events string) {
	t.Helper()
	t.Setenv("WEBHOOK_URL", endpoint)
	t.Setenv("WEBHOOK_EVENTS", events)
	t.Setenv("WEBHOOK_SECRET", "")
	if err := webhook.ConfigureFromEnv(db.DB); err != nil {
		t.Fatalf("configure webhook: %v", err)
	}
	t.Cleanup(func() {
		t.Setenv("WEBHOOK_URL", "")
		t.Setenv("WEBHOOK_EVENTS", "")
		_ = webhook.ConfigureFromEnv(db.DB)
	})
}

func waitForItemWebhook(t *testing.T, bodies <-chan []byte) capturedItemWebhook {
	t.Helper()
	select {
	case body := <-bodies:
		var event capturedItemWebhook
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatalf("decode webhook: %v", err)
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook")
		return capturedItemWebhook{}
	}
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

	configureTestWebhook(t, server.URL, webhook.EventItemDeleted)

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

func TestImportReplaceEmitsDeletedAndCreatedItemWebhooks(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		contents string
	}{
		{
			name:     "JSON",
			filename: "replace.json",
			contents: `{
				"version":"1.0",
				"app":"koffan",
				"data":{"lists":[{
					"name":"Weekly",
					"icon":"cart",
					"sections":[{"name":"General","items":[{
						"name":"New item","completed":true,"uncertain":true,"quantity":2
					}]}]
				}]}
			}`,
		},
		{
			name:     "CSV",
			filename: "replace.csv",
			contents: "list_name,list_icon,section_name,item_name,item_description,item_completed,item_uncertain,item_quantity\n" +
				"Weekly,cart,General,New item,,true,true,2\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initTestDatabase(t)
			bodies := make(chan []byte, 4)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, _ := io.ReadAll(request.Body)
				bodies <- body
				writer.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			configureTestWebhook(t, server.URL, webhook.EventItemDeleted+","+webhook.EventItemCreated)

			list, err := db.CreateList("Weekly", "cart")
			if err != nil {
				t.Fatalf("create existing list: %v", err)
			}
			section, err := db.CreateSectionForList(list.ID, "Old section")
			if err != nil {
				t.Fatalf("create existing section: %v", err)
			}
			if _, err := db.CreateItem(section.ID, "Old item", "", 1); err != nil {
				t.Fatalf("create existing item: %v", err)
			}

			app := fiber.New()
			app.Post("/import", ImportData)
			response := postImportFileWithResolution(t, app, test.filename, test.contents, "replace")
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("import status = %d, body = %s", response.StatusCode, body)
			}

			deleted := waitForItemWebhook(t, bodies)
			created := waitForItemWebhook(t, bodies)
			if deleted.Event != webhook.EventItemDeleted || deleted.Data.Item.Name != "Old item" {
				t.Fatalf("first event = %#v, want deleted Old item", deleted)
			}
			if deleted.Data.Section.Name != "Old section" || deleted.Data.List.Name != "Weekly" {
				t.Fatalf("deleted context = %#v, want old section and Weekly list", deleted.Data)
			}
			if created.Event != webhook.EventItemCreated || created.Data.Item.Name != "New item" {
				t.Fatalf("second event = %#v, want created New item", created)
			}
			if !created.Data.Item.Completed || !created.Data.Item.Uncertain || created.Data.Item.Quantity != 2 {
				t.Fatalf("created item state = %#v, want final imported state", created.Data.Item)
			}
			if deleted.ID == "" || created.ID == "" || deleted.ID == created.ID {
				t.Fatalf("event IDs = %q and %q, want unique non-empty IDs", deleted.ID, created.ID)
			}
		})
	}
}

func TestClearDatabaseEmitsDeletedItemWebhooks(t *testing.T) {
	initTestDatabase(t)
	bodies := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		bodies <- body
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	configureTestWebhook(t, server.URL, webhook.EventItemDeleted)

	list, err := db.CreateList("Weekly", "cart")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	section, err := db.CreateSectionForList(list.ID, "Dairy")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	if _, err := db.CreateItem(section.ID, "Milk", "", 1); err != nil {
		t.Fatalf("create item: %v", err)
	}

	app := fiber.New()
	app.Get("/csrf", GenerateCSRFToken)
	app.Delete("/database", ClearDatabase)
	csrfResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/csrf", nil))
	if err != nil {
		t.Fatalf("get CSRF token: %v", err)
	}
	var csrf struct {
		Token string `json:"csrf_token"`
	}
	if err := json.NewDecoder(csrfResponse.Body).Decode(&csrf); err != nil {
		csrfResponse.Body.Close()
		t.Fatalf("decode CSRF token: %v", err)
	}
	csrfResponse.Body.Close()

	requestBody, _ := json.Marshal(ClearDatabaseRequest{Confirmation: "DELETE", CSRFToken: csrf.Token})
	request := httptest.NewRequest(http.MethodDelete, "/database", bytes.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("clear database request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("clear database status = %d, body = %s", response.StatusCode, body)
	}

	event := waitForItemWebhook(t, bodies)
	if event.Event != webhook.EventItemDeleted || event.Data.Item.Name != "Milk" {
		t.Fatalf("event = %#v, want deleted Milk", event)
	}
	if event.Data.Section.Name != "Dairy" || event.Data.List.Name != "Weekly" {
		t.Fatalf("event context = %#v, want Dairy and Weekly", event.Data)
	}
	var itemCount int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM items").Scan(&itemCount); err != nil {
		t.Fatalf("count remaining items: %v", err)
	}
	if itemCount != 0 {
		t.Fatalf("remaining items = %d, want 0", itemCount)
	}
}

func TestDisabledWebhookPreparationDoesNotReadDatabase(t *testing.T) {
	t.Setenv("WEBHOOK_URL", "")
	t.Setenv("WEBHOOK_EVENTS", "")
	if err := webhook.ConfigureFromEnv(nil); err != nil {
		t.Fatalf("disable webhook: %v", err)
	}

	previousDB := db.DB
	db.DB = nil
	t.Cleanup(func() { db.DB = previousDB })

	if prepared := PrepareSectionItemWebhooks(webhook.EventItemDeleted, 1); prepared != nil {
		t.Fatalf("section preparation = %#v, want nil", prepared)
	}
	if prepared := PrepareListItemWebhooks(webhook.EventItemDeleted, 1); prepared != nil {
		t.Fatalf("list preparation = %#v, want nil", prepared)
	}
	if prepared := PrepareAllItemWebhooks(webhook.EventItemDeleted); prepared != nil {
		t.Fatalf("database preparation = %#v, want nil", prepared)
	}
	NotifyCreatedListItems(1, nil)
	NotifyItemWebhook(webhook.EventItemCreated, &db.Item{ID: 1, SectionID: 1, Name: strconv.Itoa(1)})
}
