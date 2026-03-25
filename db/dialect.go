package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Driver holds the active database driver name.
// Values: "sqlite" or "postgres".
// Set by Init() in db.go.
var Driver string

// TimestampNow returns the current Unix timestamp.
// Use this as a query parameter instead of strftime('%s','now') in SQL.
func TimestampNow() int64 {
	return time.Now().Unix()
}

// InsertReturningID executes an INSERT and returns the new row's ID.
// The query must use ? placeholders; callers must NOT pre-rebind.
// This function handles rebinding internally for both SQLite and Postgres.
// Works with both *sqlx.DB and *sqlx.Tx via sqlx.Ext.
func InsertReturningID(ext sqlx.Ext, query string, args ...interface{}) (int64, error) {
	if Driver == "postgres" {
		rebound := sqlx.Rebind(sqlx.DOLLAR, query)
		var id int64
		err := ext.QueryRowx(rebound+" RETURNING id", args...).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	// SQLite path
	rebound := sqlx.Rebind(sqlx.QUESTION, query)
	result, err := ext.Exec(rebound, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// TableExists reports whether a table with the given name exists in the database.
func TableExists(tableName string) (bool, error) {
	var count int
	var err error

	if Driver == "postgres" {
		err = DB.QueryRow(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1",
			tableName,
		).Scan(&count)
	} else {
		err = DB.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			tableName,
		).Scan(&count)
	}

	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return count > 0, nil
}

// isValidIdentifier validates that a string is a valid SQL identifier.
// First character must be a letter or underscore.
// Subsequent characters must be letters, digits, or underscores.
func isValidIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			// First character: letter or underscore
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			// Subsequent characters: letter, digit, or underscore
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// ColumnExists reports whether a column exists in the given table.
func ColumnExists(tableName, columnName string) (bool, error) {
	if !isValidIdentifier(tableName) {
		return false, fmt.Errorf("invalid table name: %q", tableName)
	}

	var count int
	var err error

	if Driver == "postgres" {
		err = DB.QueryRow(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name=$1 AND column_name=$2 AND table_schema='public'",
			tableName,
			columnName,
		).Scan(&count)
	} else {
		// pragma_table_info takes the table name as part of the SQL string, not a bind param.
		q := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name=?", tableName)
		err = DB.QueryRow(q, columnName).Scan(&count)
	}

	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return count > 0, nil
}

// UpsertHistoryQuery returns the dialect-specific ON CONFLICT clause for item_history upserts.
// The caller is responsible for building the full INSERT statement prefix.
// Uses EXCLUDED.last_used_at to reference the inserted value — avoids positional parameter issues.
func UpsertHistoryQuery() string {
	if Driver == "postgres" {
		return `ON CONFLICT ((LOWER(name))) DO UPDATE SET last_section_id = EXCLUDED.last_section_id, usage_count = item_history.usage_count + 1, last_used_at = EXCLUDED.last_used_at`
	}
	return `ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET last_section_id = excluded.last_section_id, usage_count = usage_count + 1, last_used_at = excluded.last_used_at`
}

// UpsertHistoryImportQuery returns the dialect-specific ON CONFLICT clause for item_history imports.
// Used by SaveItemHistoryWithCount and SaveItemHistoryWithCountTx to preserve higher counts and sections.
// Preserves last_section_id when imported value is 0, takes MAX of usage_count.
func UpsertHistoryImportQuery() string {
	if Driver == "postgres" {
		return `ON CONFLICT ((LOWER(name))) DO UPDATE SET last_section_id = CASE WHEN EXCLUDED.last_section_id > 0 THEN EXCLUDED.last_section_id ELSE item_history.last_section_id END, usage_count = CASE WHEN EXCLUDED.usage_count > item_history.usage_count THEN EXCLUDED.usage_count ELSE item_history.usage_count END, last_used_at = EXCLUDED.last_used_at`
	}
	return `ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET last_section_id = CASE WHEN excluded.last_section_id > 0 THEN excluded.last_section_id ELSE last_section_id END, usage_count = CASE WHEN excluded.usage_count > usage_count THEN excluded.usage_count ELSE usage_count END, last_used_at = excluded.last_used_at`
}

// caseInsensitiveOrder returns the dialect-specific ORDER BY clause for case-insensitive sorting.
// For Postgres: LOWER(column) direction
// For SQLite: column COLLATE NOCASE direction
func caseInsensitiveOrder(column, direction string) string {
	if Driver == "postgres" {
		return fmt.Sprintf("LOWER(%s) %s", column, direction)
	}
	return fmt.Sprintf("%s COLLATE NOCASE %s", column, direction)
}
