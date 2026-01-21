package api

import (
	"shopping-list/db"
	"unicode"
)

// ErrorResponse represents an API error
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ListsResponse wraps multiple lists
type ListsResponse struct {
	Lists []db.List `json:"lists"`
}

// SectionsResponse wraps multiple sections
type SectionsResponse struct {
	Sections []db.Section `json:"sections"`
}

// ItemsResponse wraps multiple items
type ItemsResponse struct {
	Items []db.Item `json:"items"`
}

// BatchCreateRequest represents the request body for batch creation
type BatchCreateRequest struct {
	// Option 1: Create new list with nested sections/items
	List *BatchListInput `json:"list,omitempty"`

	// Option 2: Add sections to existing list
	ListID   int64               `json:"list_id,omitempty"`
	Sections []BatchSectionInput `json:"sections,omitempty"`

	// Option 3: Add items to existing section
	SectionID int64            `json:"section_id,omitempty"`
	Items     []BatchItemInput `json:"items,omitempty"`
}

// BatchListInput represents a new list with nested sections/items
type BatchListInput struct {
	Name     string              `json:"name"`
	Icon     string              `json:"icon,omitempty"`
	Sections []BatchSectionInput `json:"sections,omitempty"`
}

// BatchSectionInput represents a section with nested items
type BatchSectionInput struct {
	Name  string           `json:"name"`
	Items []BatchItemInput `json:"items,omitempty"`
}

// BatchItemInput represents an item for creation
type BatchItemInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// BatchCreateResponse represents the response from batch creation
type BatchCreateResponse struct {
	List     *db.List     `json:"list,omitempty"`
	Sections []db.Section `json:"sections,omitempty"`
	Items    []db.Item    `json:"items,omitempty"`
}

// CreateListRequest for creating a new list
type CreateListRequest struct {
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
}

// UpdateListRequest for updating a list
type UpdateListRequest struct {
	Name string `json:"name,omitempty"`
	Icon string `json:"icon,omitempty"`
}

// CreateSectionRequest for creating a new section
type CreateSectionRequest struct {
	ListID int64  `json:"list_id"`
	Name   string `json:"name"`
}

// UpdateSectionRequest for updating a section
type UpdateSectionRequest struct {
	Name string `json:"name"`
}

// CreateItemRequest for creating a new item
type CreateItemRequest struct {
	SectionID   int64  `json:"section_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Quantity    int    `json:"quantity,omitempty"`
}

// UpdateItemRequest for updating an item
type UpdateItemRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Quantity    *int   `json:"quantity,omitempty"`
	Completed   *bool  `json:"completed,omitempty"`
	Uncertain   *bool  `json:"uncertain,omitempty"`
}

// MoveItemRequest for moving item to another section
type MoveItemRequest struct {
	SectionID int64 `json:"section_id"`
}

// iconAliases maps string aliases to emoji icons
var iconAliases = map[string]string{
	"cart":      "🛒",
	"shopping":  "🛒",
	"home":      "🏠",
	"house":     "🏠",
	"gift":      "🎁",
	"present":   "🎁",
	"christmas": "🎄",
	"xmas":      "🎄",
	"birthday":  "🎂",
	"cake":      "🎂",
	"food":      "🍕",
	"pizza":     "🍕",
	"salad":     "🥗",
	"healthy":   "🥗",
	"medicine":  "💊",
	"health":    "💊",
	"pills":     "💊",
	"pet":       "🐕",
	"pets":      "🐕",
	"dog":       "🐕",
	"cleaning":  "🧹",
	"clean":     "🧹",
	"package":   "📦",
	"packages":  "📦",
	"box":       "📦",
	"travel":    "✈️",
	"trip":      "✈️",
	"flight":    "✈️",
	"fitness":   "🏋️",
	"gym":       "🏋️",
	"workout":   "🏋️",
	"books":     "📚",
	"book":      "📚",
	"reading":   "📚",
	"tools":     "🛠️",
	"tool":      "🛠️",
	"work":      "💼",
	"office":    "💼",
	"business":  "💼",
}

// DefaultIcon is the fallback icon when invalid input is provided
const DefaultIcon = "🛒"

// isEmoji checks if a string starts with an emoji character
func isEmoji(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		// Check for common emoji ranges
		if r >= 0x1F300 && r <= 0x1F9FF { // Miscellaneous Symbols and Pictographs, Emoticons, etc.
			return true
		}
		if r >= 0x2600 && r <= 0x26FF { // Miscellaneous Symbols
			return true
		}
		if r >= 0x2700 && r <= 0x27BF { // Dingbats
			return true
		}
		if r >= 0x1F600 && r <= 0x1F64F { // Emoticons
			return true
		}
		if r >= 0x1F680 && r <= 0x1F6FF { // Transport and Map Symbols
			return true
		}
		// If first rune is a letter or digit, it's not an emoji
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return false
}

// NormalizeIcon converts string aliases to emoji, validates emoji input,
// or returns default icon for invalid input
func NormalizeIcon(icon string) string {
	if icon == "" {
		return ""
	}
	// Check if it's a known alias
	if emoji, ok := iconAliases[icon]; ok {
		return emoji
	}
	// Check if it's already a valid emoji
	if isEmoji(icon) {
		return icon
	}
	// Invalid input - return default icon
	return DefaultIcon
}
