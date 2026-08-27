package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestLegacyDatabaseFixtureMigratesWithoutFalseAttribution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := `
		CREATE TABLE sections(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,sort_order INTEGER NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at INTEGER DEFAULT 0);
		CREATE TABLE items(id INTEGER PRIMARY KEY AUTOINCREMENT,section_id INTEGER NOT NULL,name TEXT NOT NULL,description TEXT DEFAULT '',completed BOOLEAN DEFAULT FALSE,uncertain BOOLEAN DEFAULT FALSE,sort_order INTEGER NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at INTEGER DEFAULT 0);
		CREATE TABLE sessions(id TEXT PRIMARY KEY,expires_at INTEGER NOT NULL);
		CREATE TABLE item_history(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL COLLATE NOCASE,last_section_id INTEGER,usage_count INTEGER DEFAULT 1,last_used_at INTEGER DEFAULT 0,UNIQUE(name COLLATE NOCASE));
		INSERT INTO sections(id,name,sort_order) VALUES(1,'Legacy section',0);
		INSERT INTO items(id,section_id,name,sort_order) VALUES(1,1,'Legacy item',0);
		INSERT INTO item_history(name,last_section_id) VALUES('Legacy suggestion',1);
		INSERT INTO item_history(name,last_section_id) VALUES('Orphaned legacy suggestion',999);`
	if _, err = legacy.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	t.Setenv("DB_PATH", path)
	t.Setenv("ADMIN_USERNAME", "fixture-admin")
	t.Setenv("ADMIN_PASSWORD", "fixture-password")
	Init()
	t.Cleanup(Close)

	item, err := GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.CreatedBy != nil || item.UpdatedBy != nil || item.CreatedByUserID != nil || item.UpdatedByUserID != nil {
		t.Fatalf("legacy item was falsely attributed: %#v", item)
	}
	lists, err := GetAllLists()
	if err != nil || len(lists) != 1 || lists[0].OwnerUserID == 0 {
		t.Fatalf("legacy list migration failed: lists=%#v err=%v", lists, err)
	}
	var migratedSectionID sql.NullInt64
	var migratedUserID sql.NullInt64
	if err = DB.QueryRow(`SELECT last_section_id,user_id FROM item_history WHERE name='Orphaned legacy suggestion'`).Scan(&migratedSectionID, &migratedUserID); err != nil {
		t.Fatalf("orphaned history entry was not preserved: %v", err)
	}
	if migratedSectionID.Valid {
		t.Fatalf("orphaned history section reference was preserved: %d", migratedSectionID.Int64)
	}
	if !migratedUserID.Valid || migratedUserID.Int64 != lists[0].OwnerUserID {
		t.Fatalf("migrated history owner=%v, want %d", migratedUserID, lists[0].OwnerUserID)
	}

	// Running the already-completed migrations again must preserve the fixture.
	Close()
	Init()
	item, err = GetItemByID(1)
	if err != nil || item.Name != "Legacy item" || item.CreatedBy != nil {
		t.Fatalf("idempotent migration changed legacy item: item=%#v err=%v", item, err)
	}
}
