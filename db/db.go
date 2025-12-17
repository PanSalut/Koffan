package db

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./shopping.db"
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Test connection
	if err = DB.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	// Create tables
	createTables()

	log.Println("Database initialized successfully")
}

func createTables() {
	schema := `
	CREATE TABLE IF NOT EXISTS sections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		section_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		completed BOOLEAN DEFAULT FALSE,
		uncertain BOOLEAN DEFAULT FALSE,
		sort_order INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (section_id) REFERENCES sections(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		expires_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mobile_helper TEXT DEFAULT 'button',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_items_section ON items(section_id, sort_order);
	CREATE INDEX IF NOT EXISTS idx_sections_order ON sections(sort_order);

	-- Insert default preferences if not exists
	INSERT OR IGNORE INTO preferences (id, mobile_helper) VALUES (1, 'button');
	`

	_, err := DB.Exec(schema)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
