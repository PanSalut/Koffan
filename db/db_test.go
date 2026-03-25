// Package db provides database access for Koffan.
//
// Integration tests:
//
//	Run SQLite tests: go test ./db/...
//	Run Postgres tests: TEST_DATABASE_URL="postgres://..." go test ./db/... -run TestPostgres
package db

import (
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// setupTestDB initialises a fresh test database and returns a cleanup function.
// For SQLite: uses an in-memory database.
// For Postgres: uses the provided DSN, creates and drops tables around the test.
func setupTestDB(t *testing.T, driver, dsn string) func() {
	t.Helper()

	var err error
	DB, err = sqlx.Open(driver, dsn)
	if err != nil {
		t.Fatalf("failed to open %s DB: %v", driver, err)
	}
	if err = DB.Ping(); err != nil {
		t.Fatalf("failed to ping %s DB: %v", driver, err)
	}

	// Set the dialect Driver used throughout the db package.
	if driver == "pgx" {
		Driver = "postgres"
	} else {
		Driver = "sqlite"
	}

	createTables()

	return func() {
		ClearAllData()
		DB.Close()
		DB = nil
	}
}

func TestSQLiteIntegration(t *testing.T) {
	cleanup := setupTestDB(t, "sqlite3", ":memory:?_foreign_keys=on")
	defer cleanup()

	runIntegrationTests(t)
}

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping Postgres tests")
	}

	cleanup := setupTestDB(t, "pgx", dsn)
	defer cleanup()

	runIntegrationTests(t)
}

func runIntegrationTests(t *testing.T) {
	t.Run("ListsCRUD", testListsCRUD)
	t.Run("SectionsCRUD", testSectionsCRUD)
	t.Run("ItemsCRUD", testItemsCRUD)
	t.Run("HistoryUpsert", testHistoryUpsert)
	t.Run("ImportPreservesHigherCount", testImportPreservesHigherCount)
	t.Run("ImportAdoptsHigherCount", testImportAdoptsHigherCount)
	t.Run("ImportPreservesSectionWhenZero", testImportPreservesSectionWhenZero)
	t.Run("Templates", testTemplates)
	t.Run("Sessions", testSessions)
	t.Run("ClearAllData", testClearAllData)
}

// ---------- Lists ----------

func testListsCRUD(t *testing.T) {
	list, err := CreateList("Test List", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if list.ID <= 0 {
		t.Error("expected ID > 0")
	}
	if list.Name != "Test List" {
		t.Errorf("name: got %q want %q", list.Name, "Test List")
	}

	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}

	fetched, err := GetListByID(list.ID)
	if err != nil {
		t.Fatalf("GetListByID: %v", err)
	}
	if fetched.Name != "Test List" {
		t.Errorf("fetched name: got %q", fetched.Name)
	}

	updated, err := UpdateList(list.ID, "Updated List", "🏠")
	if err != nil {
		t.Fatalf("UpdateList: %v", err)
	}
	if updated.Name != "Updated List" {
		t.Errorf("updated name: got %q", updated.Name)
	}

	if err := DeleteList(list.ID); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	_, err = GetListByID(list.ID)
	if err == nil {
		t.Error("expected error after delete")
	}
}

// ---------- Sections ----------

func testSectionsCRUD(t *testing.T) {
	list, err := CreateList("Section Test List", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}

	section, err := CreateSectionForList(list.ID, "Dairy")
	if err != nil {
		t.Fatalf("CreateSectionForList: %v", err)
	}
	if section.ID <= 0 {
		t.Error("expected ID > 0")
	}

	fetched, err := GetSectionByID(section.ID)
	if err != nil {
		t.Fatalf("GetSectionByID: %v", err)
	}
	if fetched.Name != "Dairy" {
		t.Errorf("name: got %q", fetched.Name)
	}

	updated, err := UpdateSection(section.ID, "Vegetables")
	if err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}
	if updated.Name != "Vegetables" {
		t.Errorf("updated name: got %q", updated.Name)
	}

	if err := DeleteSection(section.ID); err != nil {
		t.Fatalf("DeleteSection: %v", err)
	}
	if err := DeleteList(list.ID); err != nil {
		t.Fatalf("DeleteList cleanup: %v", err)
	}
}

// ---------- Items ----------

func testItemsCRUD(t *testing.T) {
	list, err := CreateList("Item Test List", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}
	section, err := CreateSectionForList(list.ID, "Produce")
	if err != nil {
		t.Fatalf("CreateSectionForList: %v", err)
	}

	item, err := CreateItem(section.ID, "Milk", "2L", 1)
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.ID <= 0 {
		t.Error("expected ID > 0")
	}
	if item.Name != "Milk" {
		t.Errorf("name: got %q", item.Name)
	}

	toggled, err := ToggleItemCompleted(item.ID)
	if err != nil {
		t.Fatalf("ToggleItemCompleted: %v", err)
	}
	if !toggled.Completed {
		t.Error("expected completed=true")
	}

	updated, err := UpdateItem(item.ID, "Milk 2%", "1L", 2)
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if updated.Name != "Milk 2%" {
		t.Errorf("updated name: got %q", updated.Name)
	}

	if err := DeleteItem(item.ID); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	_, err = GetItemByID(item.ID)
	if err == nil {
		t.Error("expected error after delete")
	}

	if err := DeleteList(list.ID); err != nil {
		t.Fatalf("DeleteList cleanup: %v", err)
	}
}

// ---------- History (case-insensitive upsert) ----------

func testHistoryUpsert(t *testing.T) {
	list, err := CreateList("History Test List", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}
	section, err := CreateSectionForList(list.ID, "Produce")
	if err != nil {
		t.Fatalf("CreateSectionForList: %v", err)
	}

	// Save "Milk"
	if err := SaveItemHistory("Milk", section.ID); err != nil {
		t.Fatalf("SaveItemHistory Milk: %v", err)
	}
	// Save "milk" — should UPDATE, not INSERT (case-insensitive)
	if err := SaveItemHistory("milk", section.ID); err != nil {
		t.Fatalf("SaveItemHistory milk: %v", err)
	}

	// Get suggestions — should have exactly 1 entry, usage_count = 2
	suggestions, err := GetItemSuggestions("milk", 10)
	if err != nil {
		t.Fatalf("GetItemSuggestions: %v", err)
	}
	if len(suggestions) != 1 {
		t.Errorf("expected 1 suggestion, got %d", len(suggestions))
	}
	if len(suggestions) > 0 && suggestions[0].UsageCount != 2 {
		t.Errorf("expected usage_count=2, got %d", suggestions[0].UsageCount)
	}

	if err := DeleteList(list.ID); err != nil {
		t.Fatalf("DeleteList cleanup: %v", err)
	}
}

func setupImportTestFixture(t *testing.T) (sectionA, sectionB *Section, cleanup func()) {
	t.Helper()
	list, err := CreateList("Import Test List", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}
	a, err := CreateSectionForList(list.ID, "Dairy")
	if err != nil {
		t.Fatalf("CreateSectionForList Dairy: %v", err)
	}
	b, err := CreateSectionForList(list.ID, "Produce")
	if err != nil {
		t.Fatalf("CreateSectionForList Produce: %v", err)
	}
	return a, b, func() { DeleteList(list.ID) }
}

func testImportPreservesHigherCount(t *testing.T) {
	sectionA, sectionB, cleanup := setupImportTestFixture(t)
	defer cleanup()

	if err := SaveItemHistoryWithCount("Butter", sectionA.ID, 50); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SaveItemHistoryWithCount("butter", sectionB.ID, 10); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	suggestions, err := GetItemSuggestions("Butter", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].UsageCount != 50 {
		t.Errorf("re-import with lower count should preserve existing 50, got %d", suggestions[0].UsageCount)
	}
}

func testImportAdoptsHigherCount(t *testing.T) {
	sectionA, _, cleanup := setupImportTestFixture(t)
	defer cleanup()

	if err := SaveItemHistoryWithCount("Eggs", sectionA.ID, 10); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SaveItemHistoryWithCount("eggs", sectionA.ID, 100); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	suggestions, err := GetItemSuggestions("Eggs", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].UsageCount != 100 {
		t.Errorf("re-import with higher count should adopt 100, got %d", suggestions[0].UsageCount)
	}
}

func testImportPreservesSectionWhenZero(t *testing.T) {
	sectionA, _, cleanup := setupImportTestFixture(t)
	defer cleanup()

	if err := SaveItemHistoryWithCount("Cheese", sectionA.ID, 5); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SaveItemHistoryWithCount("cheese", 0, 5); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	suggestions, err := GetItemSuggestions("Cheese", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].LastSectionID == 0 {
		t.Errorf("re-import with section_id=0 should preserve existing section %d, got 0", sectionA.ID)
	}
}

// ---------- Templates ----------

func testTemplates(t *testing.T) {
	tmpl, err := CreateTemplate("Weekly Shop", "Standard grocery template")
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if tmpl.ID <= 0 {
		t.Error("expected ID > 0")
	}

	item, err := AddTemplateItem(tmpl.ID, "Dairy", "Milk", "2L")
	if err != nil {
		t.Fatalf("AddTemplateItem: %v", err)
	}
	if item.ID <= 0 {
		t.Error("expected item ID > 0")
	}

	fetched, err := GetTemplateByID(tmpl.ID)
	if err != nil {
		t.Fatalf("GetTemplateByID: %v", err)
	}
	if len(fetched.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(fetched.Items))
	}

	if err := DeleteTemplate(tmpl.ID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
}

// ---------- Sessions ----------

func testSessions(t *testing.T) {
	id := "test-session-id-12345"
	expiresAt := int64(9999999999)

	if err := CreateSession(id, expiresAt); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	session, err := GetSession(id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.ID != id {
		t.Errorf("ID: got %q want %q", session.ID, id)
	}
	if session.ExpiresAt != expiresAt {
		t.Errorf("ExpiresAt: got %d want %d", session.ExpiresAt, expiresAt)
	}

	if err := DeleteSession(id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	_, err = GetSession(id)
	if err == nil {
		t.Error("expected error after delete")
	}
}

// ---------- Clear All Data ----------

func testClearAllData(t *testing.T) {
	// Create some data
	list, err := CreateList("Clear Test", "🛒")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := SetActiveList(list.ID); err != nil {
		t.Fatalf("SetActiveList: %v", err)
	}
	section, err := CreateSectionForList(list.ID, "Test Section")
	if err != nil {
		t.Fatalf("CreateSectionForList: %v", err)
	}
	if _, err := CreateItem(section.ID, "Test Item", "", 0); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	// Clear everything
	if err := ClearAllData(); err != nil {
		t.Fatalf("ClearAllData: %v", err)
	}

	// Verify lists are gone
	lists, err := GetAllLists()
	if err != nil {
		t.Fatalf("GetAllLists after clear: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("expected 0 lists, got %d", len(lists))
	}
}
