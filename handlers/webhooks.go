package handlers

import (
	"log"
	"shopping-list/db"
	"shopping-list/webhook"
)

type webhookEntity struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type itemWebhookData struct {
	Item    *db.Item       `json:"item"`
	Section *webhookEntity `json:"section,omitempty"`
	List    *webhookEntity `json:"list,omitempty"`
}

// PreparedItemWebhook keeps payload context available after a cascading delete.
type PreparedItemWebhook struct {
	data itemWebhookData
}

// NotifyItemWebhook adds list and section context to an item webhook event.
func NotifyItemWebhook(event string, item *db.Item) {
	if item == nil || !webhook.Accepts(event) {
		return
	}
	webhook.Notify(event, buildItemWebhookData(event, item))
}

func buildItemWebhookData(event string, item *db.Item) itemWebhookData {
	data := itemWebhookData{Item: item}
	var section webhookEntity
	var list webhookEntity
	err := db.DB.QueryRow(`
		SELECT s.id, s.name, l.id, l.name
		FROM sections s
		JOIN lists l ON l.id = s.list_id
		WHERE s.id = ?
	`, item.SectionID).Scan(&section.ID, &section.Name, &list.ID, &list.Name)
	if err != nil {
		log.Printf("Could not load list context for webhook event %s: %v", event, err)
	} else {
		data.Section = &section
		data.List = &list
	}
	return data
}

// PrepareItemWebhooks captures payload context before a cascading delete.
func PrepareItemWebhooks(event string, items []db.Item) []PreparedItemWebhook {
	if !webhook.Accepts(event) {
		return nil
	}

	prepared := make([]PreparedItemWebhook, 0, len(items))
	for index := range items {
		prepared = append(prepared, PreparedItemWebhook{data: buildItemWebhookData(event, &items[index])})
	}
	return prepared
}

// NotifyPreparedItemWebhooks emits events captured before a cascading delete.
func NotifyPreparedItemWebhooks(event string, prepared []PreparedItemWebhook) {
	for _, item := range prepared {
		webhook.Notify(event, item.data)
	}
}

// SnapshotSectionItems captures all items in a section before a bulk operation.
func SnapshotSectionItems(sectionID int64) []db.Item {
	section, err := db.GetSectionByID(sectionID)
	if err != nil {
		log.Printf("Could not snapshot section %d for webhook events: %v", sectionID, err)
		return nil
	}
	return section.Items
}

// PrepareSectionItemWebhooks avoids reading section items when the event is disabled.
func PrepareSectionItemWebhooks(event string, sectionID int64) []PreparedItemWebhook {
	if !webhook.Accepts(event) {
		return nil
	}
	return PrepareItemWebhooks(event, SnapshotSectionItems(sectionID))
}

// SnapshotListItems captures all items in a list before a bulk operation.
func SnapshotListItems(listID int64) []db.Item {
	sections, err := db.GetSectionsByList(listID)
	if err != nil {
		log.Printf("Could not snapshot list %d for webhook events: %v", listID, err)
		return nil
	}

	var items []db.Item
	for _, section := range sections {
		items = append(items, section.Items...)
	}
	return items
}

// PrepareListItemWebhooks avoids reading list items when the event is disabled.
func PrepareListItemWebhooks(event string, listID int64) []PreparedItemWebhook {
	if !webhook.Accepts(event) {
		return nil
	}
	return PrepareItemWebhooks(event, SnapshotListItems(listID))
}

// PrepareAllItemWebhooks captures all item payloads before clearing the database.
func PrepareAllItemWebhooks(event string) []PreparedItemWebhook {
	if !webhook.Accepts(event) {
		return nil
	}

	lists, err := db.GetAllLists()
	if err != nil {
		log.Printf("Could not snapshot database for webhook events: %v", err)
		return nil
	}

	var prepared []PreparedItemWebhook
	for _, list := range lists {
		prepared = append(prepared, PrepareItemWebhooks(event, SnapshotListItems(list.ID))...)
	}
	return prepared
}

// NotifyItemWebhooks emits one event for each item affected by a bulk operation.
func NotifyItemWebhooks(event string, items []db.Item) {
	for index := range items {
		NotifyItemWebhook(event, &items[index])
	}
}

// NotifyCreatedListItems emits created events for items added after a snapshot.
func NotifyCreatedListItems(listID int64, before []db.Item) {
	if !webhook.Accepts(webhook.EventItemCreated) {
		return
	}
	existingIDs := make(map[int64]struct{}, len(before))
	for _, item := range before {
		existingIDs[item.ID] = struct{}{}
	}

	for _, item := range SnapshotListItems(listID) {
		if _, existed := existingIDs[item.ID]; !existed {
			NotifyItemWebhook(webhook.EventItemCreated, &item)
		}
	}
}
