package db

import (
	"time"
)

// Section represents a shopping list section
type Section struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
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
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

// Session represents a user session
type Session struct {
	ID        string
	ExpiresAt int64
}

// ==================== SECTIONS ====================

func GetAllSections() ([]Section, error) {
	rows, err := DB.Query(`
		SELECT id, name, sort_order, created_at
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
		err := rows.Scan(&s.ID, &s.Name, &s.SortOrder, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		// Get items for this section
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
		SELECT id, name, sort_order, created_at
		FROM sections WHERE id = ?
	`, id).Scan(&s.ID, &s.Name, &s.SortOrder, &s.CreatedAt)
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
	// Get max sort_order
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM sections").Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO sections (name, sort_order) VALUES (?, ?)
	`, name, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetSectionByID(id)
}

func UpdateSection(id int64, name string) (*Section, error) {
	_, err := DB.Exec(`UPDATE sections SET name = ? WHERE id = ?`, name, id)
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
	var currentOrder int
	err := DB.QueryRow("SELECT sort_order FROM sections WHERE id = ?", id).Scan(&currentOrder)
	if err != nil {
		return err
	}

	if currentOrder == 0 {
		return nil // Already at top
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Swap with previous section
	_, err = tx.Exec(`
		UPDATE sections SET sort_order = sort_order + 1
		WHERE sort_order = ?
	`, currentOrder-1)
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
	var currentOrder, maxOrder int
	err := DB.QueryRow("SELECT sort_order FROM sections WHERE id = ?", id).Scan(&currentOrder)
	if err != nil {
		return err
	}
	DB.QueryRow("SELECT MAX(sort_order) FROM sections").Scan(&maxOrder)

	if currentOrder >= maxOrder {
		return nil // Already at bottom
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Swap with next section
	_, err = tx.Exec(`
		UPDATE sections SET sort_order = sort_order - 1
		WHERE sort_order = ?
	`, currentOrder+1)
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
		SELECT id, section_id, name, description, completed, uncertain, sort_order, created_at
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
		err := rows.Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.SortOrder, &i.CreatedAt)
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
		SELECT id, section_id, name, description, completed, uncertain, sort_order, created_at
		FROM items WHERE id = ?
	`, id).Scan(&i.ID, &i.SectionID, &i.Name, &i.Description, &i.Completed, &i.Uncertain, &i.SortOrder, &i.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func CreateItem(sectionID int64, name, description string) (*Item, error) {
	// Get max sort_order for this section
	var maxOrder int
	DB.QueryRow("SELECT COALESCE(MAX(sort_order), -1) FROM items WHERE section_id = ?", sectionID).Scan(&maxOrder)

	result, err := DB.Exec(`
		INSERT INTO items (section_id, name, description, sort_order) VALUES (?, ?, ?, ?)
	`, sectionID, name, description, maxOrder+1)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return GetItemByID(id)
}

func UpdateItem(id int64, name, description string) (*Item, error) {
	_, err := DB.Exec(`
		UPDATE items SET name = ?, description = ? WHERE id = ?
	`, name, description, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func DeleteItem(id int64) error {
	_, err := DB.Exec(`DELETE FROM items WHERE id = ?`, id)
	return err
}

func ToggleItemCompleted(id int64) (*Item, error) {
	_, err := DB.Exec(`UPDATE items SET completed = NOT completed WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func ToggleItemUncertain(id int64) (*Item, error) {
	_, err := DB.Exec(`UPDATE items SET uncertain = NOT uncertain WHERE id = ?`, id)
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
		UPDATE items SET section_id = ?, sort_order = ? WHERE id = ?
	`, newSectionID, maxOrder+1, id)
	if err != nil {
		return nil, err
	}
	return GetItemByID(id)
}

func MoveItemUp(id int64) error {
	item, err := GetItemByID(id)
	if err != nil {
		return err
	}

	if item.SortOrder == 0 {
		return nil // Already at top
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Swap with previous item in same section
	_, err = tx.Exec(`
		UPDATE items SET sort_order = sort_order + 1
		WHERE section_id = ? AND sort_order = ?
	`, item.SectionID, item.SortOrder-1)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE items SET sort_order = ? WHERE id = ?
	`, item.SortOrder-1, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func MoveItemDown(id int64) error {
	item, err := GetItemByID(id)
	if err != nil {
		return err
	}

	var maxOrder int
	DB.QueryRow("SELECT MAX(sort_order) FROM items WHERE section_id = ?", item.SectionID).Scan(&maxOrder)

	if item.SortOrder >= maxOrder {
		return nil // Already at bottom
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Swap with next item in same section
	_, err = tx.Exec(`
		UPDATE items SET sort_order = sort_order - 1
		WHERE section_id = ? AND sort_order = ?
	`, item.SectionID, item.SortOrder+1)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE items SET sort_order = ? WHERE id = ?
	`, item.SortOrder+1, id)
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
	var stats Stats
	DB.QueryRow("SELECT COUNT(*) FROM items").Scan(&stats.TotalItems)
	DB.QueryRow("SELECT COUNT(*) FROM items WHERE completed = TRUE").Scan(&stats.CompletedItems)
	if stats.TotalItems > 0 {
		stats.Percentage = (stats.CompletedItems * 100) / stats.TotalItems
	}
	return stats
}

// ==================== PREFERENCES ====================

type Preferences struct {
	ID           int64  `json:"id"`
	MobileHelper string `json:"mobile_helper"` // "button" or "progress"
}

func GetPreferences() Preferences {
	var prefs Preferences
	err := DB.QueryRow("SELECT id, mobile_helper FROM preferences WHERE id = 1").Scan(&prefs.ID, &prefs.MobileHelper)
	if err != nil {
		// Return defaults
		return Preferences{ID: 1, MobileHelper: "button"}
	}
	return prefs
}

func UpdatePreferences(mobileHelper string) error {
	_, err := DB.Exec(`
		UPDATE preferences SET mobile_helper = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1
	`, mobileHelper)
	return err
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
