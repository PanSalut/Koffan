package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func initOfflineDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "offline.db"))
	Init()
	t.Cleanup(Close)
}

func offlineOperation(entity, action string, id int64, fields map[string]interface{}) OfflineOperation {
	values := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		values[key], _ = json.Marshal(value)
	}
	return OfflineOperation{ID: uuid.NewString(), Entity: entity, Action: action, EntityID: id, Values: values}
}

func offlineCreateTree() []OfflineOperation {
	return []OfflineOperation{
		offlineOperation("list", "create", -1, map[string]interface{}{"name": "Weekly"}),
		offlineOperation("section", "create", -1, map[string]interface{}{"name": "Dairy", "list_id": -1}),
		offlineOperation("item", "create", -1, map[string]interface{}{"name": "Milk", "section_id": -1}),
	}
}

func requireOfflineError(t *testing.T, err error, conflict bool, operationID string) {
	t.Helper()
	var inputErr *OfflineError
	if !errors.As(err, &inputErr) || inputErr.Conflict != conflict || inputErr.OperationID != operationID {
		t.Fatalf("error = %#v, want conflict %v, operation %s", err, conflict, operationID)
	}
}

func TestOfflineAtomicCRUDAndLostAcknowledgement(t *testing.T) {
	initOfflineDatabase(t)
	operations := offlineCreateTree()
	operations = append(operations,
		offlineOperation("list", "create", -2, map[string]interface{}{"name": "Second"}),
		offlineOperation("section", "create", -2, map[string]interface{}{"name": "Other", "list_id": -2}),
		offlineOperation("item", "update", -1, map[string]interface{}{"name": "Oat milk", "description": "Unsweetened", "quantity": 2, "completed": true, "uncertain": true, "sort_order": 3}),
		offlineOperation("item", "update", -1, map[string]interface{}{"section_id": -2}),
		offlineOperation("section", "update", -2, map[string]interface{}{"name": "Vegan", "sort_mode": "alphabetical_desc", "sort_order": 2}),
		offlineOperation("list", "update", -2, map[string]interface{}{"name": "Weekend", "icon": "🥛", "show_completed": false, "sort_order": 2}),
		offlineOperation("list", "delete", -1, nil),
	)
	request := OfflineSyncRequest{ClientID: uuid.NewString(), Operations: operations}
	response, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !response.Changed || len(response.Results) != len(operations) {
		t.Fatalf("unexpected response: %#v", response)
	}
	snapshot := response.Snapshot
	if len(snapshot.Lists) != 1 || len(snapshot.Sections) != 1 || len(snapshot.Items) != 1 {
		t.Fatalf("cascade or move failed: %#v", snapshot)
	}
	if list := snapshot.Lists[0]; list.Name != "Weekend" || list.Icon != "🥛" || list.ShowCompleted || list.SortOrder != 2 || list.Stats.TotalItems != 1 || list.Stats.CompletedItems != 1 {
		t.Fatalf("list = %#v", list)
	}
	if section := snapshot.Sections[0]; section.Name != "Vegan" || section.SortMode != "alphabetical_desc" || section.SortOrder != 2 || section.ListID != snapshot.Lists[0].ID {
		t.Fatalf("section = %#v", section)
	}
	item := snapshot.Items[0]
	if item.Name != "Oat milk" || item.Description != "Unsweetened" || item.Quantity != 2 || !item.Completed || !item.Uncertain || item.SortOrder != 3 || item.SectionID != snapshot.Sections[0].ID {
		t.Fatalf("item = %#v", item)
	}

	// Simulate a lost acknowledgement and server restart before retrying every
	// create, update, move and delete with the original temporary references.
	Close()
	Init()
	replayed, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Changed || !reflect.DeepEqual(response.Results, replayed.Results) || !reflect.DeepEqual(response.Snapshot, replayed.Snapshot) {
		t.Fatalf("retry changed committed state: %#v", replayed)
	}
	var historyCount, receipts int
	if err := DB.QueryRow("SELECT usage_count FROM item_history WHERE name = 'Milk'").Scan(&historyCount); err != nil {
		t.Fatal(err)
	}
	if err := DB.QueryRow("SELECT COUNT(*) FROM offline_operations").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if historyCount != 1 || receipts != len(operations) {
		t.Fatalf("retry duplicated side effects: history=%d, receipts=%d", historyCount, receipts)
	}
}

func TestOfflineConflictRollsBackMutationsReceiptsMappingsAndHooks(t *testing.T) {
	initOfflineDatabase(t)
	operations := offlineCreateTree()
	missingUpdate := offlineOperation("item", "update", 999999, map[string]interface{}{"completed": true})
	operations = append(operations, missingUpdate)
	hook := func(tx *sql.Tx, event OfflineItemEvent) error {
		_, err := tx.Exec("INSERT INTO webhook_outbox (event,payload) VALUES (?,?)", event.Event, []byte("test payload"))
		return err
	}
	_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: operations}, hook)
	requireOfflineError(t, err, true, missingUpdate.ID)
	for _, table := range []string{"lists", "sections", "items", "offline_operations", "offline_entity_ids", "item_history", "webhook_outbox"} {
		var count int
		if err := DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s contains %d rows after rejected batch", table, count)
		}
	}
}

func TestOfflineTemporaryIDsAreScopedPerClientAndEntity(t *testing.T) {
	initOfflineDatabase(t)
	firstClient, secondClient := uuid.NewString(), uuid.NewString()
	first, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: firstClient, Operations: offlineCreateTree()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	unknownParent := offlineOperation("section", "create", -2, map[string]interface{}{"name": "Wrong owner", "list_id": -1})
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: secondClient, Operations: []OfflineOperation{unknownParent}}, nil)
	requireOfflineError(t, err, true, unknownParent.ID)
	otherOperations := offlineCreateTree()
	otherOperations[0].Values["name"] = json.RawMessage(`"Other shopper"`)
	// Even the same operation UUID belongs to its own stable client ID.
	otherOperations[0].ID = first.Results[0].ID
	second, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: secondClient, Operations: otherOperations}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3; index++ {
		if first.Results[index].ServerID == second.Results[index].ServerID {
			t.Fatalf("client IDs collided for entity %s", first.Results[index].Entity)
		}
	}
	if len(second.Snapshot.Items) != 2 || len(second.Snapshot.Sections) != 2 || len(second.Snapshot.Lists) != 2 {
		t.Fatalf("snapshot omitted another client's list: %#v", second.Snapshot)
	}
}

func TestOfflineRejectsReusedOperationAndTemporaryIDs(t *testing.T) {
	initOfflineDatabase(t)
	request := OfflineSyncRequest{ClientID: uuid.NewString(), Operations: offlineCreateTree()}
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil); err != nil {
		t.Fatal(err)
	}
	changed := request.Operations[2]
	changed.Values = map[string]json.RawMessage{"name": json.RawMessage(`"Cheese"`), "section_id": json.RawMessage(`-1`)}
	_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: request.ClientID, Operations: []OfflineOperation{changed}}, nil)
	requireOfflineError(t, err, true, changed.ID)
	changed.ID = uuid.NewString()
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: request.ClientID, Operations: []OfflineOperation{changed}}, nil)
	requireOfflineError(t, err, true, changed.ID)
	state, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil || len(state.Items) != 1 || state.Items[0].Name != "Milk" {
		t.Fatalf("reused IDs changed state: %#v, %v", state, err)
	}
}

func TestOfflineSeparateBatchesExplicitCompletionAndIdempotentDelete(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: offlineCreateTree()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	itemID := created.Results[2].ServerID
	for _, completed := range []bool{true, false} {
		operation := offlineOperation("item", "update", -1, map[string]interface{}{"completed": completed})
		request := OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{operation}}
		for attempt := 0; attempt < 2; attempt++ {
			response, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil)
			if err != nil || response.Snapshot.Items[0].Completed != completed {
				t.Fatalf("completion retry = %#v, %v", response, err)
			}
			if attempt == 1 && response.Changed {
				t.Fatal("acknowledged operation applied twice")
			}
		}
	}
	for _, entity := range []string{"item", "section", "list"} {
		operation := offlineOperation(entity, "delete", -1, nil)
		request := OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{operation}}
		if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil); err != nil {
			t.Fatal(err)
		}
		// A fresh operation deleting the already missing row is safe too.
		request.Operations[0].ID = uuid.NewString()
		response, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil)
		if err != nil || response.Changed {
			t.Fatalf("duplicate deletion = %#v, %v", response, err)
		}
	}
	missing := offlineOperation("item", "update", itemID, map[string]interface{}{"completed": true})
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{missing}}, nil)
	requireOfflineError(t, err, true, missing.ID)
	snapshot, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	if string(encoded) != `{"lists":[],"sections":[],"items":[]}` {
		t.Fatalf("empty snapshot = %s", encoded)
	}
}

func TestOfflineValidationRejectsInvalidFieldsAndReferences(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: offlineCreateTree()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, entity, field string
		value               interface{}
		conflict            bool
	}{
		{"empty item name", "item", "name", "", false},
		{"blank name", "item", "name", "   ", false},
		{"long item name", "item", "name", strings.Repeat("x", 201), false},
		{"long list name", "list", "name", strings.Repeat("x", 101), false},
		{"reserved section name", "section", "name", "[HISTORY]", false},
		{"long description", "item", "description", strings.Repeat("x", 501), false},
		{"long icon", "list", "icon", strings.Repeat("x", 21), false},
		{"invalid sort mode", "section", "sort_mode", "random", false},
		{"negative quantity", "item", "quantity", -1, false},
		{"fractional quantity", "item", "quantity", 1.5, false},
		{"unsafe quantity", "item", "quantity", int64(9007199254740992), false},
		{"negative order", "list", "sort_order", -1, false},
		{"string boolean", "item", "completed", "true", false},
		{"null boolean", "item", "completed", nil, false},
		{"null quantity", "item", "quantity", nil, false},
		{"null description", "item", "description", nil, false},
		{"zero parent", "section", "list_id", 0, false},
		{"deleted destination list", "section", "list_id", 999999, true},
		{"deleted destination section", "item", "section_id", 999999, true},
		{"unknown temporary parent", "item", "section_id", -999, true},
		{"invalid active flag", "list", "is_active", "true", false},
		{"SQL field injection", "item", "name = ''; DELETE FROM items; --", "bad", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			index := map[string]int{"list": 0, "section": 1, "item": 2}[test.entity]
			operation := offlineOperation(test.entity, "update", created.Results[index].ServerID, map[string]interface{}{test.field: test.value})
			_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{operation}}, nil)
			requireOfflineError(t, err, test.conflict, operation.ID)
		})
	}
	after, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil || !reflect.DeepEqual(created.Snapshot, after) {
		t.Fatalf("invalid changes mutated state: %#v, %v", after, err)
	}
}

func TestOfflineBatchEnvelopeAndCreateValidation(t *testing.T) {
	initOfflineDatabase(t)
	invalid := []OfflineOperation{
		offlineOperation("list", "create", 1, map[string]interface{}{"name": "Name"}),
		offlineOperation("list", "create", 0, map[string]interface{}{"name": "Name"}),
		offlineOperation("list", "create", -9007199254740992, map[string]interface{}{"name": "Name"}),
		offlineOperation("list", "create", -1, nil),
		offlineOperation("section", "create", -1, map[string]interface{}{"name": "Name"}),
		offlineOperation("item", "create", -1, map[string]interface{}{"name": "Name"}),
		offlineOperation("list", "delete", 1, map[string]interface{}{"name": "Name"}),
		offlineOperation("session", "delete", 1, nil),
		offlineOperation("item", "toggle", 1, nil),
	}
	for _, operation := range invalid {
		_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: []OfflineOperation{operation}}, nil)
		requireOfflineError(t, err, false, operation.ID)
	}
	_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: "not-a-uuid"}, nil)
	requireOfflineError(t, err, false, "")
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: make([]OfflineOperation, MaxOfflineOperations+1)}, nil)
	requireOfflineError(t, err, false, "")
}

func TestOfflineConcurrentRetryCreatesOnlyOnce(t *testing.T) {
	initOfflineDatabase(t)
	request := OfflineSyncRequest{ClientID: uuid.NewString(), Operations: offlineCreateTree()}
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, nil); err != nil {
				t.Errorf("concurrent retry: %v", err)
			}
		}()
	}
	wait.Wait()
	state, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil || len(state.Lists) != 1 || len(state.Sections) != 1 || len(state.Items) != 1 {
		t.Fatalf("concurrent retry duplicated entities: %#v, %v", state, err)
	}
}

func TestOfflineWebhookHooksPreserveCascadeContextAndReplayOnce(t *testing.T) {
	initOfflineDatabase(t)
	var captured []OfflineItemEvent
	hook := func(_ *sql.Tx, event OfflineItemEvent) error { captured = append(captured, event); return nil }
	clientID := uuid.NewString()
	request := OfflineSyncRequest{ClientID: clientID, Operations: offlineCreateTree()}
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, hook); err != nil {
		t.Fatal(err)
	}
	complete := offlineOperation("item", "update", -1, map[string]interface{}{"completed": true})
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{complete}}, hook); err != nil {
		t.Fatal(err)
	}
	// Editing an already completed item emits updated, not completed again.
	edit := offlineOperation("item", "update", -1, map[string]interface{}{"completed": true, "description": "Organic"})
	deletion := offlineOperation("list", "delete", -1, nil)
	request.Operations = []OfflineOperation{edit, deletion}
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, hook); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, request, hook); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 4 {
		t.Fatalf("events = %#v", captured)
	}
	for index, want := range []string{"item.created", "item.completed", "item.updated", "item.deleted"} {
		if event := captured[index]; event.Event != want || event.Item.Name != "Milk" || event.Section.Name != "Dairy" || event.List.Name != "Weekly" {
			t.Fatalf("event %d = %#v", index, event)
		}
	}
}

func TestOfflineSectionMoveKeepsChildrenAndListNamesRemainUnique(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	operations := append(offlineCreateTree(), offlineOperation("list", "create", -2, map[string]interface{}{"name": "Destination"}))
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: operations}, nil)
	if err != nil {
		t.Fatal(err)
	}
	move := offlineOperation("section", "update", -1, map[string]interface{}{"list_id": -2})
	response, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{move}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Snapshot.Sections[0].ListID != created.Results[3].ServerID || response.Snapshot.Items[0].SectionID != created.Results[1].ServerID {
		t.Fatalf("moving a section lost its items: %#v", response.Snapshot)
	}
	duplicate := offlineOperation("list", "update", -2, map[string]interface{}{"name": "weekly"})
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{duplicate}}, nil)
	requireOfflineError(t, err, true, duplicate.ID)
}

func TestOfflineUnchangedPatchPreservesVersionAndDoesNotEmitWebhook(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: offlineCreateTree()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec("UPDATE items SET updated_at = 123 WHERE id = ?", created.Results[2].ServerID); err != nil {
		t.Fatal(err)
	}
	noOp := offlineOperation("item", "update", -1, map[string]interface{}{"completed": false, "name": "Milk", "quantity": 0})
	hook := func(_ *sql.Tx, _ OfflineItemEvent) error { t.Error("unchanged patch emitted webhook"); return nil }
	response, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{noOp}}, hook)
	if err != nil {
		t.Fatal(err)
	}
	if response.Changed || response.Snapshot.Items[0].UpdatedAt != 123 {
		t.Fatalf("unchanged patch changed version: %#v", response)
	}
}

func TestOfflineDuplicateOperationInOneBatchCreatesOnlyOnce(t *testing.T) {
	initOfflineDatabase(t)
	operation := offlineOperation("list", "create", -1, map[string]interface{}{"name": "Shopping"})
	response, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: []OfflineOperation{operation, operation}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Snapshot.Lists) != 1 || len(response.Results) != 2 || response.Results[0].ServerID != response.Results[1].ServerID {
		t.Fatalf("duplicate operation created twice: %#v", response)
	}
}

func TestOfflineListActivationIsExclusiveAndReplayPreservesLaterSelection(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	first := offlineOperation("list", "create", -1, map[string]interface{}{"name": "First", "is_active": true})
	second := offlineOperation("list", "create", -2, map[string]interface{}{"name": "Second", "is_active": true})
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{first, second}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertActive := func(snapshot OfflineSnapshot, expected int64) {
		t.Helper()
		var active []int64
		for _, list := range snapshot.Lists {
			if list.IsActive {
				active = append(active, list.ID)
			}
		}
		if len(active) != 1 || active[0] != expected {
			t.Fatalf("active lists = %v, want only %d", active, expected)
		}
	}
	assertActive(created.Snapshot, created.Results[1].ServerID)
	activation := offlineOperation("list", "update", -1, map[string]interface{}{"is_active": true})
	updated, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{activation}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertActive(updated.Snapshot, created.Results[0].ServerID)
	// Retrying an older create must not make that list active again.
	retried, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{second}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Changed {
		t.Fatal("retry changed active list")
	}
	assertActive(retried.Snapshot, created.Results[0].ServerID)
	missing := offlineOperation("item", "update", 999999, map[string]interface{}{"name": "Missing"})
	failedActivation := offlineOperation("list", "update", -2, map[string]interface{}{"is_active": true})
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{failedActivation, missing}}, nil)
	requireOfflineError(t, err, true, missing.ID)
	after, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertActive(after, created.Results[0].ServerID)
}

func TestOfflineSnapshotExcludesReservedHistoryContainersAndCounters(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	created, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: offlineCreateTree()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a legacy imported database that predates reserved-name checks.
	historyList, err := CreateList(DefaultWorkspaceID, "[HISTORY]", "cart")
	if err != nil {
		t.Fatal(err)
	}
	listSection, err := CreateSectionForList(historyList.ID, "Legacy suggestions")
	if err != nil {
		t.Fatal(err)
	}
	historySection, err := CreateSectionForList(created.Results[0].ServerID, "[HISTORY]")
	if err != nil {
		t.Fatal(err)
	}
	for _, sectionID := range []int64{listSection.ID, historySection.ID} {
		item, err := CreateItem(sectionID, "Hidden history", "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := SetItemCompleted(item.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	assertSnapshot := func(snapshot OfflineSnapshot) {
		t.Helper()
		if !reflect.DeepEqual(snapshot, created.Snapshot) {
			t.Fatalf("history data leaked into shopping snapshot or counters: %#v", snapshot)
		}
	}
	snapshot, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(snapshot)
	replayed, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(replayed.Snapshot)
	// Excluding a container from shopping does not delete any imported data.
	var itemCount int
	if err := DB.QueryRow("SELECT COUNT(*) FROM items").Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 3 {
		t.Fatalf("history filtering deleted records: count=%d", itemCount)
	}
}

func TestOfflineDefaultSortOrderCannotExceedJavaScriptSafeInteger(t *testing.T) {
	initOfflineDatabase(t)
	clientID := uuid.NewString()
	maximum := offlineOperation("list", "create", -1, map[string]interface{}{"name": "Maximum", "sort_order": maxOfflineInteger})
	if _, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{maximum}}, nil); err != nil {
		t.Fatal(err)
	}
	appendList := offlineOperation("list", "create", -2, map[string]interface{}{"name": "After maximum"})
	_, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: clientID, Operations: []OfflineOperation{appendList}}, nil)
	requireOfflineError(t, err, true, appendList.ID)
	snapshot, err := GetOfflineSnapshot(DefaultWorkspaceID)
	if err != nil || len(snapshot.Lists) != 1 {
		t.Fatalf("unsafe default order was saved: %#v, %v", snapshot, err)
	}
}

func TestOfflineMoveToAnotherListStaysInsideTheWorkspace(t *testing.T) {
	initOfflineDatabase(t)
	first, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: []OfflineOperation{
		offlineOperation("list", "create", -1, map[string]interface{}{"name": "Rewe"}),
		offlineOperation("section", "create", -1, map[string]interface{}{"name": "Dairy", "list_id": -1}),
		offlineOperation("list", "create", -2, map[string]interface{}{"name": "Lidl"}),
		offlineOperation("section", "create", -2, map[string]interface{}{"name": "Fridge", "list_id": -2}),
		offlineOperation("item", "create", -1, map[string]interface{}{"name": "Milk", "section_id": -1}),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lidlSection, milk int64
	for _, section := range first.Snapshot.Sections {
		if section.Name == "Fridge" {
			lidlSection = section.ID
		}
	}
	milk = first.Snapshot.Items[0].ID

	moved, err := ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: []OfflineOperation{
		offlineOperation("item", "update", milk, map[string]interface{}{"section_id": lidlSection, "sort_order": 0}),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := moved.Snapshot.Items[0]; got.ID != milk || got.SectionID != lidlSection {
		t.Fatalf("item was not moved to the other list: %#v", got)
	}
	for _, list := range moved.Snapshot.Lists {
		want := 0
		if list.Name == "Lidl" {
			want = 1
		}
		if list.Stats.TotalItems != want {
			t.Fatalf("list %q has %d items, want %d", list.Name, list.Stats.TotalItems, want)
		}
	}

	// A section of another workspace is not a valid destination.
	user, err := CreateUser("mover", "Mover", "correct horse battery", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := CreateWorkspace("other", "", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreignList, err := CreateList(other.ID, "Foreign", "")
	if err != nil {
		t.Fatal(err)
	}
	foreignSection, err := CreateSectionForList(foreignList.ID, "Foreign section")
	if err != nil {
		t.Fatal(err)
	}
	escape := offlineOperation("item", "update", milk, map[string]interface{}{"section_id": foreignSection.ID})
	_, err = ApplyOfflineSync(DefaultWorkspaceID, OfflineSyncRequest{ClientID: uuid.NewString(), Operations: []OfflineOperation{escape}}, nil)
	requireOfflineError(t, err, true, escape.ID)
	var sectionID int64
	if err := DB.QueryRow("SELECT section_id FROM items WHERE id = ?", milk).Scan(&sectionID); err != nil || sectionID != lidlSection {
		t.Fatalf("item left its workspace: section %d, err %v", sectionID, err)
	}
}
