package handlers

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"shopping-list/db"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

type concurrentWriteDetector struct {
	active     atomic.Int32
	concurrent atomic.Bool
}

func (writer *concurrentWriteDetector) beginWrite() {
	if writer.active.Add(1) > 1 {
		writer.concurrent.Store(true)
	}
	time.Sleep(250 * time.Microsecond)
	writer.active.Add(-1)
}

func (writer *concurrentWriteDetector) WriteJSON(interface{}) error {
	writer.beginWrite()
	return nil
}

func (writer *concurrentWriteDetector) WriteMessage(int, []byte) error {
	writer.beginWrite()
	return nil
}

func (writer *concurrentWriteDetector) Close() error {
	return nil
}

func initTestDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	db.Init()
	t.Cleanup(db.Close)
}

func postImportFile(t *testing.T, app *fiber.App, filename, contents string) *http.Response {
	return postImportFileWithResolution(t, app, filename, contents, "skip")
}

func postImportFileWithResolution(t *testing.T, app *fiber.App, filename, contents, conflictResolution string) *http.Response {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := io.WriteString(part, contents); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.WriteField("conflict_resolution", conflictResolution); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := app.Test(req, 2000)
	if err != nil {
		t.Fatalf("import request did not complete: %v", err)
	}
	return resp
}

func TestBulkItemMutationsReturnOnlyExactChangedRows(t *testing.T) {
	initTestDatabase(t)

	list, err := db.CreateList("Weekly", "cart")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	if err := db.SetActiveList(list.ID); err != nil {
		t.Fatalf("activate list: %v", err)
	}
	section, err := db.CreateSectionForList(list.ID, "General")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	first, err := db.CreateItem(section.ID, "Milk", "", 1)
	if err != nil {
		t.Fatalf("create first item: %v", err)
	}
	second, err := db.CreateItem(section.ID, "Bread", "", 1)
	if err != nil {
		t.Fatalf("create second item: %v", err)
	}
	if _, err := db.DB.Exec("UPDATE items SET completed = TRUE, updated_at = 1 WHERE id = ?", second.ID); err != nil {
		t.Fatalf("prepare completed item: %v", err)
	}
	if _, err := db.DB.Exec("UPDATE items SET updated_at = 1 WHERE id = ?", first.ID); err != nil {
		t.Fatalf("prepare active item: %v", err)
	}

	checked, err := db.CheckAllItems(section.ID)
	if err != nil {
		t.Fatalf("check all items: %v", err)
	}
	if len(checked) != 1 || checked[0].ID != first.ID {
		t.Fatalf("checked items = %#v, want only item %d", checked, first.ID)
	}
	if !checked[0].Completed || checked[0].UpdatedAt <= 1 {
		t.Fatalf("checked item state = %#v, want completed with current updated_at", checked[0])
	}
	checkedAgain, err := db.CheckAllItems(section.ID)
	if err != nil {
		t.Fatalf("check all items again: %v", err)
	}
	if len(checkedAgain) != 0 {
		t.Fatalf("second check returned %d rows, want 0", len(checkedAgain))
	}

	unchecked, err := db.UncheckAllItems(section.ID)
	if err != nil {
		t.Fatalf("uncheck all items: %v", err)
	}
	if len(unchecked) != 2 {
		t.Fatalf("unchecked items = %d, want 2", len(unchecked))
	}
	for _, item := range unchecked {
		if item.Completed {
			t.Fatalf("uncheck returned completed item: %#v", item)
		}
	}

	if _, err := db.DB.Exec("UPDATE items SET completed = TRUE WHERE id = ?", first.ID); err != nil {
		t.Fatalf("prepare completed item for deletion: %v", err)
	}
	deleted, err := db.DeleteCompletedItems()
	if err != nil {
		t.Fatalf("delete completed items: %v", err)
	}
	if len(deleted) != 1 || deleted[0].ID != first.ID || !deleted[0].Completed {
		t.Fatalf("deleted items = %#v, want completed item %d", deleted, first.ID)
	}
}

func TestImportDataDoesNotDeadlockSingleConnectionPool(t *testing.T) {
	initTestDatabase(t)
	app := fiber.New()
	app.Post("/import", ImportData)

	jsonImport := `{
		"version":"1.0",
		"app":"koffan",
		"data":{
			"lists":[{
				"name":"Weekly",
				"icon":"shopping-cart",
				"sections":[{"name":"General","items":[{"name":"Milk"}]}]
			}],
			"templates":[{
				"name":"Basics",
				"items":[{"section_name":"General","name":"Bread"}]
			}]
		}
	}`
	resp := postImportFile(t, app, "review.json", jsonImport)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("JSON import status = %d, body = %s", resp.StatusCode, body)
	}

	csvImport := "list_name,list_icon,section_name,item_name,item_description,item_completed,item_uncertain,item_quantity\n" +
		"Weekend,shopping-cart,General,Eggs,,false,false,1\n"
	resp = postImportFile(t, app, "review.csv", csvImport)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("CSV import status = %d, body = %s", resp.StatusCode, body)
	}

	lists, err := db.GetAllLists()
	if err != nil {
		t.Fatalf("read lists after import: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("imported lists = %d, want 2", len(lists))
	}

	templates, err := db.GetAllTemplates()
	if err != nil {
		t.Fatalf("read templates after import: %v", err)
	}
	if len(templates) != 1 || len(templates[0].Items) != 1 {
		t.Fatalf("imported templates = %#v, want one template with one item", templates)
	}
}

func TestAuthMiddlewareHandlesShortSessionID(t *testing.T) {
	initTestDatabase(t)
	app := fiber.New()
	app.Use(recover.New())
	app.Use(AuthMiddleware)
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "x"})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s; want redirect", resp.StatusCode, body)
	}
	if location := resp.Header.Get("Location"); location != "/login" {
		t.Fatalf("Location = %q, want /login", location)
	}
}

func TestItemUIHandlersEnforceLengthLimits(t *testing.T) {
	app := fiber.New()
	app.Post("/items", CreateItem)
	app.Put("/items/:id", UpdateItem)

	tests := []struct {
		name   string
		method string
		path   string
		form   url.Values
	}{
		{
			name:   "create rejects long name",
			method: http.MethodPost,
			path:   "/items",
			form: url.Values{
				"section_id": {"1"},
				"name":       {strings.Repeat("a", MaxItemNameLength+1)},
			},
		},
		{
			name:   "update rejects long description",
			method: http.MethodPut,
			path:   "/items/1",
			form: url.Values{
				"name":        {"Milk"},
				"description": {strings.Repeat("a", MaxDescriptionLength+1)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, body = %s; want 400", resp.StatusCode, body)
			}
		})
	}
}

func TestConcurrentWebSocketBroadcastsUseSingleWriter(t *testing.T) {
	detector := &concurrentWriteDetector{}
	client := &webSocketClient{conn: detector}

	clientsMu.Lock()
	originalClients := clients
	clients = map[*websocket.Conn]*webSocketClient{
		nil: client,
	}
	clientsMu.Unlock()
	t.Cleanup(func() {
		clientsMu.Lock()
		clients = originalClients
		clientsMu.Unlock()
	})

	var waitGroup sync.WaitGroup
	for i := 0; i < 20; i++ {
		waitGroup.Add(2)
		go func(index int) {
			defer waitGroup.Done()
			BroadcastUpdate("test", map[string]int{"index": index})
		}(i)
		go func() {
			defer waitGroup.Done()
			_ = client.writeJSON(map[string]string{"type": "pong"})
		}()
	}
	waitGroup.Wait()

	if detector.concurrent.Load() {
		t.Fatal("websocket writes overlapped")
	}
}
