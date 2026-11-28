package db

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const MaxOfflineOperations = 500
const maxOfflineInteger = int64(9007199254740991)

// OfflineOperation carries explicit field changes, never relative toggles.
// Its ID and payload must stay unchanged until the server acknowledges it.
type OfflineOperation struct {
	Prepend  bool                       `json:"prepend,omitempty"`
	ID       string                     `json:"id"`
	Entity   string                     `json:"entity"`
	Action   string                     `json:"action"`
	EntityID int64                      `json:"entity_id"`
	Values   map[string]json.RawMessage `json:"values"`
}

type OfflineSyncRequest struct {
	ClientID   string             `json:"client_id"`
	Operations []OfflineOperation `json:"operations"`
}

type OfflineResult struct {
	ID       string `json:"id"`
	Entity   string `json:"entity"`
	EntityID int64  `json:"entity_id"`
	ServerID int64  `json:"server_id"`
}

type OfflineSnapshot struct {
	Lists    []List    `json:"lists"`
	Sections []Section `json:"sections"`
	Items    []Item    `json:"items"`
	// GeneratedAt (Unix milliseconds) lets clients tell a fresh snapshot from
	// an older copy, such as a page restored from the offline cache. Handlers
	// stamp it when they send a snapshot; it is not part of the data.
	GeneratedAt int64 `json:"generated_at,omitempty"`
}

type OfflineSyncResponse struct {
	Results  []OfflineResult `json:"results"`
	Snapshot OfflineSnapshot `json:"snapshot"`
	Changed  bool            `json:"-"`
}

// OfflineError distinguishes a malformed request from an edit whose target
// was deleted by another shopper. Neither error commits any part of a batch.
type OfflineError struct {
	Conflict    bool
	OperationID string
	Message     string
}

func (err *OfflineError) Error() string { return err.Message }

// OfflineItemEvent captures parent context inside the same transaction,
// including before cascading list and section deletes.
type OfflineItemEvent struct {
	Event   string
	Item    Item
	Section struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	List struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
}

type OfflineItemHook func(*sql.Tx, OfflineItemEvent) error

func offlineInvalid(message string) error  { return &OfflineError{Message: message} }
func offlineConflict(message string) error { return &OfflineError{Conflict: true, Message: message} }

func validOfflineUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && len(value) == 36 && parsed.String() == strings.ToLower(value)
}

// ApplyOfflineSync commits the mutations, temporary IDs, operation receipts and
// optional webhook outbox records together. A lost HTTP response is safe to retry.
func ApplyOfflineSync(workspaceID int64, request OfflineSyncRequest, hook OfflineItemHook) (*OfflineSyncResponse, error) {
	if !validOfflineUUID(request.ClientID) {
		return nil, offlineInvalid("A valid client_id UUID is required")
	}
	if len(request.Operations) > MaxOfflineOperations {
		return nil, offlineInvalid("Too many offline operations (maximum 500)")
	}
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	response := &OfflineSyncResponse{Results: make([]OfflineResult, 0, len(request.Operations))}
	for _, operation := range request.Operations {
		result, changed, err := applyOfflineOperation(tx, workspaceID, request.ClientID, operation, hook)
		if err != nil {
			var inputErr *OfflineError
			if errors.As(err, &inputErr) {
				inputErr.OperationID = operation.ID
			}
			return nil, err
		}
		response.Results = append(response.Results, result)
		response.Changed = response.Changed || changed
	}
	response.Snapshot, err = offlineSnapshotTx(tx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return response, nil
}

func applyOfflineOperation(tx *sql.Tx, workspaceID int64, clientID string, operation OfflineOperation, hook OfflineItemHook) (OfflineResult, bool, error) {
	result := OfflineResult{ID: operation.ID, Entity: operation.Entity, EntityID: operation.EntityID}
	if !validOfflineUUID(operation.ID) {
		return result, false, offlineInvalid("A valid operation UUID is required")
	}
	table, ok := map[string]string{"list": "lists", "section": "sections", "item": "items"}[operation.Entity]
	if !ok {
		return result, false, offlineInvalid("Unknown offline entity")
	}
	if operation.Action != "create" && operation.Action != "update" && operation.Action != "delete" {
		return result, false, offlineInvalid("Unknown offline action")
	}
	if operation.EntityID == 0 || operation.EntityID > maxOfflineInteger || operation.EntityID < -maxOfflineInteger {
		return result, false, offlineInvalid("entity_id must be a nonzero safe integer")
	}
	if operation.Action == "create" && operation.EntityID >= 0 {
		return result, false, offlineInvalid("Creates require a negative temporary entity_id")
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		return result, false, offlineInvalid("Invalid operation values")
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	var previousFingerprint string
	err = tx.QueryRow(`SELECT request_hash, server_id FROM offline_operations WHERE client_id = ? AND operation_id = ?`, clientID, operation.ID).Scan(&previousFingerprint, &result.ServerID)
	if err == nil {
		if fingerprint != previousFingerprint {
			return result, false, offlineConflict("Operation ID was already used with different changes")
		}
		return result, false, nil
	}
	if err != sql.ErrNoRows {
		return result, false, err
	}
	values, err := validateOfflineValues(operation)
	if err != nil {
		return result, false, err
	}

	if operation.Action != "create" {
		result.ServerID, err = resolveOfflineID(tx, clientID, operation.Entity, operation.EntityID)
		if err != nil {
			return result, false, err
		}
	}
	for field, entity := range map[string]string{"list_id": "list", "section_id": "section"} {
		if rawID, exists := values[field]; exists {
			parentID, err := resolveOfflineID(tx, clientID, entity, rawID.(int64))
			if err != nil {
				return result, false, err
			}
			parentTable := "lists"
			if entity == "section" {
				parentTable = "sections"
			}
			if exists, err := offlineEntityExists(tx, workspaceID, parentTable, parentID); err != nil {
				return result, false, err
			} else if !exists {
				return result, false, offlineConflict("The destination " + entity + " no longer exists")
			}
			values[field] = parentID
		}
	}
	if operation.Entity == "list" {
		if name, supplied := values["name"]; supplied {
			var count int
			if err := tx.QueryRow("SELECT COUNT(*) FROM lists WHERE workspace_id = ? AND LOWER(name) = LOWER(?) AND id != ?", workspaceID, name, result.ServerID).Scan(&count); err != nil {
				return result, false, err
			}
			if count > 0 {
				return result, false, offlineConflict("A list with this name already exists")
			}
		}
	}
	changed := false
	completionChanged := false
	var events []OfflineItemEvent
	if operation.Action == "delete" {
		owned, err := offlineEntityExists(tx, workspaceID, table, result.ServerID)
		if err != nil {
			return result, false, err
		}
		// An entity of another workspace is treated exactly like a missing one.
		if hook != nil && owned {
			events, err = offlineItemEvents(tx, operation.Entity, result.ServerID, "item.deleted")
			if err != nil {
				return result, false, err
			}
		}
		// owned was just checked in this transaction, so the delete needs no
		// second ownership subquery over the whole workspace.
		if owned {
			deleted, err := tx.Exec("DELETE FROM "+table+" WHERE id = ?", result.ServerID)
			if err != nil {
				return result, false, err
			}
			count, err := deleted.RowsAffected()
			if err != nil {
				return result, false, err
			}
			changed = count > 0
		}
	} else if operation.Action == "create" {
		var existingID int64
		err := tx.QueryRow("SELECT server_id FROM offline_entity_ids WHERE client_id = ? AND entity = ? AND temporary_id = ?", clientID, operation.Entity, operation.EntityID).Scan(&existingID)
		if err == nil {
			return result, false, offlineConflict("Temporary ID was already used by another create operation")
		}
		if err != sql.ErrNoRows {
			return result, false, err
		}
		if operation.Entity == "item" && operation.Prepend {
			var first, last int64
			if err := tx.QueryRow("SELECT MIN(0, COALESCE(MIN(sort_order), 0)), COALESCE(MAX(sort_order), 0) FROM items WHERE section_id = ?", values["section_id"]).Scan(&first, &last); err != nil {
				return result, false, err
			}
			if last >= maxOfflineInteger {
				return result, false, offlineConflict("Cannot shift the maximum sort_order")
			}
			if _, err := tx.Exec("UPDATE items SET sort_order = sort_order + 1 WHERE section_id = ?", values["section_id"]); err != nil {
				return result, false, err
			}
			values["sort_order"] = first
		} else if _, supplied := values["sort_order"]; !supplied {
			query := "SELECT COALESCE(MAX(sort_order), -1) + 1 FROM " + table
			var args []interface{}
			if operation.Entity == "list" {
				query += " WHERE workspace_id = ?"
				args = append(args, workspaceID)
			}
			if operation.Entity == "section" {
				query += " WHERE list_id = ?"
				args = append(args, values["list_id"])
			}
			if operation.Entity == "item" {
				query += " WHERE section_id = ?"
				args = append(args, values["section_id"])
			}
			var order int64
			if err := tx.QueryRow(query, args...).Scan(&order); err != nil {
				return result, false, err
			}
			if order > maxOfflineInteger {
				return result, false, offlineConflict("Cannot append after the maximum sort_order")
			}
			values["sort_order"] = order
		}
		if operation.Entity == "list" {
			if _, supplied := values["icon"]; !supplied {
				values["icon"] = "🛒"
			}
			values["workspace_id"] = workspaceID
		}
		fields, args := offlineFields(values)
		placeholders := strings.TrimRight(strings.Repeat("?,", len(fields)), ",")
		created, err := tx.Exec("INSERT INTO "+table+" ("+strings.Join(fields, ",")+") VALUES ("+placeholders+")", args...)
		if err != nil {
			return result, false, err
		}
		result.ServerID, err = created.LastInsertId()
		if err != nil {
			return result, false, err
		}
		if _, err := tx.Exec(`INSERT INTO offline_entity_ids (client_id, entity, temporary_id, server_id) VALUES (?, ?, ?, ?)`, clientID, operation.Entity, operation.EntityID, result.ServerID); err != nil {
			return result, false, err
		}
		changed = true
		if operation.Entity == "item" {
			if _, err := tx.Exec(`INSERT INTO item_history (workspace_id, name, last_section_id, usage_count) VALUES (?, ?, ?, 1)
				ON CONFLICT(workspace_id, name COLLATE NOCASE) DO UPDATE SET usage_count = usage_count + 1, last_section_id = excluded.last_section_id, last_used_at = strftime('%s', 'now')`, workspaceID, values["name"], values["section_id"]); err != nil {
				return result, false, err
			}
		}
	} else {
		exists, err := offlineEntityExists(tx, workspaceID, table, result.ServerID)
		if err != nil {
			return result, false, err
		}
		if !exists {
			return result, false, offlineConflict("The " + operation.Entity + " was deleted by another shopper")
		}
		if operation.Entity == "item" && values["completed"] == true {
			var wasCompleted bool
			if err := tx.QueryRow("SELECT completed FROM items WHERE id = ?", result.ServerID).Scan(&wasCompleted); err != nil {
				return result, false, err
			}
			completionChanged = !wasCompleted
		}
		if len(values) > 0 {
			fields, args := offlineFields(values)
			assignments, differences := make([]string, len(fields)), make([]string, len(fields))
			for index, field := range fields {
				assignments[index] = field + " = ?"
				differences[index] = field + " IS NOT ?"
			}
			queryArgs := append(append(append([]interface{}{}, args...), result.ServerID), args...)
			updated, err := tx.Exec("UPDATE "+table+" SET "+strings.Join(assignments, ",")+", updated_at = strftime('%s', 'now') WHERE id = ? AND ("+strings.Join(differences, " OR ")+")", queryArgs...)
			if err != nil {
				return result, false, err
			}
			count, err := updated.RowsAffected()
			if err != nil {
				return result, false, err
			}
			changed = count > 0
		}
	}
	if operation.Entity == "list" && operation.Action != "delete" && values["is_active"] == true {
		// Activating a list is exclusive, including when it is created offline.
		// Receipt replays return earlier, so they cannot undo a later activation.
		deactivated, err := tx.Exec("UPDATE lists SET is_active = FALSE, updated_at = strftime('%s', 'now') WHERE id != ? AND is_active = TRUE AND workspace_id = ?", result.ServerID, workspaceID)
		if err != nil {
			return result, false, err
		}
		count, err := deactivated.RowsAffected()
		if err != nil {
			return result, false, err
		}
		changed = changed || count > 0
	}
	if hook != nil && changed {
		if operation.Entity == "item" && operation.Action != "delete" {
			event := "item.updated"
			if operation.Action == "create" {
				event = "item.created"
			} else if completionChanged {
				event = "item.completed"
			}
			events, err = offlineItemEvents(tx, "item", result.ServerID, event)
			if err != nil {
				return result, false, err
			}
		}
		for _, event := range events {
			if err := hook(tx, event); err != nil {
				return result, false, err
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO offline_operations (client_id, operation_id, request_hash, server_id) VALUES (?, ?, ?, ?)`, clientID, operation.ID, fingerprint, result.ServerID); err != nil {
		return result, false, err
	}
	return result, changed, nil
}

// offlineOwnedSQL selects the IDs of a table's rows that belong to a workspace
// (bind: workspace id). table comes from a fixed map, never from user input.
func offlineOwnedSQL(table string) string {
	switch table {
	case "lists":
		return "SELECT id FROM lists WHERE workspace_id = ?"
	case "sections":
		return "SELECT s.id FROM sections s JOIN lists l ON l.id = s.list_id WHERE l.workspace_id = ?"
	default:
		return "SELECT i.id FROM items i JOIN sections s ON s.id = i.section_id JOIN lists l ON l.id = s.list_id WHERE l.workspace_id = ?"
	}
}

// offlineEntityExists reports whether the row exists inside the workspace.
// Rows of other workspaces are indistinguishable from missing ones.
func offlineEntityExists(tx *sql.Tx, workspaceID int64, table string, id int64) (bool, error) {
	var exists bool
	err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM ("+offlineOwnedSQL(table)+") WHERE id = ?)", workspaceID, id).Scan(&exists)
	return exists, err
}

func resolveOfflineID(tx *sql.Tx, clientID, entity string, id int64) (int64, error) {
	if id > 0 {
		return id, nil
	}
	var serverID int64
	err := tx.QueryRow("SELECT server_id FROM offline_entity_ids WHERE client_id = ? AND entity = ? AND temporary_id = ?", clientID, entity, id).Scan(&serverID)
	if err == sql.ErrNoRows {
		return 0, offlineConflict("Unknown temporary " + entity + " ID; sync its create operation first")
	}
	return serverID, err
}

func offlineFields(values map[string]interface{}) ([]string, []interface{}) {
	fields := make([]string, 0, len(values))
	for field := range values {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	args := make([]interface{}, len(fields))
	for index, field := range fields {
		args[index] = values[field]
	}
	return fields, args
}

func validateOfflineValues(operation OfflineOperation) (map[string]interface{}, error) {
	allowed := map[string]map[string]bool{
		"list":    {"name": true, "icon": true, "show_completed": true, "sort_order": true, "is_active": true},
		"section": {"name": true, "list_id": true, "sort_mode": true, "sort_order": true},
		"item":    {"name": true, "description": true, "quantity": true, "completed": true, "uncertain": true, "section_id": true, "sort_order": true},
	}[operation.Entity]
	values := make(map[string]interface{}, len(operation.Values))
	if operation.Action == "delete" && len(operation.Values) > 0 {
		return nil, offlineInvalid("Delete operations cannot contain values")
	}
	for field, raw := range operation.Values {
		if !allowed[field] {
			return nil, offlineInvalid("Unknown " + operation.Entity + " field: " + field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, offlineInvalid(field + " cannot be null")
		}
		switch field {
		case "name", "icon", "description", "sort_mode":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, offlineInvalid(field + " must be a string")
			}
			limit := 100
			if field == "name" && operation.Entity == "item" {
				limit = 200
			}
			if field == "description" {
				limit = 500
			}
			if field == "icon" {
				limit = 20
			}
			if len(value) > limit {
				return nil, offlineInvalid(fmt.Sprintf("%s exceeds maximum length %d", field, limit))
			}
			if field == "name" && strings.TrimSpace(value) == "" {
				return nil, offlineInvalid("Name cannot be empty")
			}
			if field == "name" && operation.Entity != "item" && value == "[HISTORY]" {
				return nil, offlineInvalid("This name is reserved")
			}
			if field == "sort_mode" && value != "manual" && value != "alphabetical" && value != "alphabetical_desc" {
				return nil, offlineInvalid("Invalid section sort mode")
			}
			values[field] = value
		case "completed", "uncertain", "show_completed", "is_active":
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, offlineInvalid(field + " must be a boolean")
			}
			values[field] = value
		default:
			var value int64
			if err := json.Unmarshal(raw, &value); err != nil || value > maxOfflineInteger || value < -maxOfflineInteger {
				return nil, offlineInvalid(field + " must be a safe integer")
			}
			if (field == "quantity" || field == "sort_order") && value < 0 {
				return nil, offlineInvalid(field + " cannot be negative")
			}
			if (field == "section_id" || field == "list_id") && value == 0 {
				return nil, offlineInvalid(field + " cannot be zero")
			}
			values[field] = value
		}
	}
	if operation.Action == "create" {
		required := []string{"name"}
		if operation.Entity == "section" {
			required = append(required, "list_id")
		}
		if operation.Entity == "item" {
			required = append(required, "section_id")
		}
		for _, field := range required {
			if _, exists := values[field]; !exists {
				return nil, offlineInvalid(field + " is required")
			}
		}
	}
	return values, nil
}

func GetOfflineSnapshot(workspaceID int64) (OfflineSnapshot, error) {
	tx, err := DB.Begin()
	if err != nil {
		return OfflineSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := offlineSnapshotTx(tx, workspaceID)
	if err != nil {
		return snapshot, err
	}
	return snapshot, tx.Commit()
}

func offlineSnapshotTx(tx *sql.Tx, workspaceID int64) (OfflineSnapshot, error) {
	snapshot := OfflineSnapshot{Lists: []List{}, Sections: []Section{}, Items: []Item{}}
	rows, err := tx.Query(listSelectWithStats+" WHERE l.workspace_id = ? AND l.name != '[HISTORY]' GROUP BY l.id ORDER BY l.sort_order, l.id", workspaceID)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		list, err := scanListWithStats(rows)
		if err != nil {
			rows.Close()
			return snapshot, err
		}
		snapshot.Lists = append(snapshot.Lists, list)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return snapshot, err
	}
	rows, err = tx.Query(`SELECT s.id, s.list_id, s.name, s.sort_order, COALESCE(s.sort_mode, 'manual'), s.created_at, COALESCE(s.updated_at, 0)
		FROM sections s JOIN lists l ON l.id = s.list_id
		WHERE l.workspace_id = ? AND l.name != '[HISTORY]' AND s.name != '[HISTORY]' ORDER BY s.list_id, s.sort_order, s.id`, workspaceID)
	if err != nil {
		return snapshot, err
	}
	sections, err := scanSectionRows(rows)
	if err != nil {
		return snapshot, err
	}
	if sections != nil {
		snapshot.Sections = sections
	}
	rows, err = tx.Query(`SELECT i.id, i.section_id, i.name, i.description, i.completed, i.uncertain, COALESCE(i.quantity, 0), i.sort_order, i.created_at, COALESCE(i.updated_at, 0)
		FROM items i JOIN sections s ON s.id = i.section_id JOIN lists l ON l.id = s.list_id
		WHERE l.workspace_id = ? AND l.name != '[HISTORY]' AND s.name != '[HISTORY]' ORDER BY i.section_id, i.sort_order, i.id`, workspaceID)
	if err != nil {
		return snapshot, err
	}
	items, err := scanItemRows(rows)
	if err != nil {
		return snapshot, err
	}
	if items != nil {
		snapshot.Items = items
	}
	// Reserved history containers can exist in older imported databases. Keep
	// their descendants out of shopping data and exclude them from counters.
	listIndexes := make(map[int64]int, len(snapshot.Lists))
	for index := range snapshot.Lists {
		listIndexes[snapshot.Lists[index].ID] = index
		snapshot.Lists[index].Stats = Stats{}
	}
	sectionLists := make(map[int64]int64, len(snapshot.Sections))
	for _, section := range snapshot.Sections {
		sectionLists[section.ID] = section.ListID
	}
	for _, item := range snapshot.Items {
		stats := &snapshot.Lists[listIndexes[sectionLists[item.SectionID]]].Stats
		stats.TotalItems++
		if item.Completed {
			stats.CompletedItems++
		}
	}
	for index := range snapshot.Lists {
		stats := &snapshot.Lists[index].Stats
		if stats.TotalItems > 0 {
			stats.Percentage = stats.CompletedItems * 100 / stats.TotalItems
		}
	}
	return snapshot, nil
}

func offlineItemEvents(tx *sql.Tx, entity string, id int64, eventName string) ([]OfflineItemEvent, error) {
	column := map[string]string{"list": "l.id", "section": "s.id", "item": "i.id"}[entity]
	rows, err := tx.Query(`SELECT i.id, i.section_id, i.name, i.description, i.completed, i.uncertain,
		COALESCE(i.quantity, 0), i.sort_order, i.created_at, COALESCE(i.updated_at, 0),
		s.id, s.name, l.id, l.name FROM items i JOIN sections s ON s.id = i.section_id
		JOIN lists l ON l.id = s.list_id WHERE `+column+` = ? ORDER BY i.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []OfflineItemEvent
	for rows.Next() {
		event := OfflineItemEvent{Event: eventName}
		i := &event.Item
		if err := rows.Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.Quantity, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt,
			&event.Section.ID, &event.Section.Name, &event.List.ID, &event.List.Name); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
