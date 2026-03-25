package db

import (
	"log"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var DB *sqlx.DB

func Init() {
	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL != "" {
		// Postgres path
		Driver = "postgres"
		dbPath := os.Getenv("DB_PATH")
		if dbPath != "" {
			log.Println("Warning: DB_PATH is ignored when DATABASE_URL is set")
		}
		var err error
		DB, err = sqlx.Open("pgx", databaseURL)
		if err != nil {
			log.Fatal("Failed to open Postgres connection:", err)
		}
		if err = DB.Ping(); err != nil {
			log.Fatal("Failed to ping Postgres database:", err)
		}
	} else {
		// SQLite path (default)
		Driver = "sqlite"
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "./shopping.db"
		}
		// Create parent directory if needed
		dir := filepath.Dir(dbPath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Fatal("Failed to create database directory:", err)
			}
		}
		var err error
		DB, err = sqlx.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
		if err != nil {
			log.Fatal("Failed to connect to database:", err)
		}
		if err = DB.Ping(); err != nil {
			log.Fatal("Failed to ping database:", err)
		}
		// SQLite-specific PRAGMAs
		_, err = DB.Exec("PRAGMA journal_mode=WAL")
		if err != nil {
			log.Println("Warning: Could not enable WAL mode:", err)
		}
		_, err = DB.Exec("PRAGMA busy_timeout=5000")
		if err != nil {
			log.Println("Warning: Could not set busy timeout:", err)
		}
	}

	createTables()

	if Driver == "sqlite" {
		log.Println("Database initialized successfully (WAL mode)")
	} else {
		log.Println("Database initialized successfully (Postgres)")
	}
}

func createTables() {
	if Driver == "postgres" {
		createTablesPostgres()
	} else {
		createTablesSQLite()
	}
}

func createTablesSQLite() {
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

	runMigrations()
}

func createTablesPostgres() {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS lists (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT NOT NULL DEFAULT '🛒',
			sort_order INTEGER NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`,
		`CREATE TABLE IF NOT EXISTS sections (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			sort_mode TEXT DEFAULT 'manual',
			list_id BIGINT REFERENCES lists(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`,
		`CREATE TABLE IF NOT EXISTS items (
			id BIGSERIAL PRIMARY KEY,
			section_id BIGINT NOT NULL REFERENCES sections(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			completed BOOLEAN DEFAULT FALSE,
			uncertain BOOLEAN DEFAULT FALSE,
			quantity INTEGER DEFAULT 0,
			sort_order INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			expires_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS item_history (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			last_section_id BIGINT,
			usage_count INTEGER DEFAULT 1,
			last_used_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`,
		`CREATE TABLE IF NOT EXISTS templates (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`,
		`CREATE TABLE IF NOT EXISTS template_items (
			id BIGSERIAL PRIMARY KEY,
			template_id BIGINT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
			section_name TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_items_section ON items(section_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_order ON sections(sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_list ON sections(list_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_lists_order ON lists(sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_lists_active ON lists(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_templates_order ON templates(sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_template_items_template ON template_items(template_id, sort_order)`,
		// Functional unique index for case-insensitive name uniqueness
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_item_history_name_lower ON item_history (LOWER(name))`,
	}

	for _, stmt := range statements {
		if _, err := DB.Exec(stmt); err != nil {
			log.Fatalf("Failed to create Postgres schema: %v\nStatement: %s", err, stmt)
		}
	}
	log.Println("Database schema created successfully (Postgres)")

	runMigrations()
}

func runMigrations() {
	// Migration: Add updated_at to sections
	exists, err := ColumnExists("sections", "updated_at")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if !exists {
		log.Println("Running migration: Adding updated_at to sections...")
		if _, err := DB.Exec("ALTER TABLE sections ADD COLUMN updated_at INTEGER"); err != nil {
			log.Println("Migration failed for sections:", err)
		} else {
			if _, err := DB.Exec(DB.Rebind("UPDATE sections SET updated_at = ?"), TimestampNow()); err != nil {
				log.Printf("WARNING: Migration UPDATE failed for sections: %v", err)
			}
			log.Println("Migration completed: sections.updated_at added")
		}
	}

	// Migration: Add updated_at to items
	exists, err = ColumnExists("items", "updated_at")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if !exists {
		log.Println("Running migration: Adding updated_at to items...")
		if _, err := DB.Exec("ALTER TABLE items ADD COLUMN updated_at INTEGER"); err != nil {
			log.Println("Migration failed for items:", err)
		} else {
			if _, err := DB.Exec(DB.Rebind("UPDATE items SET updated_at = ?"), TimestampNow()); err != nil {
				log.Printf("WARNING: Migration UPDATE failed for items: %v", err)
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

	// Migration: Add quantity to items
	migrateItemQuantity()

	// Migration: Add sort_mode to sections
	migrateSectionSortMode()
}

func migrateToMultipleLists() {
	exists, err := TableExists("lists")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if exists {
		return // Already migrated
	}

	log.Println("Running migration: Adding multiple lists support...")

	// Create lists table — driver-specific DDL
	var createErr error
	if Driver == "postgres" {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS lists (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT NOT NULL DEFAULT '🛒',
			sort_order INTEGER NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`)
	} else {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			is_active BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at INTEGER DEFAULT (strftime('%s', 'now'))
		)`)
	}
	if createErr != nil {
		log.Println("Migration failed - creating lists table:", createErr)
		return
	}

	if _, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_lists_order ON lists(sort_order)"); err != nil {
		log.Println("Migration warning - creating lists order index:", err)
	}
	if _, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_lists_active ON lists(is_active)"); err != nil {
		log.Println("Migration warning - creating lists active index:", err)
	}

	// Add list_id to sections
	if _, err = DB.Exec("ALTER TABLE sections ADD COLUMN list_id INTEGER REFERENCES lists(id) ON DELETE CASCADE"); err != nil {
		log.Println("Migration failed - adding list_id to sections:", err)
		return
	}

	if _, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_sections_list ON sections(list_id, sort_order)"); err != nil {
		log.Println("Migration warning - creating sections list index:", err)
	}

	log.Println("Migration completed: Multiple lists support added")
}

func migrateTemplates() {
	exists, err := TableExists("templates")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if exists {
		return // Already migrated
	}

	log.Println("Running migration: Adding templates support...")

	var createErr error
	if Driver == "postgres" {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS templates (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at BIGINT DEFAULT EXTRACT(EPOCH FROM NOW())::bigint
		)`)
	} else {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at INTEGER DEFAULT (strftime('%s', 'now'))
		)`)
	}
	if createErr != nil {
		log.Println("Migration failed - creating templates table:", createErr)
		return
	}
	if _, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_templates_order ON templates(sort_order)"); err != nil {
		log.Println("Migration warning:", err)
	}

	if Driver == "postgres" {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS template_items (
			id BIGSERIAL PRIMARY KEY,
			template_id BIGINT NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
			section_name TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`)
	} else {
		_, createErr = DB.Exec(`CREATE TABLE IF NOT EXISTS template_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			template_id INTEGER NOT NULL,
			section_name TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			sort_order INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE
		)`)
	}
	if createErr != nil {
		log.Println("Migration failed - creating template_items table:", createErr)
		return
	}
	if _, err = DB.Exec("CREATE INDEX IF NOT EXISTS idx_template_items_template ON template_items(template_id, sort_order)"); err != nil {
		log.Println("Migration warning:", err)
	}

	log.Println("Migration completed: Templates support added")
}

func migrateListIcons() {
	exists, err := ColumnExists("lists", "icon")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if exists {
		return
	}
	log.Println("Running migration: Adding icon to lists...")
	if _, err = DB.Exec(`ALTER TABLE lists ADD COLUMN icon TEXT DEFAULT '🛒'`); err != nil {
		log.Println("Migration failed - adding icon to lists:", err)
		return
	}
	log.Println("Migration completed: List icons added")
}

func migrateItemQuantity() {
	exists, err := ColumnExists("items", "quantity")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if exists {
		return
	}
	log.Println("Running migration: Adding quantity to items...")
	if _, err = DB.Exec("ALTER TABLE items ADD COLUMN quantity INTEGER DEFAULT 0"); err != nil {
		log.Println("Migration failed - adding quantity to items:", err)
		return
	}
	log.Println("Migration completed: Item quantity added")
}

func migrateSectionSortMode() {
	exists, err := ColumnExists("sections", "sort_mode")
	if err != nil {
		log.Println("Migration check failed:", err)
		return
	}
	if exists {
		return
	}
	log.Println("Running migration: Adding sort_mode to sections...")
	if _, err = DB.Exec("ALTER TABLE sections ADD COLUMN sort_mode TEXT DEFAULT 'manual'"); err != nil {
		log.Println("Migration failed - adding sort_mode to sections:", err)
		return
	}
	log.Println("Migration completed: Section sort_mode added")
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
