package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./shopping.db"
	}

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatal("Failed to create database directory:", err)
		}
	}

	var err error
	// Enable WAL mode and foreign keys for better concurrency
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Test connection
	if err = DB.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	// Enable WAL mode explicitly (in case pragma wasn't applied via connection string)
	_, err = DB.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		log.Println("Warning: Could not enable WAL mode:", err)
	}

	// Set busy timeout to 5 seconds
	_, err = DB.Exec("PRAGMA busy_timeout=5000")
	if err != nil {
		log.Println("Warning: Could not set busy timeout:", err)
	}

	// Create tables
	createTables()

	log.Println("Database initialized successfully (WAL mode)")
}

func createTables() {
	schema := `
	CREATE TABLE IF NOT EXISTS sections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
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
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (section_id) REFERENCES sections(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		expires_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS item_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL COLLATE NOCASE,
		last_section_id INTEGER,
		usage_count INTEGER DEFAULT 1,
		last_used_at INTEGER DEFAULT (strftime('%s', 'now')),
		UNIQUE(name COLLATE NOCASE)
	);

	CREATE INDEX IF NOT EXISTS idx_items_section ON items(section_id, sort_order);
	CREATE INDEX IF NOT EXISTS idx_sections_order ON sections(sort_order);
	CREATE INDEX IF NOT EXISTS idx_item_history_name ON item_history(name COLLATE NOCASE);
	`

	_, err := DB.Exec(schema)
	if err != nil {
		log.Fatal("Failed to create tables:", err)
	}

	// Migration: Add updated_at column if it doesn't exist
	runMigrations()
}

func runMigrations() {
	// Check if updated_at column exists in sections
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sections') WHERE name='updated_at'").Scan(&count)
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}

	if count == 0 {
		log.Println("Running migration: Adding updated_at to sections...")
		// SQLite doesn't support dynamic DEFAULT in ALTER TABLE, so add with NULL first
		_, err := DB.Exec("ALTER TABLE sections ADD COLUMN updated_at INTEGER")
		if err != nil {
			log.Println("Migration failed for sections:", err)
		} else {
			// Set updated_at for existing rows
			_, updateErr := DB.Exec("UPDATE sections SET updated_at = strftime('%s', 'now')")
			if updateErr != nil {
				log.Printf("WARNING: Migration UPDATE failed for sections: %v", updateErr)
			}
			log.Println("Migration completed: sections.updated_at added")
		}
	}

	// Check if updated_at column exists in items
	err = DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='updated_at'").Scan(&count)
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}

	if count == 0 {
		log.Println("Running migration: Adding updated_at to items...")
		// SQLite doesn't support dynamic DEFAULT in ALTER TABLE, so add with NULL first
		_, err := DB.Exec("ALTER TABLE items ADD COLUMN updated_at INTEGER")
		if err != nil {
			log.Println("Migration failed for items:", err)
		} else {
			// Set updated_at for existing rows
			_, updateErr := DB.Exec("UPDATE items SET updated_at = strftime('%s', 'now')")
			if updateErr != nil {
				log.Printf("WARNING: Migration UPDATE failed for items: %v", updateErr)
			}
			log.Println("Migration completed: items.updated_at added")
		}
	}

	// Migration: Multiple lists support
	migrateToMultipleLists()

	// Migration: Templates support
	migrateTemplates()

	// Migration: Add icon to lists
	migrateListIcons()
}

func migrateToMultipleLists() {
	// Check if lists table exists
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lists'").Scan(&count)
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}

	if count > 0 {
		return // Already migrated
	}

	log.Println("Running migration: Adding multiple lists support...")

	// Create lists table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			is_active BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at INTEGER DEFAULT (strftime('%s', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_lists_order ON lists(sort_order);
		CREATE INDEX IF NOT EXISTS idx_lists_active ON lists(is_active);
	`)
	if err != nil {
		log.Println("Migration failed - creating lists table:", err)
		return
	}

	// Add list_id column to sections
	_, err = DB.Exec("ALTER TABLE sections ADD COLUMN list_id INTEGER REFERENCES lists(id) ON DELETE CASCADE")
	if err != nil {
		log.Println("Migration failed - adding list_id to sections:", err)
		return
	}

	// Create index for list_id
	_, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_sections_list ON sections(list_id, sort_order)")
	if err != nil {
		log.Println("Migration warning - creating sections list index:", err)
	}

	log.Println("Migration completed: Multiple lists support added")
}

func migrateTemplates() {
	// Check if templates table exists
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='templates'").Scan(&count)
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}

	if count > 0 {
		return // Already migrated
	}

	log.Println("Running migration: Adding templates support...")

	// Create templates table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at INTEGER DEFAULT (strftime('%s', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_templates_order ON templates(sort_order);
	`)
	if err != nil {
		log.Println("Migration failed - creating templates table:", err)
		return
	}

	// Create template_items table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS template_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id INTEGER NOT NULL,
			section_name TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_template_items_template ON template_items(template_id, sort_order);
	`)
	if err != nil {
		log.Println("Migration failed - creating template_items table:", err)
		return
	}

	log.Println("Migration completed: Templates support added")
}

func migrateListIcons() {
	// Check if icon column exists in lists
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('lists') WHERE name='icon'").Scan(&count)
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}

	if count > 0 {
		return // Already migrated
	}

	log.Println("Running migration: Adding icon to lists...")

	_, err = DB.Exec("ALTER TABLE lists ADD COLUMN icon TEXT DEFAULT '🛒'")
	if err != nil {
		log.Println("Migration failed - adding icon to lists:", err)
		return
	}

	log.Println("Migration completed: List icons added")
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
