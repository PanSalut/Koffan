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
// Works with both *sqlx.DB and *sqlx.Tx via sqlx.Ext.
// For SQLite: uses LastInsertId(). For Postgres: appends RETURNING id.
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

// ColumnExists reports whether a column exists in the given table.
func ColumnExists(tableName, columnName string) (bool, error) {
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
