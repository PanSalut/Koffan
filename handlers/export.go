package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"shopping-list/db"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ExportData represents the full export structure
type ExportData struct {
	Version    string     `json:"version"`
	ExportedAt string     `json:"exported_at"`
	App        string     `json:"app"`
	Data       ExportBody `json:"data"`
}

// ExportBody contains the actual data
type ExportBody struct {
	Lists     []ExportList     `json:"lists"`
	Templates []ExportTemplate `json:"templates,omitempty"`
	History   []ExportHistory  `json:"history,omitempty"`
}

// ExportList represents a list with sections and items
type ExportList struct {
	Name     string          `json:"name"`
	Icon     string          `json:"icon"`
	IsActive bool            `json:"is_active"`
	Sections []ExportSection `json:"sections"`
}

// ExportSection represents a section with items
type ExportSection struct {
	Name  string       `json:"name"`
	Items []ExportItem `json:"items"`
}

// ExportItem represents a shopping item
type ExportItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Completed   bool   `json:"completed"`
	Uncertain   bool   `json:"uncertain"`
}

// ExportTemplate represents a template
type ExportTemplate struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Items       []ExportTemplateItem `json:"items"`
}

// ExportTemplateItem represents a template item
type ExportTemplateItem struct {
	SectionName string `json:"section_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ExportHistory represents history item
type ExportHistory struct {
	Name        string `json:"name"`
	LastSection string `json:"last_section"`
	UsageCount  int    `json:"usage_count"`
}

// ExportAllData exports all data as JSON or CSV
func ExportAllData(c *fiber.Ctx) error {
	format := c.Query("format", "json")
	includeTemplates := c.Query("include_templates", "true") == "true"
	includeHistory := c.Query("include_history", "true") == "true"

	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch lists"})
	}

	if format == "csv" {
		return exportAllAsCSV(c, lists)
	}

	return exportAllAsJSON(c, lists, includeTemplates, includeHistory)
}

// ExportSingleList exports a single list
func ExportSingleList(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid list ID"})
	}

	format := c.Query("format", "json")

	list, err := db.GetListByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "List not found"})
	}

	sections, err := db.GetSectionsByList(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch sections"})
	}

	if format == "csv" {
		return exportListAsCSV(c, list, sections)
	}

	return exportListAsJSON(c, list, sections)
}

func exportAllAsJSON(c *fiber.Ctx, lists []db.List, includeTemplates, includeHistory bool) error {
	exportData := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		App:        "koffan",
		Data: ExportBody{
			Lists: make([]ExportList, 0, len(lists)),
		},
	}

	for _, list := range lists {
		sections, err := db.GetSectionsByList(list.ID)
		if err != nil {
			continue
		}

		exportList := ExportList{
			Name:     list.Name,
			Icon:     list.Icon,
			IsActive: list.IsActive,
			Sections: make([]ExportSection, 0, len(sections)),
		}

		for _, section := range sections {
			exportSection := ExportSection{
				Name:  section.Name,
				Items: make([]ExportItem, 0, len(section.Items)),
			}

			for _, item := range section.Items {
				exportSection.Items = append(exportSection.Items, ExportItem{
					Name:        item.Name,
					Description: item.Description,
					Completed:   item.Completed,
					Uncertain:   item.Uncertain,
				})
			}

			exportList.Sections = append(exportList.Sections, exportSection)
		}

		exportData.Data.Lists = append(exportData.Data.Lists, exportList)
	}

	// Include templates if requested
	if includeTemplates {
		templates, err := db.GetAllTemplates()
		if err == nil {
			exportData.Data.Templates = make([]ExportTemplate, 0, len(templates))
			for _, tmpl := range templates {
				exportTemplate := ExportTemplate{
					Name:        tmpl.Name,
					Description: tmpl.Description,
					Items:       make([]ExportTemplateItem, 0, len(tmpl.Items)),
				}
				for _, item := range tmpl.Items {
					exportTemplate.Items = append(exportTemplate.Items, ExportTemplateItem{
						SectionName: item.SectionName,
						Name:        item.Name,
						Description: item.Description,
					})
				}
				exportData.Data.Templates = append(exportData.Data.Templates, exportTemplate)
			}
		}
	}

	// Include history if requested
	if includeHistory {
		historyItems, err := db.GetAllItemSuggestions(1000)
		if err == nil {
			exportData.Data.History = make([]ExportHistory, 0, len(historyItems))
			for _, h := range historyItems {
				sectionName := h.LastSectionName
				// Fallback: if no section in history, find where item currently exists
				if sectionName == "" {
					sectionName = db.GetSectionNameForItem(h.Name)
				}
				exportData.Data.History = append(exportData.Data.History, ExportHistory{
					Name:        h.Name,
					LastSection: sectionName,
					UsageCount:  h.UsageCount,
				})
			}
		}
	}

	filename := fmt.Sprintf("koffan-export-%s.json", time.Now().Format("2006-01-02"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Set("Content-Type", "application/json")

	return c.JSON(exportData)
}

func exportListAsJSON(c *fiber.Ctx, list *db.List, sections []db.Section) error {
	exportData := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		App:        "koffan",
		Data: ExportBody{
			Lists: make([]ExportList, 0, 1),
		},
	}

	exportList := ExportList{
		Name:     list.Name,
		Icon:     list.Icon,
		IsActive: list.IsActive,
		Sections: make([]ExportSection, 0, len(sections)),
	}

	for _, section := range sections {
		exportSection := ExportSection{
			Name:  section.Name,
			Items: make([]ExportItem, 0, len(section.Items)),
		}

		for _, item := range section.Items {
			exportSection.Items = append(exportSection.Items, ExportItem{
				Name:        item.Name,
				Description: item.Description,
				Completed:   item.Completed,
				Uncertain:   item.Uncertain,
			})
		}

		exportList.Sections = append(exportList.Sections, exportSection)
	}

	exportData.Data.Lists = append(exportData.Data.Lists, exportList)

	filename := fmt.Sprintf("koffan-%s-%s.json", sanitizeFilename(list.Name), time.Now().Format("2006-01-02"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Set("Content-Type", "application/json")

	return c.JSON(exportData)
}

func exportAllAsCSV(c *fiber.Ctx, lists []db.List) error {
	includeHistory := c.Query("include_history", "true") == "true"
	delimiter := c.Query("delimiter", ",")

	filename := fmt.Sprintf("koffan-export-%s.csv", time.Now().Format("2006-01-02"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Set("Content-Type", "text/csv; charset=utf-8")

	// Write BOM for Excel compatibility
	c.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c.Response().BodyWriter())
	// Set delimiter
	if len(delimiter) > 0 {
		writer.Comma = rune(delimiter[0])
	}
	defer writer.Flush()

	// Header
	writer.Write([]string{"list_name", "list_icon", "section_name", "item_name", "item_description", "item_completed", "item_uncertain"})

	for _, list := range lists {
		sections, err := db.GetSectionsByList(list.ID)
		if err != nil {
			continue
		}

		hasItems := false
		for _, section := range sections {
			for _, item := range section.Items {
				hasItems = true
				writer.Write([]string{
					list.Name,
					list.Icon,
					section.Name,
					item.Name,
					item.Description,
					strconv.FormatBool(item.Completed),
					strconv.FormatBool(item.Uncertain),
				})
			}
		}

		// Export empty list with just name and icon
		if !hasItems {
			writer.Write([]string{
				list.Name,
				list.Icon,
				"",
				"",
				"",
				"",
				"",
			})
		}
	}

	// Export history if requested
	// Format: [HISTORY],,item_name,last_section,usage_count,,
	if includeHistory {
		historyItems, err := db.GetAllItemSuggestions(1000)
		if err == nil {
			for _, h := range historyItems {
				sectionName := h.LastSectionName
				// Fallback: if no section in history, find where item currently exists
				if sectionName == "" {
					sectionName = db.GetSectionNameForItem(h.Name)
				}
				writer.Write([]string{
					"[HISTORY]",
					"",
					h.Name,
					sectionName,
					strconv.Itoa(h.UsageCount),
					"",
					"",
				})
			}
		}
	}

	return nil
}

func exportListAsCSV(c *fiber.Ctx, list *db.List, sections []db.Section) error {
	filename := fmt.Sprintf("koffan-%s-%s.csv", sanitizeFilename(list.Name), time.Now().Format("2006-01-02"))
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Set("Content-Type", "text/csv; charset=utf-8")

	// Write BOM for Excel compatibility
	c.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(c.Response().BodyWriter())
	defer writer.Flush()

	// Header
	writer.Write([]string{"list_name", "list_icon", "section_name", "item_name", "item_description", "item_completed", "item_uncertain"})

	for _, section := range sections {
		for _, item := range section.Items {
			writer.Write([]string{
				list.Name,
				list.Icon,
				section.Name,
				item.Name,
				item.Description,
				strconv.FormatBool(item.Completed),
				strconv.FormatBool(item.Uncertain),
			})
		}
	}

	return nil
}

// sanitizeFilename removes or replaces characters that are not safe for filenames
func sanitizeFilename(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == ' ' {
			result = append(result, '-')
		}
	}
	if len(result) == 0 {
		return "list"
	}
	return string(result)
}

// GetExportPreview returns a preview of what will be exported (for UI)
func GetExportPreview(c *fiber.Ctx) error {
	lists, err := db.GetAllLists()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch lists"})
	}

	templates, _ := db.GetAllTemplates()
	history, _ := db.GetAllItemSuggestions(100)

	totalItems := 0
	for _, list := range lists {
		totalItems += list.Stats.TotalItems
	}

	return c.JSON(fiber.Map{
		"lists_count":     len(lists),
		"items_count":     totalItems,
		"templates_count": len(templates),
		"history_count":   len(history),
	})
}

// decodeJSON helper for import
func decodeJSON(data []byte) (*ExportData, error) {
	var exportData ExportData
	if err := json.Unmarshal(data, &exportData); err != nil {
		return nil, err
	}
	return &exportData, nil
}
