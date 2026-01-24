package db

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Section represents a shopping list section
type Section struct {
	ID        int64     `json:"id"`
	ListID    int64     `json:"list_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt int64     `json:"updated_at"`
	Items     []Item    `json:"items"`
}

// Item represents a shopping list item
type Item struct {
	ID          int64     `json:"id"`
	SectionID   int64     `json:"section_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Completed   bool      `json:"completed"`
	Uncertain   bool      `json:"uncertain"`
	Quantity    int       `json:"quantity"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   int64     `json:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID        string
	ExpiresAt int64
}

// List represents a shopping list
type List struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt int64     `json:"updated_at"`
	Stats     Stats     `json:"stats,omitempty"`
}

// Template represents a reusable template
type Template struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	SortOrder   int            `json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
	Items       []TemplateItem `json:"items,omitempty"`
}

// TemplateItem represents an item in a template
type TemplateItem struct {
	ID          int64     `json:"id"`
	TemplateID  int64     `json:"template_id"`
	SectionName string    `json:"section_name"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

// ==================== LISTS ====================

// GetAllLists returns all shopping lists with their stats
func GetAllLists() ([]List, error) {
	rows, err := DB.Query(`
		SELECT id, name, COALESCE(icon, '🛒'), sort_order, is_active, created_at, COALESCE(updated_at, 0)
		FROM lists
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []List
	for rows.Next() {
		var l List
		err := rows.Scan(&l.ID, &l.Name, &l.Icon, &l.SortOrder, &l.IsActive, &l.CreatedAt, &l.UpdatedAt)
		if err != nil {
			return nil, err
		}
		l.Stats = GetListStats(l.ID)
		lists = append(lists, l)
	}
	return lists, nil
}

// GetListByID returns a single list by ID
func GetListByID(id int64) (*List, error) {
	var l List
	err := DB.QueryRow(`
		SELECT id, name, COALESCE(icon, '🛒'), sort_order, is_active, created_at, COALESCE(updated_at, 0)
		FROM lists WHERE id = ?
	`, id).Scan(&l.ID, &l.Name, &l.Icon, &l.SortOrder, &l.IsActive, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	l.Stats = GetListStats(l.ID)
	return &l, nil
}

// GetActiveList returns the currently active list
func GetActiveList() (*List, error) {
	var l List
	err := DB.QueryRow(`
		SELECT id, name, COALESCE(icon, '🛒'), sort_order, is_active, created_at, COALESCE(updated_at, 0)
		FROM lists WHERE is_active = TRUE
		LIMIT 1
	`).Scan(&l.ID, &l.Name, &l.Icon, &l.SortOrder, &l.IsActive, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	l.Stats = GetListStats(l.ID)
	return &l, nil
}

// CreateList creates a new shopping list
func CreateList(name, icon string) (*List, error) {
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM lists").Scan(&maxOrder)

	if icon == "" {
		icon = "🛒"
	}

	result, err := DB.Exec(`
		INSERT INTO lists (name, icon, sort_order, is_active) VALUES (?, ?, ?, FALSE)
	`, name, icon, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetListByID(id)
}

// ListNameExists checks if a list with the given name already exists (case-insensitive)
// excludeID allows excluding a specific list (useful when updating)
func ListNameExists(name string, excludeID int64) (bool, error) {
	var count int
	var err error
	if excludeID > 0 {
		err = DB.QueryRow(`
			SELECT COUNT(*) FROM lists
			WHERE name = ? COLLATE NOCASE AND id != ?
		`, name, excludeID).Scan(&count)
	} else {
		err = DB.QueryRow(`
			SELECT COUNT(*) FROM lists
			WHERE name = ? COLLATE NOCASE
		`, name).Scan(&count)
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateList updates a list's name and icon
func UpdateList(id int64, name, icon string) (*List, error) {
	if icon == "" {
		_, err := DB.Exec(`UPDATE lists SET name = ?, updated_at = strftime('%s', 'now') WHERE id = ?`, name, id)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := DB.Exec(`UPDATE lists SET name = ?, icon = ?, updated_at = strftime('%s', 'now') WHERE id = ?`, name, icon, id)
		if err != nil {
			return nil, err
		}
	}
	return GetListByID(id)
}

// DeleteList deletes a list and all its sections/items
func DeleteList(id int64) error {
	_, err := DB.Exec(`DELETE FROM lists WHERE id = ?`, id)
	return err
}

// SetActiveList sets a list as the active one
func SetActiveList(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Deactivate all lists
	_, err = tx.Exec("UPDATE lists SET is_active = FALSE")
	if err != nil {
		return err
	}

	// Activate the selected list
	_, err = tx.Exec("UPDATE lists SET is_active = TRUE, updated_at = strftime('%s', 'now') WHERE id = ?", id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// MoveListUp moves a list up in sort order
func MoveListUp(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentOrder int
	err = tx.QueryRow("SELECT sort_order FROM lists WHERE id = ?", id).Scan(&currentOrder)
	if err != nil {
		return err
	}

	if currentOrder == 0 {
		return nil
	}

	_, err = tx.Exec(`UPDATE lists SET sort_order = sort_order + 1 WHERE sort_order = ?`, currentOrder-1)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE lists SET sort_order = ? WHERE id = ?`, currentOrder-1, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// MoveListDown moves a list down in sort order
func MoveListDown(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentOrder, maxOrder int
	err = tx.QueryRow("SELECT sort_order FROM lists WHERE id = ?", id).Scan(&currentOrder)
	if err != nil {
		return err
	}
	err = tx.QueryRow("SELECT MAX(sort_order) FROM lists").Scan(&maxOrder)
	if err != nil {
		return err
	}

	if currentOrder >= maxOrder {
		return nil
	}

	_, err = tx.Exec(`UPDATE lists SET sort_order = sort_order - 1 WHERE sort_order = ?`, currentOrder+1)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE lists SET sort_order = ? WHERE id = ?`, currentOrder+1, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetListStats returns stats for a specific list
func GetListStats(listID int64) Stats {
	var stats Stats
	DB.QueryRow(`
		SELECT COUNT(*) FROM items i
		JOIN sections s ON i.section_id = s.id
		WHERE s.list_id = ?
	`, listID).Scan(&stats.TotalItems)
	DB.QueryRow(`
		SELECT COUNT(*) FROM items i
		JOIN sections s ON i.section_id = s.id
		WHERE s.list_id = ? AND i.completed = TRUE
	`, listID).Scan(&stats.CompletedItems)
	if stats.TotalItems > 0 {
		stats.Percentage = (stats.CompletedItems * 100) / stats.TotalItems
	}
	return stats
}

// ==================== SECTIONS ====================

func GetAllSections() ([]Section, error) {
	activeList, err := GetActiveList()
	if err != nil {
		// Fallback: return all sections if no active list (shouldn't happen)
		return getAllSectionsGlobal()
	}
	return GetSectionsByList(activeList.ID)
}

// GetSectionsByList returns all sections for a specific list
func GetSectionsByList(listID int64) ([]Section, error) {
	rows, err := DB.Query(`
		SELECT id, list_id, name, sort_order, created_at, COALESCE(updated_at, 0)
		FROM sections
		WHERE list_id = ?
		ORDER BY sort_order ASC
	`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sections []Section
	for rows.Next() {
		var s Section
		err := rows.Scan(&s.ID, &s.ListID, &s.Name, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Items, err = GetItemsBySection(s.ID)
		if err != nil {
			return nil, err
		}
		sections = append(sections, s)
	}
	return sections, nil
}

// getAllSectionsGlobal returns all sections (fallback, used during migration)
func getAllSectionsGlobal() ([]Section, error) {
	rows, err := DB.Query(`
		SELECT id, list_id, name, sort_order, created_at, COALESCE(updated_at, 0)
		FROM sections
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sections []Section
	for rows.Next() {
		var s Section
		err := rows.Scan(&s.ID, &s.ListID, &s.Name, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Items, err = GetItemsBySection(s.ID)
		if err != nil {
			return nil, err
		}
		sections = append(sections, s)
	}
	return sections, nil
}

func GetSectionByID(id int64) (*Section, error) {
	var s Section
	err := DB.QueryRow(`
		SELECT id, list_id, name, sort_order, created_at, COALESCE(updated_at, 0)
		FROM sections WHERE id = ?
	`, id).Scan(&s.ID, &s.ListID, &s.Name, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Items, err = GetItemsBySection(s.ID)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func CreateSection(name string) (*Section, error) {
	activeList, err := GetActiveList()
	if err != nil {
		return nil, fmt.Errorf("no active list found")
	}
	return CreateSectionForList(activeList.ID, name)
}

// CreateSectionForList creates a section for a specific list
func CreateSectionForList(listID int64, name string) (*Section, error) {
	// Get max sort_order for this list
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM sections WHERE list_id = ?", listID).Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO sections (name, sort_order, list_id) VALUES (?, ?, ?)
	`, name, maxOrder+1, listID)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetSectionByID(id)
}

func UpdateSection(id int64, name string) (*Section, error) {
	_, err := DB.Exec(`UPDATE sections SET name = ?, updated_at = strftime('%s', 'now') WHERE id = ?`, name, id)
	if err != nil {
		return nil, err
	}
	return GetSectionByID(id)
}

func DeleteSection(id int64) error {
	_, err := DB.Exec(`DELETE FROM sections WHERE id = ?`, id)
	return err
}

func MoveSectionUp(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentOrder int
	var listID int64
	err = tx.QueryRow("SELECT sort_order, list_id FROM sections WHERE id = ?", id).Scan(&currentOrder, &listID)
	if err != nil {
		return err
	}

	if currentOrder == 0 {
		return nil // Already at top
	}

	// Swap with previous section (within the same list)
	_, err = tx.Exec(`
		UPDATE sections SET sort_order = sort_order + 1
		WHERE sort_order = ? AND list_id = ?
	`, currentOrder-1, listID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE sections SET sort_order = ? WHERE id = ?
	`, currentOrder-1, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func MoveSectionDown(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentOrder int
	var listID int64
	err = tx.QueryRow("SELECT sort_order, list_id FROM sections WHERE id = ?", id).Scan(&currentOrder, &listID)
	if err != nil {
		return err
	}

	var maxOrder int
	err = tx.QueryRow("SELECT MAX(sort_order) FROM sections WHERE list_id = ?", listID).Scan(&maxOrder)
	if err != nil {
		return err
	}

	if currentOrder >= maxOrder {
		return nil // Already at bottom
	}

	// Swap with next section (within the same list)
	_, err = tx.Exec(`
		UPDATE sections SET sort_order = sort_order - 1
		WHERE sort_order = ? AND list_id = ?
	`, currentOrder+1, listID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE sections SET sort_order = ? WHERE id = ?
	`, currentOrder+1, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ==================== ITEMS ====================

func GetItemsBySection(sectionID int64) ([]Item, error) {
	rows, err := DB.Query(`
		SELECT id, section_id, name, description, completed, uncertain, COALESCE(quantity, 0), sort_order, created_at, COALESCE(updated_at, 0)
		FROM items
		WHERE section_id = ?
		ORDER BY completed ASC, sort_order ASC
	`, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var i Item
		err := rows.Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.Quantity, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

func GetItemByID(id int64) (*Item, error) {
	var i Item
	err := DB.QueryRow(`
		SELECT id, section_id, name, description, completed, uncertain, COALESCE(quantity, 0), sort_order, created_at, COALESCE(updated_at, 0)
		FROM items WHERE id = ?
	`, id).Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.Quantity, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func CreateItem(sectionID int64, name, description string, quantity int) (*Item, error) {
	// Get max sort_order for this section
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", sectionID).Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO items (section_id, name, description, quantity, sort_order) VALUES (?, ?, ?, ?, ?)
	`, sectionID, name, description, quantity, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetItemByID(id)
}

func UpdateItem(id int64, name, description string, quantity int) (*Item, error) {
	_, err := DB.Exec(`
		UPDATE items SET name = ?, description = ?, quantity = ?, updated_at = strftime('%s', 'now') WHERE id = ?
	`, name, description, quantity, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func DeleteItem(id int64) error {
	_, err := DB.Exec(`DELETE FROM items WHERE id = ?`, id)
	return err
}

// DeleteCompletedItems deletes all completed items from the active list
func DeleteCompletedItems() (int64, error) {
	activeList, err := GetActiveList()
	if err != nil {
		return 0, err
	}

	result, err := DB.Exec(`
		DELETE FROM items WHERE completed = TRUE AND section_id IN (
			SELECT id FROM sections WHERE list_id = ?
		)
	`, activeList.ID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func ToggleItemCompleted(id int64) (*Item, error) {
	_, err := DB.Exec(`UPDATE items SET completed = NOT completed, updated_at = strftime('%s', 'now') WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func ToggleItemUncertain(id int64) (*Item, error) {
	_, err := DB.Exec(`UPDATE items SET uncertain = NOT uncertain, updated_at = strftime('%s', 'now') WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func MoveItemToSection(id, newSectionID int64) (*Item, error) {
	// Get max sort_order in new section
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", newSectionID).Scan(&maxOrder)

	_, err := DB.Exec(`
		UPDATE items SET section_id = ?, sort_order = ?, updated_at = strftime('%s', 'now') WHERE id = ?
	`, newSectionID, maxOrder+1, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

// MoveItemToSectionAtPosition moves an item to a new section at a specific position among ACTIVE items
func MoveItemToSectionAtPosition(id, newSectionID int64, targetPosition int) (*Item, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Verify item exists and get current section
	var currentSectionID int64
	err = tx.QueryRow("SELECT section_id FROM items WHERE id = ?", id).Scan(&currentSectionID)
	if err != nil {
		return nil, err // Item not found
	}

	// If same section, use regular reorder logic instead
	if currentSectionID == newSectionID {
		tx.Rollback()
		return reorderItemInSection(id, targetPosition)
	}

	// Get all ACTIVE items in target section, ordered by sort_order
	rows, err := tx.Query(`
		SELECT id, sort_order FROM items
		WHERE section_id = ? AND completed = FALSE
		ORDER BY sort_order ASC
	`, newSectionID)
	if err != nil {
		return nil, err
	}

	type itemOrder struct {
		id        int64
		sortOrder int
	}
	var activeItems []itemOrder
	for rows.Next() {
		var item itemOrder
		if err := rows.Scan(&item.id, &item.sortOrder); err != nil {
			rows.Close()
			return nil, err
		}
		activeItems = append(activeItems, item)
	}
	rows.Close()

	// Determine the target sort_order
	var targetSortOrder int
	if len(activeItems) == 0 {
		// No active items - check if there are ANY items (completed) and use max+1
		var maxOrder int
		err = tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", newSectionID).Scan(&maxOrder)
		if err != nil {
			return nil, err
		}
		targetSortOrder = maxOrder + 1
	} else if targetPosition <= 0 {
		// Insert at beginning - use sort_order less than first
		targetSortOrder = activeItems[0].sortOrder
	} else if targetPosition >= len(activeItems) {
		// Insert at end - use sort_order greater than last active
		targetSortOrder = activeItems[len(activeItems)-1].sortOrder + 1
	} else {
		// Insert in middle - use sort_order of item at target position
		targetSortOrder = activeItems[targetPosition].sortOrder
	}

	// Shift all items with sort_order >= targetSortOrder up by 1
	_, err = tx.Exec(`
		UPDATE items SET sort_order = sort_order + 1
		WHERE section_id = ? AND sort_order >= ?
	`, newSectionID, targetSortOrder)
	if err != nil {
		return nil, err
	}

	// Move the item to new section with target sort_order
	_, err = tx.Exec(`
		UPDATE items SET section_id = ?, sort_order = ?, updated_at = strftime('%s', 'now')
		WHERE id = ?
	`, newSectionID, targetSortOrder, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return GetItemByID(id)
}

// reorderItemInSection moves an item to a specific position within its current section
func reorderItemInSection(id int64, targetPosition int) (*Item, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get item's current section and sort_order
	var sectionID int64
	var currentSortOrder int
	err = tx.QueryRow("SELECT section_id, sort_order FROM items WHERE id = ?", id).Scan(&sectionID, &currentSortOrder)
	if err != nil {
		return nil, err
	}

	// Get all ACTIVE items in section (excluding the moved item), ordered by sort_order
	rows, err := tx.Query(`
		SELECT id, sort_order FROM items
		WHERE section_id = ? AND completed = FALSE AND id != ?
		ORDER BY sort_order ASC
	`, sectionID, id)
	if err != nil {
		return nil, err
	}

	type itemOrder struct {
		id        int64
		sortOrder int
	}
	var otherItems []itemOrder
	for rows.Next() {
		var item itemOrder
		if err := rows.Scan(&item.id, &item.sortOrder); err != nil {
			rows.Close()
			return nil, err
		}
		otherItems = append(otherItems, item)
	}
	rows.Close()

	// Clamp target position
	if targetPosition < 0 {
		targetPosition = 0
	}
	if targetPosition > len(otherItems) {
		targetPosition = len(otherItems)
	}

	// Renumber all active items with moved item at target position
	newOrder := 0
	for i, item := range otherItems {
		if i == targetPosition {
			// Insert moved item here
			_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", newOrder, id)
			if err != nil {
				return nil, err
			}
			newOrder++
		}
		_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", newOrder, item.id)
		if err != nil {
			return nil, err
		}
		newOrder++
	}

	// If target is at end
	if targetPosition >= len(otherItems) {
		_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", newOrder, id)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return GetItemByID(id)
}

func MoveItemUp(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sectionID int64
	var sortOrder int
	err = tx.QueryRow("SELECT section_id, sort_order FROM items WHERE id = ?", id).Scan(&sectionID, &sortOrder)
	if err != nil {
		return err
	}

	// Find previous item (closest smaller sort_order) - handles non-contiguous sort_order
	var prevID int64
	var prevSortOrder int
	err = tx.QueryRow(`
		SELECT id, sort_order FROM items
		WHERE section_id = ? AND sort_order < ?
		ORDER BY sort_order DESC
		LIMIT 1
	`, sectionID, sortOrder).Scan(&prevID, &prevSortOrder)

	if err == sql.ErrNoRows {
		return nil // Already at top
	}
	if err != nil {
		return err
	}

	// Swap sort_order values
	_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", sortOrder, prevID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", prevSortOrder, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func MoveItemDown(id int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sectionID int64
	var sortOrder int
	err = tx.QueryRow("SELECT section_id, sort_order FROM items WHERE id = ?", id).Scan(&sectionID, &sortOrder)
	if err != nil {
		return err
	}

	// Find next item (closest larger sort_order) - handles non-contiguous sort_order
	var nextID int64
	var nextSortOrder int
	err = tx.QueryRow(`
		SELECT id, sort_order FROM items
		WHERE section_id = ? AND sort_order > ?
		ORDER BY sort_order ASC
		LIMIT 1
	`, sectionID, sortOrder).Scan(&nextID, &nextSortOrder)

	if err == sql.ErrNoRows {
		return nil // Already at bottom
	}
	if err != nil {
		return err
	}

	// Swap sort_order values
	_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", sortOrder, nextID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE items SET sort_order = ? WHERE id = ?", nextSortOrder, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ==================== SESSIONS ====================

func CreateSession(id string, expiresAt int64) error {
	_, err := DB.Exec(`INSERT INTO sessions (id, expires_at) VALUES (?, ?)`, id, expiresAt)
	return err
}

func GetSession(id string) (*Session, error) {
	var s Session
	err := DB.QueryRow(`SELECT id, expires_at FROM sessions WHERE id = ?`, id).Scan(&s.ID, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSession(id string) error {
	_, err := DB.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func CleanExpiredSessions() error {
	_, err := DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
	return err
}

// ==================== STATS ====================

type Stats struct {
	TotalItems     int `json:"total_items"`
	CompletedItems int `json:"completed_items"`
	Percentage     int `json:"percentage"`
}

func GetStats() Stats {
	activeList, err := GetActiveList()
	if err != nil {
		// Fallback to global stats
		return getGlobalStats()
	}
	return GetListStats(activeList.ID)
}

// getGlobalStats returns stats for all items (fallback)
func getGlobalStats() Stats {
	var stats Stats
	DB.QueryRow("SELECT COUNT(*) FROM items").Scan(&stats.TotalItems)
	DB.QueryRow("SELECT COUNT(*) FROM items WHERE completed = TRUE").Scan(&stats.CompletedItems)
	if stats.TotalItems > 0 {
		stats.Percentage = (stats.CompletedItems * 100) / stats.TotalItems
	}
	return stats
}

// ==================== SECTION STATS ====================

type SectionStats struct {
	TotalItems     int `json:"total_items"`
	CompletedItems int `json:"completed_items"`
	Percentage     int `json:"percentage"`
}

func GetSectionStats(sectionID int64) SectionStats {
	var stats SectionStats
	DB.QueryRow("SELECT COUNT(*) FROM items WHERE section_id = ?", sectionID).Scan(&stats.TotalItems)
	DB.QueryRow("SELECT COUNT(*) FROM items WHERE section_id = ? AND completed = TRUE", sectionID).Scan(&stats.CompletedItems)
	if stats.TotalItems > 0 {
		stats.Percentage = (stats.CompletedItems * 100) / stats.TotalItems
	}
	return stats
}

// ==================== BATCH DELETE SECTIONS ====================

func DeleteSections(ids []int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		_, err := tx.Exec("DELETE FROM sections WHERE id = ?", id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ==================== ITEM HISTORY (Auto-completion) ====================

type ItemSuggestion struct {
	Name            string `json:"name"`
	LastSectionID   int64  `json:"last_section_id"`
	LastSectionName string `json:"last_section_name"`
	UsageCount      int    `json:"usage_count"`
}

// SaveItemHistory saves or updates item name in history for auto-completion
func SaveItemHistory(name string, sectionID int64) error {
	_, err := DB.Exec(`
		INSERT INTO item_history (name, last_section_id, usage_count, last_used_at)
		VALUES (?, ?, 1, strftime('%s', 'now'))
		ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET
			last_section_id = excluded.last_section_id,
			usage_count = usage_count + 1,
			last_used_at = strftime('%s', 'now')
	`, name, sectionID)
	return err
}

// SaveItemHistoryWithCount saves item history with a specific usage count (used for import)
func SaveItemHistoryWithCount(name string, sectionID int64, usageCount int) error {
	_, err := DB.Exec(`
		INSERT INTO item_history (name, last_section_id, usage_count, last_used_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
		ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET
			last_section_id = CASE WHEN excluded.last_section_id > 0 THEN excluded.last_section_id ELSE last_section_id END,
			usage_count = CASE WHEN excluded.usage_count > usage_count THEN excluded.usage_count ELSE usage_count END,
			last_used_at = strftime('%s', 'now')
	`, name, sectionID, usageCount)
	return err
}

// SaveItemHistoryWithCountTx saves item history with a specific usage count within a transaction
func SaveItemHistoryWithCountTx(tx *sql.Tx, name string, sectionID int64, usageCount int) error {
	_, err := tx.Exec(`
		INSERT INTO item_history (name, last_section_id, usage_count, last_used_at)
		VALUES (?, ?, ?, strftime('%s', 'now'))
		ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET
			last_section_id = CASE WHEN excluded.last_section_id > 0 THEN excluded.last_section_id ELSE last_section_id END,
			usage_count = CASE WHEN excluded.usage_count > usage_count THEN excluded.usage_count ELSE usage_count END,
			last_used_at = strftime('%s', 'now')
	`, name, sectionID, usageCount)
	return err
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// scoreSuggestion calculates a match score (higher is better)
func scoreSuggestion(name, query string) int {
	nameLower := strings.ToLower(name)
	queryLower := strings.ToLower(query)

	// Exact match: highest score
	if nameLower == queryLower {
		return 1000
	}

	// Prefix match: high score
	if strings.HasPrefix(nameLower, queryLower) {
		return 500
	}

	// Contains match: medium score
	if strings.Contains(nameLower, queryLower) {
		return 200
	}

	// Fuzzy match: score based on Levenshtein distance
	// Only consider if query is at least 3 chars and distance is reasonable
	if len(query) >= 3 {
		distance := levenshteinDistance(nameLower, queryLower)
		maxDistance := len(query) / 2 // Allow ~50% typos

		if distance <= maxDistance {
			return 100 - distance*20 // Lower score for more typos
		}

		// Also check if any word in the name fuzzy matches
		words := strings.Fields(nameLower)
		for _, word := range words {
			wordDist := levenshteinDistance(word, queryLower)
			if wordDist <= maxDistance {
				return 80 - wordDist*15
			}
		}
	}

	return 0 // No match
}

// GetItemSuggestions returns item name suggestions matching the query with fuzzy matching
func GetItemSuggestions(query string, limit int) ([]ItemSuggestion, error) {
	if limit <= 0 {
		limit = 10
	}

	// Fetch more items to allow for fuzzy matching and scoring
	rows, err := DB.Query(`
		SELECT h.name, COALESCE(h.last_section_id, 0), COALESCE(s.name, ''), h.usage_count
		FROM item_history h
		LEFT JOIN sections s ON h.last_section_id = s.id
		ORDER BY h.usage_count DESC, h.last_used_at DESC
		LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scoredSuggestion struct {
		suggestion ItemSuggestion
		score      int
	}

	var scored []scoredSuggestion
	for rows.Next() {
		var s ItemSuggestion
		if err := rows.Scan(&s.Name, &s.LastSectionID, &s.LastSectionName, &s.UsageCount); err != nil {
			return nil, err
		}

		score := scoreSuggestion(s.Name, query)
		if score > 0 {
			// Boost score slightly by usage count
			score += s.UsageCount / 10
			scored = append(scored, scoredSuggestion{s, score})
		}
	}

	// Sort by score (descending), then by usage_count (descending)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].suggestion.UsageCount > scored[j].suggestion.UsageCount
	})

	// Return top results
	var suggestions []ItemSuggestion
	for i := 0; i < len(scored) && i < limit; i++ {
		suggestions = append(suggestions, scored[i].suggestion)
	}

	return suggestions, nil
}

// GetAllItemSuggestions returns all item suggestions for offline cache
func GetAllItemSuggestions(limit int) ([]ItemSuggestion, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := DB.Query(`
		SELECT h.name, COALESCE(h.last_section_id, 0), COALESCE(s.name, ''), h.usage_count
		FROM item_history h
		LEFT JOIN sections s ON h.last_section_id = s.id
		ORDER BY h.usage_count DESC, h.last_used_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []ItemSuggestion
	for rows.Next() {
		var s ItemSuggestion
		if err := rows.Scan(&s.Name, &s.LastSectionID, &s.LastSectionName, &s.UsageCount); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, nil
}

// HistoryItem represents an item from history with ID for management
type HistoryItem struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	LastSectionID   int64  `json:"last_section_id"`
	LastSectionName string `json:"last_section_name"`
	UsageCount      int    `json:"usage_count"`
}

// GetItemHistoryList returns all history items for management UI
func GetItemHistoryList() ([]HistoryItem, error) {
	rows, err := DB.Query(`
		SELECT h.id, h.name, COALESCE(h.last_section_id, 0), COALESCE(s.name, ''), h.usage_count
		FROM item_history h
		LEFT JOIN sections s ON h.last_section_id = s.id
		ORDER BY h.usage_count DESC, h.last_used_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []HistoryItem
	for rows.Next() {
		var h HistoryItem
		if err := rows.Scan(&h.ID, &h.Name, &h.LastSectionID, &h.LastSectionName, &h.UsageCount); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, nil
}

// DeleteItemHistory deletes a single item from history
func DeleteItemHistory(id int64) error {
	result, err := DB.Exec("DELETE FROM item_history WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("history item not found")
	}
	return nil
}

// DeleteItemHistoryBatch deletes multiple items from history
func DeleteItemHistoryBatch(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	// Build placeholders
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM item_history WHERE id IN (%s)", strings.Join(placeholders, ","))
	result, err := DB.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ==================== TEMPLATES ====================

// GetAllTemplates returns all templates with their items
func GetAllTemplates() ([]Template, error) {
	rows, err := DB.Query(`
		SELECT id, name, description, sort_order, created_at, COALESCE(updated_at, 0)
		FROM templates
		ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, err
		}
		t.Items, err = GetTemplateItems(t.ID)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, nil
}

// GetTemplateByID returns a single template by ID with items
func GetTemplateByID(id int64) (*Template, error) {
	var t Template
	err := DB.QueryRow(`
		SELECT id, name, description, sort_order, created_at, COALESCE(updated_at, 0)
		FROM templates WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &t.Description, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.Items, err = GetTemplateItems(t.ID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTemplateItems returns all items for a template
func GetTemplateItems(templateID int64) ([]TemplateItem, error) {
	rows, err := DB.Query(`
		SELECT id, template_id, section_name, name, description, sort_order, created_at
		FROM template_items
		WHERE template_id = ?
		ORDER BY section_name ASC, sort_order ASC
	`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TemplateItem
	for rows.Next() {
		var ti TemplateItem
		err := rows.Scan(&ti.ID, &ti.TemplateID, &ti.SectionName, &ti.Name, &ti.Description, &ti.SortOrder, &ti.CreatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, ti)
	}
	return items, nil
}

// CreateTemplate creates a new template
func CreateTemplate(name, description string) (*Template, error) {
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM templates").Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO templates (name, description, sort_order) VALUES (?, ?, ?)
	`, name, description, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetTemplateByID(id)
}

// UpdateTemplate updates a template's name and description
func UpdateTemplate(id int64, name, description string) (*Template, error) {
	_, err := DB.Exec(`
		UPDATE templates SET name = ?, description = ?, updated_at = strftime('%s', 'now') WHERE id = ?
	`, name, description, id)
	if err != nil {
		return nil, err
	}
	return GetTemplateByID(id)
}

// DeleteTemplate deletes a template and all its items
func DeleteTemplate(id int64) error {
	_, err := DB.Exec(`DELETE FROM templates WHERE id = ?`, id)
	return err
}

// AddTemplateItem adds an item to a template
func AddTemplateItem(templateID int64, sectionName, name, description string) (*TemplateItem, error) {
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM template_items WHERE template_id = ?", templateID).Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO template_items (template_id, section_name, name, description, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, templateID, sectionName, name, description, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetTemplateItemByID(id)
}

// GetTemplateItemByID returns a single template item by ID
func GetTemplateItemByID(id int64) (*TemplateItem, error) {
	var ti TemplateItem
	err := DB.QueryRow(`
		SELECT id, template_id, section_name, name, description, sort_order, created_at
		FROM template_items WHERE id = ?
	`, id).Scan(&ti.ID, &ti.TemplateID, &ti.SectionName, &ti.Name, &ti.Description, &ti.SortOrder, &ti.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ti, nil
}

// UpdateTemplateItem updates a template item
func UpdateTemplateItem(id int64, sectionName, name, description string) (*TemplateItem, error) {
	_, err := DB.Exec(`
		UPDATE template_items SET section_name = ?, name = ?, description = ? WHERE id = ?
	`, sectionName, name, description, id)
	if err != nil {
		return nil, err
	}
	return GetTemplateItemByID(id)
}

// DeleteTemplateItem deletes a template item
func DeleteTemplateItem(id int64) error {
	_, err := DB.Exec(`DELETE FROM template_items WHERE id = ?`, id)
	return err
}

// ApplyTemplateToList applies a template to a list (adds items from template)
func ApplyTemplateToList(templateID, listID int64) error {
	template, err := GetTemplateByID(templateID)
	if err != nil {
		return err
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Group items by section name
	sectionItems := make(map[string][]TemplateItem)
	for _, item := range template.Items {
		sectionItems[item.SectionName] = append(sectionItems[item.SectionName], item)
	}

	// For each section in template
	for sectionName, items := range sectionItems {
		// Find or create section in target list
		var sectionID int64
		err := tx.QueryRow(`
			SELECT id FROM sections WHERE list_id = ? AND name = ? COLLATE NOCASE
		`, listID, sectionName).Scan(&sectionID)

		if err != nil {
			// Section doesn't exist, create it
			var maxOrder int
			tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM sections WHERE list_id = ?", listID).Scan(&maxOrder)

			result, err := tx.Exec(`
				INSERT INTO sections (name, sort_order, list_id) VALUES (?, ?, ?)
			`, sectionName, maxOrder+1, listID)
			if err != nil {
				return err
			}
			sectionID, _ = result.LastInsertId()
		}

		// Add items to section
		for _, item := range items {
			var maxItemOrder int
			tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", sectionID).Scan(&maxItemOrder)

			_, err := tx.Exec(`
				INSERT INTO items (section_id, name, description, sort_order)
				VALUES (?, ?, ?, ?)
			`, sectionID, item.Name, item.Description, maxItemOrder+1)
			if err != nil {
				return err
			}

			// Save to item history
			tx.Exec(`
				INSERT INTO item_history (name, last_section_id, usage_count, last_used_at)
				VALUES (?, ?, 1, strftime('%s', 'now'))
				ON CONFLICT(name COLLATE NOCASE) DO UPDATE SET
					last_section_id = excluded.last_section_id,
					usage_count = usage_count + 1,
					last_used_at = strftime('%s', 'now')
			`, item.Name, sectionID)
		}
	}

	return tx.Commit()
}

// CreateTemplateFromList creates a template from an existing list
func CreateTemplateFromList(listID int64, templateName, templateDescription string) (*Template, error) {
	sections, err := GetSectionsByList(listID)
	if err != nil {
		return nil, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Create template
	var maxOrder int
	tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM templates").Scan(&maxOrder)

	result, err := tx.Exec(`
		INSERT INTO templates (name, description, sort_order) VALUES (?, ?, ?)
	`, templateName, templateDescription, maxOrder+1)
	if err != nil {
		return nil, err
	}
	templateID, _ := result.LastInsertId()

	// Add items from list sections
	itemOrder := 0
	for _, section := range sections {
		for _, item := range section.Items {
			if !item.Completed { // Only add non-completed items
				_, err := tx.Exec(`
					INSERT INTO template_items (template_id, section_name, name, description, sort_order)
					VALUES (?, ?, ?, ?, ?)
				`, templateID, section.Name, item.Name, item.Description, itemOrder)
				if err != nil {
					return nil, err
				}
				itemOrder++
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return GetTemplateByID(templateID)
}

// ==================== TRANSACTION HELPERS (for batch API) ====================

// CreateListTx creates a list within a transaction
func CreateListTx(tx *sql.Tx, name, icon string) (*List, error) {
	var maxOrder int
	tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM lists").Scan(&maxOrder)

	if icon == "" {
		icon = "🛒"
	}

	result, err := tx.Exec(`
		INSERT INTO lists (name, icon, sort_order, is_active) VALUES (?, ?, ?, FALSE)
	`, name, icon, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	var l List
	err = tx.QueryRow(`
		SELECT id, name, COALESCE(icon, '🛒'), sort_order, is_active, created_at, COALESCE(updated_at, 0)
		FROM lists WHERE id = ?
	`, id).Scan(&l.ID, &l.Name, &l.Icon, &l.SortOrder, &l.IsActive, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// CreateSectionForListTx creates a section within a transaction
func CreateSectionForListTx(tx *sql.Tx, listID int64, name string, sortOrder int) (*Section, error) {
	result, err := tx.Exec(`
		INSERT INTO sections (name, sort_order, list_id) VALUES (?, ?, ?)
	`, name, sortOrder, listID)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	var s Section
	err = tx.QueryRow(`
		SELECT id, list_id, name, sort_order, created_at, COALESCE(updated_at, 0)
		FROM sections WHERE id = ?
	`, id).Scan(&s.ID, &s.ListID, &s.Name, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Items = []Item{}
	return &s, nil
}

// CreateItemTx creates an item within a transaction
func CreateItemTx(tx *sql.Tx, sectionID int64, name, description string, quantity, sortOrder int) (*Item, error) {
	result, err := tx.Exec(`
		INSERT INTO items (section_id, name, description, quantity, sort_order) VALUES (?, ?, ?, ?, ?)
	`, sectionID, name, description, quantity, sortOrder)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	var i Item
	err = tx.QueryRow(`
		SELECT id, section_id, name, description, completed, uncertain, COALESCE(quantity, 0), sort_order, created_at, COALESCE(updated_at, 0)
		FROM items WHERE id = ?
	`, id).Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.Quantity, &i.SortOrder, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// SaveItemHistoryTx saves item name to history within a transaction
func SaveItemHistoryTx(tx *sql.Tx, name string, sectionID int64) {
	tx.Exec(`
		INSERT INTO item_history (name, last_section_id, usage_count)
		VALUES (?, ?, 1)
		ON CONFLICT(name) DO UPDATE SET
			usage_count = usage_count + 1,
			last_section_id = excluded.last_section_id
	`, name, sectionID)
}

// GetMaxSectionOrderTx gets max sort_order for sections in a list within a transaction
func GetMaxSectionOrderTx(tx *sql.Tx, listID int64) int {
	var maxOrder int
	tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM sections WHERE list_id = ?", listID).Scan(&maxOrder)
	return maxOrder
}

// GetMaxItemOrderTx gets max sort_order for items in a section within a transaction
func GetMaxItemOrderTx(tx *sql.Tx, sectionID int64) int {
	var maxOrder int
	tx.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", sectionID).Scan(&maxOrder)
	return maxOrder
}

// GetSectionIDByNameTx finds section ID by name (case-insensitive) within a transaction
// Returns 0 if section not found
func GetSectionIDByNameTx(tx *sql.Tx, sectionName string) int64 {
	if sectionName == "" {
		return 0
	}

	var sectionID int64
	err := tx.QueryRow(`
		SELECT id FROM sections
		WHERE name = ? COLLATE NOCASE
		LIMIT 1
	`, sectionName).Scan(&sectionID)

	if err != nil {
		return 0
	}
	return sectionID
}

// GetSectionNameForItem finds the section name where an item currently exists
// Used as fallback when last_section_id is not set in history
func GetSectionNameForItem(itemName string) string {
	var sectionName string
	err := DB.QueryRow(`
		SELECT s.name FROM items i
		JOIN sections s ON i.section_id = s.id
		WHERE i.name = ? COLLATE NOCASE
		LIMIT 1
	`, itemName).Scan(&sectionName)

	if err != nil {
		return ""
	}
	return sectionName
}

// ==================== DATABASE CLEAR ====================

// ClearAllData clears all user data from database (lists, sections, items, templates, history)
// Sessions are preserved so user remains logged in
func ClearAllData() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete in proper order due to foreign key constraints
	// 1. template_items (references templates)
	if _, err := tx.Exec("DELETE FROM template_items"); err != nil {
		return fmt.Errorf("failed to delete template_items: %w", err)
	}

	// 2. templates
	if _, err := tx.Exec("DELETE FROM templates"); err != nil {
		return fmt.Errorf("failed to delete templates: %w", err)
	}

	// 3. items (references sections)
	if _, err := tx.Exec("DELETE FROM items"); err != nil {
		return fmt.Errorf("failed to delete items: %w", err)
	}

	// 4. sections (references lists)
	if _, err := tx.Exec("DELETE FROM sections"); err != nil {
		return fmt.Errorf("failed to delete sections: %w", err)
	}

	// 5. lists
	if _, err := tx.Exec("DELETE FROM lists"); err != nil {
		return fmt.Errorf("failed to delete lists: %w", err)
	}

	// 6. item_history
	if _, err := tx.Exec("DELETE FROM item_history"); err != nil {
		return fmt.Errorf("failed to delete item_history: %w", err)
	}

	// Note: sessions are NOT deleted - user remains logged in

	return tx.Commit()
}
