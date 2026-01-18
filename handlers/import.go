package handlers

import (
	"encoding/csv"
	"fmt"
	"io"
	"shopping-list/db"
	"shopping-list/i18n"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	MaxImportFileSize = 5 * 1024 * 1024 // 5MB
)

// ImportPreviewResponse represents the preview of data to be imported
type ImportPreviewResponse struct {
	Valid            bool             `json:"valid"`
	Error            string           `json:"error,omitempty"`
	Format           string           `json:"format"`
	ListsCount       int              `json:"lists_count"`
	ItemsCount       int              `json:"items_count"`
	TemplatesCount   int              `json:"templates_count"`
	HistoryCount     int              `json:"history_count"`
	Lists            []ImportListInfo `json:"lists"`
	ConflictingLists []string         `json:"conflicting_lists,omitempty"`
}

// ImportListInfo contains info about a list to be imported
type ImportListInfo struct {
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Sections    int    `json:"sections"`
	Items       int    `json:"items"`
	HasConflict bool   `json:"has_conflict"`
}

// ImportRequest contains import options
type ImportRequest struct {
	ConflictResolution string `json:"conflict_resolution"` // "skip", "replace", "copy"
}

// PreviewImport validates and returns a preview of the import data
func PreviewImport(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "No file provided",
		})
	}

	if file.Size > MaxImportFileSize {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "File too large (max 5MB)",
		})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "Failed to open file",
		})
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return c.Status(500).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "Failed to read file",
		})
	}

	// Detect format
	format := detectFormat(file.Filename, data)

	if format == "json" {
		return previewJSONImport(c, data)
	} else if format == "csv" {
		delimiter := c.Query("delimiter", ",")
		return previewCSVImport(c, data, delimiter)
	}

	return c.Status(400).JSON(ImportPreviewResponse{
		Valid: false,
		Error: "Unsupported file format. Use JSON or CSV.",
	})
}

func detectFormat(filename string, data []byte) string {
	if strings.HasSuffix(strings.ToLower(filename), ".json") {
		return "json"
	}
	if strings.HasSuffix(strings.ToLower(filename), ".csv") {
		return "csv"
	}

	// Try to detect by content
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}

	return "csv"
}

func previewJSONImport(c *fiber.Ctx, data []byte) error {
	exportData, err := decodeJSON(data)
	if err != nil {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "Invalid JSON format: " + err.Error(),
		})
	}

	// Validate structure
	if exportData.App != "koffan" && exportData.App != "" {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "This file was not exported from Koffan",
		})
	}

	// Get existing lists for conflict detection
	existingLists, _ := db.GetAllLists()
	existingNames := make(map[string]bool)
	for _, list := range existingLists {
		existingNames[strings.ToLower(list.Name)] = true
	}

	preview := ImportPreviewResponse{
		Valid:            true,
		Format:           "json",
		ListsCount:       len(exportData.Data.Lists),
		TemplatesCount:   len(exportData.Data.Templates),
		HistoryCount:     len(exportData.Data.History),
		Lists:            make([]ImportListInfo, 0, len(exportData.Data.Lists)),
		ConflictingLists: make([]string, 0),
	}

	for _, list := range exportData.Data.Lists {
		// Validate list name length
		if len(list.Name) > MaxListNameLength {
			return c.Status(400).JSON(ImportPreviewResponse{
				Valid: false,
				Error: "List name too long: " + list.Name,
			})
		}

		// Validate reserved name [HISTORY]
		if list.Name == "[HISTORY]" {
			return c.Status(400).JSON(ImportPreviewResponse{
				Valid: false,
				Error: i18n.Get(i18n.GetDefaultLang(), "common.reserved_name"),
			})
		}

		itemCount := 0
		for _, section := range list.Sections {
			// Validate section name length
			if len(section.Name) > MaxSectionNameLength {
				return c.Status(400).JSON(ImportPreviewResponse{
					Valid: false,
					Error: fmt.Sprintf("Section name too long in list '%s': %s", list.Name, section.Name),
				})
			}

			for _, item := range section.Items {
				// Validate item name and description length
				if len(item.Name) > MaxItemNameLength {
					return c.Status(400).JSON(ImportPreviewResponse{
						Valid: false,
						Error: fmt.Sprintf("Item name too long in list '%s': %s", list.Name, item.Name),
					})
				}
				if len(item.Description) > MaxDescriptionLength {
					return c.Status(400).JSON(ImportPreviewResponse{
						Valid: false,
						Error: fmt.Sprintf("Item description too long in list '%s', item '%s'", list.Name, item.Name),
					})
				}
			}
			itemCount += len(section.Items)
		}

		hasConflict := existingNames[strings.ToLower(list.Name)]
		if hasConflict {
			preview.ConflictingLists = append(preview.ConflictingLists, list.Name)
		}

		preview.Lists = append(preview.Lists, ImportListInfo{
			Name:        list.Name,
			Icon:        list.Icon,
			Sections:    len(list.Sections),
			Items:       itemCount,
			HasConflict: hasConflict,
		})
		preview.ItemsCount += itemCount
	}

	return c.JSON(preview)
}

func previewCSVImport(c *fiber.Ctx, data []byte, delimiter string) error {
	// Remove BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	// Set delimiter
	if len(delimiter) > 0 {
		reader.Comma = rune(delimiter[0])
	}

	records, err := reader.ReadAll()
	if err != nil {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "Invalid CSV format: " + err.Error(),
		})
	}

	if len(records) < 2 {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "CSV file is empty or has no data rows",
		})
	}

	// Validate header
	header := records[0]
	if len(header) < 7 {
		return c.Status(400).JSON(ImportPreviewResponse{
			Valid: false,
			Error: "Invalid CSV header. Expected: list_name, list_icon, section_name, item_name, item_description, item_completed, item_uncertain",
		})
	}

	// Get existing lists for conflict detection
	existingLists, _ := db.GetAllLists()
	existingNames := make(map[string]bool)
	for _, list := range existingLists {
		existingNames[strings.ToLower(list.Name)] = true
	}

	// Parse CSV to count lists and items
	listsMap := make(map[string]*ImportListInfo)
	conflicting := make(map[string]bool)
	historyCount := 0

	for i, row := range records[1:] {
		if len(row) < 4 {
			return c.Status(400).JSON(ImportPreviewResponse{
				Valid: false,
				Error: "Invalid row " + strconv.Itoa(i+2) + ": not enough columns",
			})
		}

		listName := strings.TrimSpace(row[0])
		if listName == "" {
			continue
		}

		// Check for history marker
		if listName == "[HISTORY]" {
			historyCount++
			continue
		}

		if len(listName) > MaxListNameLength {
			return c.Status(400).JSON(ImportPreviewResponse{
				Valid: false,
				Error: "List name too long in row " + strconv.Itoa(i+2),
			})
		}

		// Validate item name length
		itemName := strings.TrimSpace(row[3])
		if len(itemName) > MaxItemNameLength {
			return c.Status(400).JSON(ImportPreviewResponse{
				Valid: false,
				Error: fmt.Sprintf("Item name too long in row %d: %s", i+2, itemName),
			})
		}

		// Validate description length if present
		if len(row) > 4 {
			description := strings.TrimSpace(row[4])
			if len(description) > MaxDescriptionLength {
				return c.Status(400).JSON(ImportPreviewResponse{
					Valid: false,
					Error: fmt.Sprintf("Item description too long in row %d", i+2),
				})
			}
		}

		key := strings.ToLower(listName)
		if _, exists := listsMap[key]; !exists {
			icon := "🛒"
			if len(row) > 1 && row[1] != "" {
				icon = row[1]
			}
			hasConflict := existingNames[key]
			if hasConflict {
				conflicting[listName] = true
			}
			listsMap[key] = &ImportListInfo{
				Name:        listName,
				Icon:        icon,
				Sections:    0,
				Items:       0,
				HasConflict: hasConflict,
			}
		}
		listsMap[key].Items++
	}

	preview := ImportPreviewResponse{
		Valid:            true,
		Format:           "csv",
		ListsCount:       len(listsMap),
		ItemsCount:       0,
		HistoryCount:     historyCount,
		Lists:            make([]ImportListInfo, 0, len(listsMap)),
		ConflictingLists: make([]string, 0),
	}

	for name := range conflicting {
		preview.ConflictingLists = append(preview.ConflictingLists, name)
	}

	for _, info := range listsMap {
		preview.Lists = append(preview.Lists, *info)
		preview.ItemsCount += info.Items
	}

	return c.JSON(preview)
}

// ImportData imports data from uploaded file
func ImportData(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file provided"})
	}

	if file.Size > MaxImportFileSize {
		return c.Status(400).JSON(fiber.Map{"error": "File too large (max 5MB)"})
	}

	conflictResolution := c.FormValue("conflict_resolution", "skip")
	if conflictResolution != "skip" && conflictResolution != "replace" && conflictResolution != "copy" {
		conflictResolution = "skip"
	}

	copySuffix := c.FormValue("copy_suffix", "copy")
	delimiter := c.FormValue("delimiter", ",")

	f, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to open file"})
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to read file"})
	}

	format := detectFormat(file.Filename, data)

	if format == "json" {
		return importJSON(c, data, conflictResolution, copySuffix)
	} else if format == "csv" {
		return importCSV(c, data, conflictResolution, copySuffix, delimiter)
	}

	return c.Status(400).JSON(fiber.Map{"error": "Unsupported file format"})
}

func importJSON(c *fiber.Ctx, data []byte, conflictResolution, copySuffix string) error {
	exportData, err := decodeJSON(data)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid JSON format"})
	}

	// Start transaction
	tx, err := db.DB.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start transaction"})
	}
	defer tx.Rollback()

	// Get existing lists for conflict detection
	existingLists, _ := db.GetAllLists()
	existingNames := make(map[string]int64)
	for _, list := range existingLists {
		existingNames[strings.ToLower(list.Name)] = list.ID
	}

	importedLists := 0
	importedItems := 0
	importedTemplates := 0
	importedHistory := 0
	skippedLists := 0

	// Import lists
	for _, exportList := range exportData.Data.Lists {
		// Skip reserved name
		if exportList.Name == "[HISTORY]" {
			skippedLists++
			continue
		}

		// Validate field lengths
		if len(exportList.Name) > MaxListNameLength {
			continue
		}

		existingID, hasConflict := existingNames[strings.ToLower(exportList.Name)]

		if hasConflict {
			switch conflictResolution {
			case "skip":
				skippedLists++
				continue
			case "replace":
				// Delete existing list
				_, err := tx.Exec("DELETE FROM lists WHERE id = ?", existingID)
				if err != nil {
					continue
				}
			case "copy":
				// Find unique name with suffix
				exportList.Name = findUniqueName(exportList.Name, copySuffix, existingNames)
			}
		}

		// Create list with is_active flag preserved
		list, err := db.CreateListTx(tx, exportList.Name, exportList.Icon)
		if err != nil {
			continue
		}

		// Set is_active if it was active in export
		if exportList.IsActive {
			tx.Exec("UPDATE lists SET is_active = TRUE WHERE id = ?", list.ID)
		}

		importedLists++

		// Create sections and items
		sectionOrder := 0
		for _, exportSection := range exportList.Sections {
			// Validate section name
			sectionName := exportSection.Name
			if len(sectionName) > MaxSectionNameLength {
				sectionName = sectionName[:MaxSectionNameLength]
			}

			section, err := db.CreateSectionForListTx(tx, list.ID, sectionName, sectionOrder)
			if err != nil {
				continue
			}
			sectionOrder++

			itemOrder := 0
			for _, exportItem := range exportSection.Items {
				// Validate item fields
				itemName := exportItem.Name
				if len(itemName) > MaxItemNameLength {
					itemName = itemName[:MaxItemNameLength]
				}
				itemDesc := exportItem.Description
				if len(itemDesc) > MaxDescriptionLength {
					itemDesc = itemDesc[:MaxDescriptionLength]
				}

				item, err := db.CreateItemTx(tx, section.ID, itemName, itemDesc, itemOrder)
				if err != nil {
					continue
				}
				itemOrder++

				// Set completed and uncertain flags directly
				if exportItem.Completed {
					tx.Exec("UPDATE items SET completed = TRUE WHERE id = ?", item.ID)
				}
				if exportItem.Uncertain {
					tx.Exec("UPDATE items SET uncertain = TRUE WHERE id = ?", item.ID)
				}

				importedItems++
			}
		}
	}

	// Import templates
	for _, exportTemplate := range exportData.Data.Templates {
		template, err := db.CreateTemplate(exportTemplate.Name, exportTemplate.Description)
		if err != nil {
			continue
		}

		for _, item := range exportTemplate.Items {
			db.AddTemplateItem(template.ID, item.SectionName, item.Name, item.Description)
		}
		importedTemplates++
	}

	// Import history with usage count preserved
	for _, h := range exportData.Data.History {
		usageCount := h.UsageCount
		if usageCount < 1 {
			usageCount = 1
		}
		sectionID := db.GetSectionIDByNameTx(tx, h.LastSection)
		err := db.SaveItemHistoryWithCountTx(tx, h.Name, sectionID, usageCount)
		if err == nil {
			importedHistory++
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit import"})
	}

	return c.JSON(fiber.Map{
		"success":            true,
		"imported_lists":     importedLists,
		"imported_items":     importedItems,
		"imported_templates": importedTemplates,
		"imported_history":   importedHistory,
		"skipped_lists":      skippedLists,
	})
}

func importCSV(c *fiber.Ctx, data []byte, conflictResolution, copySuffix, delimiter string) error {
	// Remove BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	// Set delimiter
	if len(delimiter) > 0 {
		reader.Comma = rune(delimiter[0])
	}

	records, err := reader.ReadAll()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid CSV format"})
	}

	if len(records) < 2 {
		return c.Status(400).JSON(fiber.Map{"error": "CSV file is empty"})
	}

	// Start transaction
	tx, err := db.DB.Begin()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to start transaction"})
	}
	defer tx.Rollback()

	// Get existing lists for conflict detection
	existingLists, _ := db.GetAllLists()
	existingNames := make(map[string]int64)
	for _, list := range existingLists {
		existingNames[strings.ToLower(list.Name)] = list.ID
	}

	// Track created lists and sections
	createdLists := make(map[string]*db.List)
	createdSections := make(map[string]map[string]*db.Section) // list key -> section name -> section
	sectionOrders := make(map[string]int)                      // list key -> next section order
	itemOrders := make(map[int64]int)                          // section id -> next item order

	importedLists := 0
	importedItems := 0
	importedHistory := 0
	skippedLists := 0
	skippedListNames := make(map[string]bool)

	// Get default section name from i18n
	defaultSectionName := i18n.Get(i18n.GetDefaultLang(), "sections.default")
	if defaultSectionName == "sections.default" {
		// Fallback if key not found
		defaultSectionName = "General"
	}

	// Skip header row
	for _, row := range records[1:] {
		if len(row) < 4 {
			continue
		}

		listName := strings.TrimSpace(row[0])
		if listName == "" {
			continue
		}

		// Handle history rows
		// Format: [HISTORY],,item_name,last_section,usage_count,,
		if listName == "[HISTORY]" {
			itemName := ""
			if len(row) > 2 {
				itemName = strings.TrimSpace(row[2])
			}
			if itemName != "" {
				// Get last section name from column 3
				lastSectionName := ""
				if len(row) > 3 {
					lastSectionName = strings.TrimSpace(row[3])
				}

				// Get usage count from column 4
				usageCount := 1
				if len(row) > 4 {
					if count, err := strconv.Atoi(strings.TrimSpace(row[4])); err == nil && count > 0 {
						usageCount = count
					}
				}

				// Find section ID by name
				sectionID := db.GetSectionIDByNameTx(tx, lastSectionName)

				err := db.SaveItemHistoryWithCountTx(tx, itemName, sectionID, usageCount)
				if err == nil {
					importedHistory++
				}
			}
			continue
		}

		listKey := strings.ToLower(listName)

		// Check if list was skipped due to conflict
		if skippedListNames[listKey] {
			continue
		}

		// Validate list name
		if len(listName) > MaxListNameLength {
			listName = listName[:MaxListNameLength]
			listKey = strings.ToLower(listName)
		}

		listIcon := "🛒"
		if len(row) > 1 && row[1] != "" {
			listIcon = row[1]
			if len(listIcon) > MaxIconLength {
				listIcon = "🛒"
			}
		}
		sectionName := ""
		if len(row) > 2 {
			sectionName = strings.TrimSpace(row[2])
		}
		itemName := strings.TrimSpace(row[3])
		itemDescription := ""
		if len(row) > 4 {
			itemDescription = strings.TrimSpace(row[4])
		}
		itemCompleted := false
		if len(row) > 5 {
			itemCompleted = strings.ToLower(strings.TrimSpace(row[5])) == "true"
		}
		itemUncertain := false
		if len(row) > 6 {
			itemUncertain = strings.ToLower(strings.TrimSpace(row[6])) == "true"
		}

		// Validate item fields
		if len(itemName) > MaxItemNameLength {
			itemName = itemName[:MaxItemNameLength]
		}
		if len(itemDescription) > MaxDescriptionLength {
			itemDescription = itemDescription[:MaxDescriptionLength]
		}

		// Get or create list
		list, exists := createdLists[listKey]
		if !exists {
			existingID, hasConflict := existingNames[listKey]

			if hasConflict {
				switch conflictResolution {
				case "skip":
					skippedLists++
					skippedListNames[listKey] = true
					continue
				case "replace":
					tx.Exec("DELETE FROM lists WHERE id = ?", existingID)
				case "copy":
					listName = findUniqueName(listName, copySuffix, existingNames)
					listKey = strings.ToLower(listName)
				}
			}

			newList, err := db.CreateListTx(tx, listName, listIcon)
			if err != nil {
				continue
			}
			list = newList
			createdLists[listKey] = list
			createdSections[listKey] = make(map[string]*db.Section)
			sectionOrders[listKey] = 0
			importedLists++
		}

		// Get or create section
		if sectionName == "" {
			sectionName = defaultSectionName
		}
		if len(sectionName) > MaxSectionNameLength {
			sectionName = sectionName[:MaxSectionNameLength]
		}
		sectionKey := strings.ToLower(sectionName)
		section, exists := createdSections[listKey][sectionKey]
		if !exists {
			newSection, err := db.CreateSectionForListTx(tx, list.ID, sectionName, sectionOrders[listKey])
			if err != nil {
				continue
			}
			section = newSection
			createdSections[listKey][sectionKey] = section
			sectionOrders[listKey]++
			itemOrders[section.ID] = 0
		}

		// Create item
		if itemName != "" {
			item, err := db.CreateItemTx(tx, section.ID, itemName, itemDescription, itemOrders[section.ID])
			if err != nil {
				continue
			}
			itemOrders[section.ID]++

			if itemCompleted {
				tx.Exec("UPDATE items SET completed = TRUE WHERE id = ?", item.ID)
			}
			if itemUncertain {
				tx.Exec("UPDATE items SET uncertain = TRUE WHERE id = ?", item.ID)
			}

			importedItems++
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit import"})
	}

	return c.JSON(fiber.Map{
		"success":          true,
		"imported_lists":   importedLists,
		"imported_items":   importedItems,
		"imported_history": importedHistory,
		"skipped_lists":    skippedLists,
	})
}

// findUniqueName finds a unique list name by adding suffix with incrementing number
// It also updates existingNames map to prevent collisions within the same import
func findUniqueName(baseName, suffix string, existingNames map[string]int64) string {
	// First try with just the suffix
	candidateName := fmt.Sprintf("%s (%s)", baseName, suffix)
	candidateKey := strings.ToLower(candidateName)
	if _, exists := existingNames[candidateKey]; !exists {
		// Mark as used to prevent collision in same import batch
		existingNames[candidateKey] = -1
		return candidateName
	}

	// Try with incrementing numbers
	for i := 2; i <= 100; i++ {
		candidateName = fmt.Sprintf("%s (%s %d)", baseName, suffix, i)
		candidateKey = strings.ToLower(candidateName)
		if _, exists := existingNames[candidateKey]; !exists {
			// Mark as used to prevent collision in same import batch
			existingNames[candidateKey] = -1
			return candidateName
		}
	}

	// Fallback - return with timestamp
	return fmt.Sprintf("%s (%s %d)", baseName, suffix, 9999)
}
