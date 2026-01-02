package api

import "shopping-list/db"

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
}

// UpdateItemRequest for updating an item
type UpdateItemRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
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

// NormalizeIcon converts string aliases to emoji, or returns the original if already emoji
func NormalizeIcon(icon string) string {
	if icon == "" {
		return ""
	}
	if emoji, ok := iconAliases[icon]; ok {
		return emoji
	}
	return icon
}
