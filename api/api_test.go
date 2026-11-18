package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"shopping-list/db"

	"github.com/gofiber/fiber/v2"
)

type apiWorkspace struct {
	id      int64
	list    *db.List
	section *db.Section
	item    *db.Item
	history int64
}

func newAPIWorkspace(t *testing.T, name string) apiWorkspace {
	t.Helper()
	owner, err := db.CreateUser(name+"-owner", name, "correct horse battery", false)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := db.CreateWorkspace(name, "", owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateList(workspace.ID, name+" list", "")
	if err != nil {
		t.Fatal(err)
	}
	section, err := db.CreateSectionForList(list.ID, name+" section")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateItem(section.ID, name+" milk", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveItemHistory(workspace.ID, item.Name, section.ID); err != nil {
		t.Fatal(err)
	}
	history, _ := db.GetItemHistoryList(workspace.ID)
	return apiWorkspace{workspace.ID, list, section, item, history[0].ID}
}

func apiCall(t *testing.T, app *fiber.App, method, path string, body interface{}) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer secret-token")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

// The REST API token authenticates a program, not a person, so it is pinned to
// one workspace (API_WORKSPACE_ID) and must not reach any other.
func TestTokenIsPinnedToOneWorkspace(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "api.db"))
	db.Init()
	t.Cleanup(db.Close)
	own, other := newAPIWorkspace(t, "own"), newAPIWorkspace(t, "other")
	t.Setenv("API_TOKEN", "secret-token")
	t.Setenv("MULTI_USER", "true")
	t.Setenv("API_WORKSPACE_ID", id(own.id))
	app := fiber.New()
	Register(app)

	// Reading: only the pinned workspace is visible.
	status, body := apiCall(t, app, "GET", "/api/v1/lists", nil)
	if status != http.StatusOK || !bytes.Contains([]byte(body), []byte("own list")) || bytes.Contains([]byte(body), []byte("other")) {
		t.Fatalf("GET /lists: %d %s", status, body)
	}
	if status, body := apiCall(t, app, "GET", "/api/v1/history", nil); status != http.StatusOK || bytes.Contains([]byte(body), []byte("other milk")) {
		t.Fatalf("GET /history: %d %s", status, body)
	}

	// Anything that names another workspace's data looks like it does not exist.
	for name, request := range map[string]struct {
		method, path string
		body         interface{}
	}{
		"get list":       {"GET", "/api/v1/lists/" + id(other.list.ID), nil},
		"rename list":    {"PUT", "/api/v1/lists/" + id(other.list.ID), map[string]string{"name": "hijacked"}},
		"delete list":    {"DELETE", "/api/v1/lists/" + id(other.list.ID), nil},
		"move list":      {"POST", "/api/v1/lists/" + id(other.list.ID) + "/move-up", nil},
		"list sections":  {"GET", "/api/v1/lists/" + id(other.list.ID) + "/sections", nil},
		"get section":    {"GET", "/api/v1/sections/" + id(other.section.ID), nil},
		"delete section": {"DELETE", "/api/v1/sections/" + id(other.section.ID), nil},
		"check all":      {"POST", "/api/v1/sections/" + id(other.section.ID) + "/check-all", nil},
		"get item":       {"GET", "/api/v1/items/" + id(other.item.ID), nil},
		"edit item":      {"PUT", "/api/v1/items/" + id(other.item.ID), map[string]string{"name": "hijacked"}},
		"toggle item":    {"POST", "/api/v1/items/" + id(other.item.ID) + "/toggle", nil},
		"delete item":    {"DELETE", "/api/v1/items/" + id(other.item.ID), nil},
		"delete history": {"DELETE", "/api/v1/history/" + id(other.history), nil},
		"add section to foreign list": {"POST", "/api/v1/sections",
			map[string]interface{}{"list_id": other.list.ID, "name": "planted"}},
		"add item to foreign section": {"POST", "/api/v1/items",
			map[string]interface{}{"section_id": other.section.ID, "name": "planted"}},
		"move own item to foreign section": {"POST", "/api/v1/items/" + id(own.item.ID) + "/move",
			map[string]interface{}{"section_id": other.section.ID}},
		"history entry in foreign section": {"POST", "/api/v1/history",
			map[string]interface{}{"name": "planted", "section_id": other.section.ID}},
		"batch into foreign list": {"POST", "/api/v1/batch",
			map[string]interface{}{"list_id": other.list.ID, "sections": []map[string]interface{}{{"name": "planted"}}}},
		"batch into foreign section": {"POST", "/api/v1/batch",
			map[string]interface{}{"section_id": other.section.ID, "items": []map[string]interface{}{{"name": "planted"}}}},
	} {
		if status, body := apiCall(t, app, request.method, request.path, request.body); status != http.StatusNotFound {
			t.Errorf("%s: %s %s returned %d (%s), want 404", name, request.method, request.path, status, body)
		}
	}

	snapshot, err := db.GetOfflineSnapshot(other.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Lists) != 1 || snapshot.Lists[0].Name != "other list" || len(snapshot.Sections) != 1 ||
		snapshot.Sections[0].Name != "other section" || len(snapshot.Items) != 1 || snapshot.Items[0].Name != "other milk" || snapshot.Items[0].Completed {
		t.Fatalf("the other workspace was modified through the API: %#v", snapshot)
	}
	if history, _ := db.GetItemHistoryList(other.id); len(history) != 1 {
		t.Fatalf("the other workspace's history changed: %#v", history)
	}
	if moved, _ := db.GetItemByID(own.item.ID); moved.SectionID != own.section.ID {
		t.Fatal("an item was moved into the other workspace")
	}

	// Writing without naming anything lands in the pinned workspace.
	if status, body := apiCall(t, app, "POST", "/api/v1/lists", map[string]string{"name": "own list"}); status != http.StatusBadRequest {
		t.Fatalf("duplicate name inside the workspace: %d %s", status, body)
	}
	if status, body := apiCall(t, app, "POST", "/api/v1/lists", map[string]string{"name": "other list"}); status != http.StatusCreated {
		t.Fatalf("the same name is fine in another workspace: %d %s", status, body)
	}
	// A history entry with no section must not fall back to workspace 1.
	if status, body := apiCall(t, app, "POST", "/api/v1/history", map[string]string{"name": "api remembered"}); status >= 300 {
		t.Fatalf("POST /history: %d %s", status, body)
	}
	remembered := func(workspaceID int64) bool {
		history, _ := db.GetItemHistoryList(workspaceID)
		for _, h := range history {
			if h.Name == "api remembered" {
				return true
			}
		}
		return false
	}
	if !remembered(own.id) || remembered(other.id) || remembered(db.DefaultWorkspaceID) {
		t.Fatalf("history entry went to the wrong workspace: own=%v other=%v default=%v", remembered(own.id), remembered(other.id), remembered(db.DefaultWorkspaceID))
	}
	// Batch creating a list, and adding items to an own section, both stay put.
	if status, body := apiCall(t, app, "POST", "/api/v1/batch", map[string]interface{}{
		"list": map[string]interface{}{"name": "Batch list", "sections": []map[string]interface{}{
			{"name": "Aisle", "items": []map[string]interface{}{{"name": "batch item"}}}}}}); status >= 300 {
		t.Fatalf("batch create list: %d %s", status, body)
	}
	if lists, _ := db.GetAllLists(own.id); len(lists) != 3 { // own, "other list", Batch list
		t.Fatalf("own lists = %d", len(lists))
	}
	if lists, _ := db.GetAllLists(other.id); len(lists) != 1 {
		t.Fatalf("other lists = %d", len(lists))
	}
	if remembered2 := func() bool {
		history, _ := db.GetItemHistoryList(other.id)
		for _, h := range history {
			if h.Name == "batch item" {
				return true
			}
		}
		return false
	}(); remembered2 {
		t.Fatal("batch history leaked into the other workspace")
	}
}

// Guards run before the API handlers, so a missing or foreign id must still be
// answered in the API's own JSON error shape, with the handlers' messages.
func TestGuardFailuresKeepTheAPIErrorShape(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "api.db"))
	db.Init()
	t.Cleanup(db.Close)
	own, other := newAPIWorkspace(t, "own"), newAPIWorkspace(t, "other")
	t.Setenv("API_TOKEN", "secret-token")
	app := fiber.New()
	Register(app)

	check := func(label, path, message string) {
		t.Helper()
		status, body := apiCall(t, app, "GET", path, nil)
		var answer struct{ Error, Message string }
		if err := json.Unmarshal([]byte(body), &answer); err != nil {
			t.Errorf("%s: %d %q is not JSON", label, status, body)
			return
		}
		if status != http.StatusNotFound || answer.Error != "not_found" || answer.Message != message {
			t.Errorf("%s: %d %+v, want 404 not_found %q", label, status, answer, message)
		}
	}
	// Default mode: an id that does not exist.
	check("missing list", "/api/v1/lists/999999", "List not found")
	check("missing section", "/api/v1/sections/999999", "Section not found")
	check("missing item", "/api/v1/items/999999", "Item not found")
	// Accounts: an id from another workspace looks the same.
	t.Setenv("MULTI_USER", "true")
	t.Setenv("API_WORKSPACE_ID", id(own.id))
	check("foreign list", "/api/v1/lists/"+id(other.list.ID), "List not found")
	check("foreign section", "/api/v1/sections/"+id(other.section.ID), "Section not found")
	check("foreign item", "/api/v1/items/"+id(other.item.ID), "Item not found")
}

func TestCreateItemReactivationReplacesExplicitQuantity(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "api.db"))
	db.Init()
	t.Cleanup(db.Close)
	workspace := newAPIWorkspace(t, "own")
	t.Setenv("API_TOKEN", "secret-token")
	t.Setenv("MULTI_USER", "true")
	t.Setenv("API_WORKSPACE_ID", id(workspace.id))
	app := fiber.New()
	Register(app)

	if _, _, err := db.SetItemCompleted(workspace.item.ID, true); err != nil {
		t.Fatal(err)
	}
	status, body := apiCall(t, app, "POST", "/api/v1/items", map[string]interface{}{
		"section_id": workspace.section.ID,
		"name":       workspace.item.Name,
		"quantity":   0,
	})
	if status != http.StatusOK {
		t.Fatalf("POST /items with zero quantity: %d %s", status, body)
	}
	var response struct {
		Item db.Item `json:"item"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Item.Completed || response.Item.Quantity != 0 {
		t.Fatalf("reactivated item = %#v, want active with quantity 0", response.Item)
	}

	if _, _, err := db.SetItemCompleted(workspace.item.ID, true); err != nil {
		t.Fatal(err)
	}
	status, body = apiCall(t, app, "POST", "/api/v1/items", map[string]interface{}{
		"section_id": workspace.section.ID,
		"name":       workspace.item.Name,
	})
	if status != http.StatusOK {
		t.Fatalf("POST /items without quantity: %d %s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Item.Completed || response.Item.Quantity != 0 {
		t.Fatalf("reactivated item without quantity = %#v, want active and unchanged", response.Item)
	}
}
