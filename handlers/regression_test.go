package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"shopping-list/db"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
)

type concurrentWriteDetector struct {
	active     atomic.Int32
	concurrent atomic.Bool
	writes     atomic.Int32
	closes     atomic.Int32
}

func (writer *concurrentWriteDetector) beginWrite() {
	if writer.active.Add(1) > 1 {
		writer.concurrent.Store(true)
	}
	time.Sleep(250 * time.Microsecond)
	writer.active.Add(-1)
}

func (writer *concurrentWriteDetector) WriteJSON(interface{}) error {
	writer.beginWrite()
	return nil
}

func (writer *concurrentWriteDetector) WriteMessage(int, []byte) error {
	writer.writes.Add(1)
	writer.beginWrite()
	return nil
}

func (writer *concurrentWriteDetector) Close() error {
	writer.closes.Add(1)
	return nil
}

func initTestDatabase(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "test.db"))
	db.Init()
	t.Cleanup(db.Close)
}

func postImportFile(t *testing.T, app *fiber.App, filename, contents string) *http.Response {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := io.WriteString(part, contents); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.WriteField("conflict_resolution", "skip"); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := app.Test(req, 2000)
	if err != nil {
		t.Fatalf("import request did not complete: %v", err)
	}
	return resp
}

func TestImportDataDoesNotDeadlockSingleConnectionPool(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", admin)
		return c.Next()
	})
	app.Post("/import", ImportData)

	jsonImport := `{
		"version":"1.0",
		"app":"koffan",
		"data":{
			"lists":[{
				"name":"Weekly",
				"icon":"shopping-cart",
				"sections":[{"name":"General","items":[{"name":"Milk"}]}]
			}],
			"templates":[{
				"name":"Basics",
				"items":[{"section_name":"General","name":"Bread"}]
			}]
		}
	}`
	resp := postImportFile(t, app, "review.json", jsonImport)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("JSON import status = %d, body = %s", resp.StatusCode, body)
	}

	csvImport := "list_name,list_icon,section_name,item_name,item_description,item_completed,item_uncertain,item_quantity\n" +
		"Weekend,shopping-cart,General,Eggs,,false,false,1\n"
	resp = postImportFile(t, app, "review.csv", csvImport)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("CSV import status = %d, body = %s", resp.StatusCode, body)
	}

	lists, err := db.GetAllLists()
	if err != nil {
		t.Fatalf("read lists after import: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("imported lists = %d, want 2", len(lists))
	}

	templates, err := db.GetAllTemplates()
	if err != nil {
		t.Fatalf("read templates after import: %v", err)
	}
	if len(templates) != 1 || len(templates[0].Items) != 1 {
		t.Fatalf("imported templates = %#v, want one template with one item", templates)
	}
	if templates[0].OwnerUserID != admin.ID {
		t.Fatalf("imported template owner=%d, want %d", templates[0].OwnerUserID, admin.ID)
	}
	for _, list := range lists {
		if list.OwnerUserID != admin.ID {
			t.Fatalf("imported list %q owner=%d, want %d", list.Name, list.OwnerUserID, admin.ID)
		}
		sections, sectionErr := db.GetSectionsByList(list.ID)
		if sectionErr != nil {
			t.Fatal(sectionErr)
		}
		for _, section := range sections {
			for _, item := range section.Items {
				if item.CreatedByUserID == nil || *item.CreatedByUserID != admin.ID || item.UpdatedByUserID == nil || *item.UpdatedByUserID != admin.ID {
					t.Fatalf("imported item attribution=%#v, want admin", item)
				}
			}
		}
	}
}

func TestOfflineDataAndExportAreScopedToAccessibleLists(t *testing.T) {
	initTestDatabase(t)
	owner, err := db.CreateLocalUser("scope-owner", "Scope Owner", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateLocalUser("scope-other", "Scope Other", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := db.CreateListForUser(owner.ID, "Owned visible", "")
	if err != nil {
		t.Fatal(err)
	}
	ownedSection, err := db.CreateSectionForList(owned.ID, "Owned section")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateItem(ownedSection.ID, "Owned item", "", 0); err != nil {
		t.Fatal(err)
	}
	private, err := db.CreateListForUser(other.ID, "Other private", "")
	if err != nil {
		t.Fatal(err)
	}
	privateSection, err := db.CreateSectionForList(private.ID, "Private section")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateItem(privateSection.ID, "Private item", "", 0); err != nil {
		t.Fatal(err)
	}
	if err = db.SetActiveListForUser(owner.ID, owned.ID); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", owner); return c.Next() })
	app.Get("/api/data", GetAllData)
	app.Get("/export", ExportAllData)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/data", nil))
	if err != nil {
		t.Fatal(err)
	}
	offlineBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(offlineBody, []byte("Owned item")) || bytes.Contains(offlineBody, []byte("Private item")) {
		t.Fatalf("scoped offline response status=%d body=%s", resp.StatusCode, offlineBody)
	}

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/export?include_templates=false&include_history=false", nil))
	if err != nil {
		t.Fatal(err)
	}
	exportBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(exportBody, []byte("Owned visible")) || bytes.Contains(exportBody, []byte("Other private")) {
		t.Fatalf("scoped export status=%d body=%s", resp.StatusCode, exportBody)
	}
}

func TestImportConflictsAreScopedToImporterOwnership(t *testing.T) {
	initTestDatabase(t)
	importer, err := db.CreateLocalUser("importer", "Importer", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateLocalUser("import-other", "Import Other", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	otherList, err := db.CreateListForUser(other.ID, "Same name", "")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", importer); return c.Next() })
	app.Post("/import", ImportData)
	contents := `{"version":"1.0","app":"koffan","data":{"lists":[{"name":"Same name","sections":[{"name":"General","items":[{"name":"Imported item"}]}]}]}}`
	resp := postImportFile(t, app, "scoped.json", contents)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("import status=%d body=%s", resp.StatusCode, body)
	}
	owned, err := db.GetListsOwnedByUser(importer.ID)
	if err != nil || len(owned) != 1 || owned[0].Name != "Same name" {
		t.Fatalf("importer lists=%#v err=%v", owned, err)
	}
	if preserved, err := db.GetListByID(otherList.ID); err != nil || preserved.OwnerUserID != other.ID {
		t.Fatalf("other user's colliding list changed: %#v err=%v", preserved, err)
	}
	sections, err := db.GetSectionsByList(owned[0].ID)
	if err != nil || len(sections) != 1 || len(sections[0].Items) != 1 {
		t.Fatalf("imported sections=%#v err=%v", sections, err)
	}
	item := sections[0].Items[0]
	if item.CreatedByUserID == nil || *item.CreatedByUserID != importer.ID {
		t.Fatalf("imported item creator=%#v, want importer", item.CreatedByUserID)
	}
}

func TestTemplatesArePrivateToOwnerWithAdminOverride(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.CreateLocalUser("template-owner", "Template Owner", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateLocalUser("template-other", "Template Other", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	template, err := db.CreateTemplateForUser(owner.ID, "Private template", "")
	if err != nil {
		t.Fatal(err)
	}
	ownerTemplates, err := db.GetTemplatesForUser(owner.ID, false)
	if err != nil || len(ownerTemplates) != 1 || ownerTemplates[0].ID != template.ID {
		t.Fatalf("owner templates=%#v err=%v", ownerTemplates, err)
	}
	otherTemplates, err := db.GetTemplatesForUser(other.ID, false)
	if err != nil || len(otherTemplates) != 0 {
		t.Fatalf("other user saw templates=%#v err=%v", otherTemplates, err)
	}
	allowed, err := db.CanManageTemplate(other.ID, template.ID, false)
	if err != nil || allowed {
		t.Fatalf("other user manage=%v err=%v", allowed, err)
	}
	allowed, err = db.CanManageTemplate(admin.ID, template.ID, true)
	if err != nil || !allowed {
		t.Fatalf("admin manage=%v err=%v", allowed, err)
	}
	users := map[string]*db.User{"owner": owner, "other": other, "admin": admin}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", users[c.Get("X-Test-User")])
		return c.Next()
	})
	app.Get("/templates/:id", GetTemplate)
	path := "/templates/" + strconv.FormatInt(template.ID, 10) + "?format=json"
	request := func(identity string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-User", identity)
		resp, requestErr := app.Test(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if status := request("other"); status != http.StatusNotFound {
		t.Fatalf("other template status=%d, want 404", status)
	}
	if status := request("owner"); status != http.StatusOK {
		t.Fatalf("owner template status=%d, want 200", status)
	}
	if status := request("admin"); status != http.StatusOK {
		t.Fatalf("admin template status=%d, want 200", status)
	}
}

func TestAuthMiddlewareHandlesShortSessionID(t *testing.T) {
	initTestDatabase(t)
	app := fiber.New()
	app.Use(recover.New())
	app.Use(AuthMiddleware)
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "x"})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s; want redirect", resp.StatusCode, body)
	}
	if location := resp.Header.Get("Location"); location != "/login" {
		t.Fatalf("Location = %q, want /login", location)
	}
}

func TestGroupListPermissionAndAdminOverride(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	member, err := db.CreateLocalUser("member", "Member", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateListForUser(admin.ID, "Shared", "")
	if err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateGroup("Household", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddUserToGroup(member.ID, group.ID, "local"); err != nil {
		t.Fatal(err)
	}
	if err = db.SetGroupListAccess(list.ID, group.ID, admin.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	permission, err := db.GetUserListPermission(member.ID, list.ID, false)
	if err != nil || permission != db.ListPermissionEdit {
		t.Fatalf("group permission = %v, %v", permission, err)
	}
	if err = db.SetGroupListAccess(list.ID, group.ID, admin.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	permission, err = db.GetUserListPermission(member.ID, list.ID, false)
	if err != nil || permission != db.ListPermissionManage {
		t.Fatalf("group full-control permission = %v, %v", permission, err)
	}
	canManage, err := db.CanManageList(member.ID, list.ID, false)
	if err != nil || !canManage {
		t.Fatalf("group full control cannot manage list: canManage=%v err=%v", canManage, err)
	}
	permission, err = db.GetUserListPermission(admin.ID, list.ID, true)
	if err != nil || permission != db.ListPermissionOwner {
		t.Fatalf("admin permission = %v, %v", permission, err)
	}
}

func TestSameListNameIsPrivatePerOwnerAndDisambiguatedWhenShared(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.CreateLocalUser("user1", "User One", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	adminList, err := db.CreateListForUser(admin.ID, "xyz", "")
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := db.ListNameExistsForOwner("xyz", user.ID, 0); err != nil || exists {
		t.Fatalf("another owner's private name leaked: exists=%v err=%v", exists, err)
	}
	userList, err := db.CreateListForUser(user.ID, "xyz", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateListForUser(user.ID, "XYZ", ""); err == nil {
		t.Fatal("same owner should not create a case-insensitive duplicate")
	}
	lists, err := db.GetListsForUser(user.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].ID != userList.ID || lists[0].AccessRole != "owner" {
		t.Fatalf("unexpected private lists: %#v", lists)
	}
	if err = db.SetListMember(adminList.ID, user.ID, admin.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	lists, err = db.GetListsForUser(user.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 {
		t.Fatalf("visible lists=%d, want 2", len(lists))
	}
	roles := map[int64]string{}
	owners := map[int64]string{}
	for _, list := range lists {
		roles[list.ID] = list.AccessRole
		owners[list.ID] = list.OwnerDisplayName
	}
	if roles[userList.ID] != "owner" || roles[adminList.ID] != "editor" || owners[adminList.ID] != admin.DisplayName {
		t.Fatalf("lists not disambiguated: roles=%v owners=%v", roles, owners)
	}
}

func TestOIDCGroupSyncOverridesOnlyOIDCManagedMemberships(t *testing.T) {
	initTestDatabase(t)
	user, err := db.UpsertOIDCUser("https://issuer.example", "subject-1", "oidc-user", "OIDC User", nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	keep, err := db.CreateGroup("Keep", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	remove, err := db.CreateGroup("Remove", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	local, err := db.CreateGroup("Local", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AddUserToGroup(user.ID, local.ID, "local"); err == nil {
		t.Fatal("manual OIDC user membership should be rejected")
	}
	// Simulate memberships created by an older build before IdP ownership was enforced.
	if _, err = db.DB.Exec(`INSERT INTO user_groups(user_id,group_id,source) VALUES(?,?,'local'),(?,?,'local')`, user.ID, remove.ID, user.ID, local.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.SyncOIDCGroups(user.ID, []string{"Keep"}, false); err != nil {
		t.Fatal(err)
	}
	keepUsers, _ := db.GetGroupUsers(keep.ID)
	removeUsers, _ := db.GetGroupUsers(remove.ID)
	localUsers, _ := db.GetGroupUsers(local.ID)
	if len(keepUsers) != 1 || len(removeUsers) != 0 || len(localUsers) != 0 {
		t.Fatalf("unexpected memberships: keep=%d remove=%d local=%d", len(keepUsers), len(removeUsers), len(localUsers))
	}
	if err = db.SyncOIDCGroups(user.ID, []string{"Auto-created"}, true); err != nil {
		t.Fatal(err)
	}
	groups, err := db.ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		if g.Name == "Auto-created" {
			found = true
		}
	}
	if !found {
		t.Fatal("OIDC group was not auto-created")
	}
}

func TestOIDCAutoCreateGroupsIsConfigurationOnly(t *testing.T) {
	t.Setenv("OIDC_AUTO_CREATE_GROUPS", "")
	enabled, err := db.OIDCAutoCreateGroups()
	if err != nil || enabled {
		t.Fatalf("empty config=%v err=%v, want false", enabled, err)
	}

	t.Setenv("OIDC_AUTO_CREATE_GROUPS", "true")
	enabled, err = db.OIDCAutoCreateGroups()
	if err != nil || !enabled {
		t.Fatalf("true config=%v err=%v, want true", enabled, err)
	}

	t.Setenv("OIDC_AUTO_CREATE_GROUPS", "invalid")
	if _, err = db.OIDCAutoCreateGroups(); err == nil {
		t.Fatal("invalid OIDC_AUTO_CREATE_GROUPS should return an error")
	}
}

func TestRenamingGroupPreservesGrantsAndChangesOIDCMatch(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.UpsertOIDCUser("https://issuer.example", "rename-subject", "rename-user", "Rename User", nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateListForUser(admin.ID, "Renamed group list", "")
	if err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateGroup("Old name", "Old description", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SetGroupListAccess(list.ID, group.ID, admin.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	if err = db.SyncOIDCGroups(user.ID, []string{"Old name"}, false); err != nil {
		t.Fatal(err)
	}

	updated, err := db.UpdateGroup(group.ID, "New name", "New description")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != group.ID || updated.Name != "New name" || updated.Description != "New description" {
		t.Fatalf("unexpected updated group: %#v", updated)
	}
	permission, err := db.GetUserListPermission(user.ID, list.ID, false)
	if err != nil || permission != db.ListPermissionEdit {
		t.Fatalf("permission after rename=%v err=%v, want edit", permission, err)
	}

	if err = db.SyncOIDCGroups(user.ID, []string{"Old name"}, false); err != nil {
		t.Fatal(err)
	}
	permission, err = db.GetUserListPermission(user.ID, list.ID, false)
	if err != nil || permission != db.ListPermissionNone {
		t.Fatalf("old OIDC name permission=%v err=%v, want none", permission, err)
	}
	if err = db.SyncOIDCGroups(user.ID, []string{"New name"}, false); err != nil {
		t.Fatal(err)
	}
	permission, err = db.GetUserListPermission(user.ID, list.ID, false)
	if err != nil || permission != db.ListPermissionEdit {
		t.Fatalf("new OIDC name permission=%v err=%v, want edit", permission, err)
	}
}

func TestGroupMembershipGrantsEffectiveAdministratorPermission(t *testing.T) {
	initTestDatabase(t)
	localUser, err := db.CreateLocalUser("group-admin", "Group Admin", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateGroupWithAdmin("Administrators", "", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := db.IsUserAdministrator(localUser.ID)
	if err != nil || admin {
		t.Fatalf("unexpected initial admin=%v err=%v", admin, err)
	}
	if err = db.AddUserToGroup(localUser.ID, group.ID, "local"); err != nil {
		t.Fatal(err)
	}
	admin, err = db.IsUserAdministrator(localUser.ID)
	if err != nil || !admin {
		t.Fatalf("group did not grant admin: admin=%v err=%v", admin, err)
	}
	if err = db.RemoveUserFromGroup(localUser.ID, group.ID); err != nil {
		t.Fatal(err)
	}
	admin, err = db.IsUserAdministrator(localUser.ID)
	if err != nil || admin {
		t.Fatalf("removed group still grants admin: admin=%v err=%v", admin, err)
	}
	oidcUser, err := db.UpsertOIDCUser("https://issuer.example", "admin-sub", "oidc-admin", "OIDC Admin", nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SyncOIDCGroups(oidcUser.ID, []string{"Administrators"}, false); err != nil {
		t.Fatal(err)
	}
	admin, err = db.IsUserAdministrator(oidcUser.ID)
	if err != nil || !admin {
		t.Fatalf("OIDC group did not grant admin: admin=%v err=%v", admin, err)
	}
	oidcUser, err = db.UpsertOIDCUser("https://issuer.example", "admin-sub", "oidc-admin", "OIDC Admin", nil, true, true)
	if err != nil || !oidcUser.IsAdmin {
		t.Fatalf("OIDC_ADMIN_GROUP grant was not applied: user=%#v err=%v", oidcUser, err)
	}
	oidcUser, err = db.UpsertOIDCUser("https://issuer.example", "admin-sub", "oidc-admin", "OIDC Admin", nil, true, false)
	if err != nil || oidcUser.IsAdmin {
		t.Fatalf("OIDC_ADMIN_GROUP removal was not applied: user=%#v err=%v", oidcUser, err)
	}
}

func TestDisabledOwnerKeepsSharedListsButDeletingOwnerCascadesThem(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.CreateLocalUser("userA", "User A", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	member, err := db.CreateLocalUser("userB", "User B", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateListForUser(owner.ID, "xyc", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SetListMember(list.ID, member.ID, admin.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.UpdateUserByAdmin(admin.ID, owner.ID, owner.Username, owner.DisplayName, false, true, ""); err != nil {
		t.Fatal(err)
	}
	canEdit, err := db.CanEditList(member.ID, list.ID, false)
	if err != nil || !canEdit {
		t.Fatalf("disabled owner broke sharing: canEdit=%v err=%v", canEdit, err)
	}
	lists, err := db.GetListsForUser(member.ID, false)
	if err != nil || len(lists) != 1 || lists[0].ID != list.ID {
		t.Fatalf("shared list disappeared after disable: %#v err=%v", lists, err)
	}
	deleted, err := db.DeleteUserPermanently(admin.ID, owner.ID)
	if err != nil || deleted != 1 {
		t.Fatalf("delete owner: lists=%d err=%v", deleted, err)
	}
	if _, err = db.GetListByID(list.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("owned list survived user deletion: %v", err)
	}
	if _, err = db.GetUserByID(owner.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("user survived deletion: %v", err)
	}
	oidcOwner, err := db.UpsertOIDCUser("https://issuer.example", "departed-subject", "departed", "Departed User", nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	oidcList, err := db.CreateListForUser(oidcOwner.ID, "OIDC list", "")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err = db.DeleteUserPermanently(admin.ID, oidcOwner.ID)
	if err != nil || deleted != 1 {
		t.Fatalf("delete OIDC owner: lists=%d err=%v", deleted, err)
	}
	if _, err = db.GetListByID(oidcList.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("OIDC-owned list survived deletion: %v", err)
	}
	if _, err = db.GetUserByID(oidcOwner.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("OIDC user survived deletion: %v", err)
	}
}

func TestClearUserOwnedDataHasSameScopeForAdminAndNormalUser(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	normal, err := db.CreateLocalUser("clear-normal", "Clear Normal", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	adminList, err := db.CreateListForUser(admin.ID, "Admin owned", "")
	if err != nil {
		t.Fatal(err)
	}
	normalList, err := db.CreateListForUser(normal.ID, "Normal owned", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SetListMember(normalList.ID, admin.ID, normal.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	adminTemplate, err := db.CreateTemplateForUser(admin.ID, "Admin template", "")
	if err != nil {
		t.Fatal(err)
	}
	normalTemplate, err := db.CreateTemplateForUser(normal.ID, "Normal template", "")
	if err != nil {
		t.Fatal(err)
	}

	deletedLists, deletedTemplates, err := db.ClearUserOwnedData(admin.ID)
	if err != nil || deletedLists != 1 || deletedTemplates != 1 {
		t.Fatalf("admin clear lists=%d templates=%d err=%v", deletedLists, deletedTemplates, err)
	}
	if _, err = db.GetListByID(adminList.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("admin-owned list survived: %v", err)
	}
	if _, err = db.GetTemplateByID(adminTemplate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("admin-owned template survived: %v", err)
	}
	if _, err = db.GetListByID(normalList.ID); err != nil {
		t.Fatalf("normal user's list was deleted by admin clear: %v", err)
	}
	if _, err = db.GetTemplateByID(normalTemplate.ID); err != nil {
		t.Fatalf("normal user's template was deleted by admin clear: %v", err)
	}
	if _, err = db.GetUserByID(normal.ID); err != nil {
		t.Fatalf("normal user was deleted: %v", err)
	}

	deletedLists, deletedTemplates, err = db.ClearUserOwnedData(normal.ID)
	if err != nil || deletedLists != 1 || deletedTemplates != 1 {
		t.Fatalf("normal clear lists=%d templates=%d err=%v", deletedLists, deletedTemplates, err)
	}
	if _, err = db.GetListByID(normalList.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("normal-owned list survived: %v", err)
	}
	if _, err = db.GetTemplateByID(normalTemplate.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("normal-owned template survived: %v", err)
	}
	if _, err = db.GetUserByID(admin.ID); err != nil {
		t.Fatalf("admin user was deleted: %v", err)
	}
}

func TestItemUIHandlersEnforceLengthLimits(t *testing.T) {
	app := fiber.New()
	app.Post("/items", CreateItem)
	app.Put("/items/:id", UpdateItem)

	tests := []struct {
		name   string
		method string
		path   string
		form   url.Values
	}{
		{
			name:   "create rejects long name",
			method: http.MethodPost,
			path:   "/items",
			form: url.Values{
				"section_id": {"1"},
				"name":       {strings.Repeat("a", MaxItemNameLength+1)},
			},
		},
		{
			name:   "update rejects long description",
			method: http.MethodPut,
			path:   "/items/1",
			form: url.Values{
				"name":        {"Milk"},
				"description": {strings.Repeat("a", MaxDescriptionLength+1)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, body = %s; want 400", resp.StatusCode, body)
			}
		})
	}
}

func TestItemCreatorAndLastModifierAttribution(t *testing.T) {
	initTestDatabase(t)
	creator, err := db.CreateLocalUser("creator", "User C", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	modifier, err := db.CreateLocalUser("modifier", "User E", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateListForUser(admin.ID, "Shared shopping", "")
	if err != nil {
		t.Fatal(err)
	}
	section, err := db.CreateSectionForList(list.ID, "Pantry")
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.CreateItem(section.ID, "Strawberry Jam", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SetItemCreatedBy(item.ID, creator.ID); err != nil {
		t.Fatal(err)
	}
	if err = db.SetItemUpdatedBy(item.ID, modifier.ID); err != nil {
		t.Fatal(err)
	}

	item, err = db.GetItemByID(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.CreatedByName != "User C" || item.UpdatedByName != "User E" {
		t.Fatalf("attribution=%q/%q, want User C/User E", item.CreatedByName, item.UpdatedByName)
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte(`"created_by":{"id":`)) || !bytes.Contains(payload, []byte(`"display_name":"User C"`)) || !bytes.Contains(payload, []byte(`"updated_by":{"id":`)) {
		t.Fatalf("structured REST attribution missing: %s", payload)
	}
	for _, legacyField := range [][]byte{[]byte("created_by_user_id"), []byte("updated_by_user_id"), []byte("created_by_name"), []byte("updated_by_name")} {
		if bytes.Contains(payload, legacyField) {
			t.Fatalf("legacy attribution field %q leaked in REST JSON: %s", legacyField, payload)
		}
	}
	sections, err := db.GetSectionsByList(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 || len(sections[0].Items) != 1 || sections[0].Items[0].CreatedByName != "User C" || sections[0].Items[0].UpdatedByName != "User E" {
		t.Fatalf("list item attribution was not loaded: %#v", sections)
	}
}

func TestOnlyOwnerOrAdminCanShareListWithUsersAndGroups(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.CreateLocalUser("share-owner", "Share Owner", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	editor, err := db.CreateLocalUser("share-editor", "Share Editor", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := db.CreateLocalUser("share-manager", "Share Manager", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := db.CreateLocalUser("share-recipient", "Share Recipient", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.CreateListForUser(owner.ID, "Owner sharing", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SetListMember(list.ID, editor.ID, owner.ID, "editor"); err != nil {
		t.Fatal(err)
	}
	if err = db.SetListMember(list.ID, manager.ID, owner.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateGroup("Sharing group", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	users := map[string]*db.User{"owner": owner, "editor": editor, "manager": manager, "admin": admin}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if user := users[c.Get("X-Test-User")]; user != nil {
			c.Locals("user", user)
		}
		return c.Next()
	})
	app.Post("/lists/:id/members", RequireListManage, AddListMember)
	app.Put("/lists/:id/groups/:groupId", RequireListManage, SetListGroupAccess)
	app.Put("/lists/:id", RequireListOwner, func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	request := func(method, path, identity, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", identity)
		resp, requestErr := app.Test(req)
		if requestErr != nil {
			t.Fatalf("%s %s: %v", method, path, requestErr)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	memberPath := "/lists/" + strconv.FormatInt(list.ID, 10) + "/members"
	if status := request(http.MethodPost, memberPath, "owner", `{"user_id":`+strconv.FormatInt(recipient.ID, 10)+`,"role":"viewer"}`); status != http.StatusNoContent {
		t.Fatalf("owner user share status=%d, want 204", status)
	}
	groupPath := "/lists/" + strconv.FormatInt(list.ID, 10) + "/groups/" + strconv.FormatInt(group.ID, 10)
	if status := request(http.MethodPut, groupPath, "owner", `{"role":"editor"}`); status != http.StatusNoContent {
		t.Fatalf("owner group share status=%d, want 204", status)
	}
	if status := request(http.MethodPost, memberPath, "editor", `{"user_id":`+strconv.FormatInt(admin.ID, 10)+`,"role":"viewer"}`); status != http.StatusForbidden {
		t.Fatalf("editor share status=%d, want 403", status)
	}
	if status := request(http.MethodPut, groupPath, "editor", `{"role":"viewer"}`); status != http.StatusForbidden {
		t.Fatalf("editor group share status=%d, want 403", status)
	}
	if status := request(http.MethodPut, groupPath, "manager", `{"role":"manager"}`); status != http.StatusNoContent {
		t.Fatalf("full-control group share status=%d, want 204", status)
	}
	if status := request(http.MethodPut, "/lists/"+strconv.FormatInt(list.ID, 10), "manager", `{}`); status != http.StatusForbidden {
		t.Fatalf("full-control owner-only operation status=%d, want 403", status)
	}
	if status := request(http.MethodPut, "/lists/"+strconv.FormatInt(list.ID, 10), "admin", `{}`); status != http.StatusNoContent {
		t.Fatalf("admin owner-only operation status=%d, want 204", status)
	}
	if status := request(http.MethodPut, groupPath, "admin", `{"role":"viewer"}`); status != http.StatusNoContent {
		t.Fatalf("admin group share status=%d, want 204", status)
	}
}

func TestTransferListOwnershipResolvesNameConflict(t *testing.T) {
	initTestDatabase(t)
	admin, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := db.CreateLocalUser("old-owner", "User A", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	newOwner, err := db.CreateLocalUser("new-owner", "User B", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	transferred, err := db.CreateListForUser(owner.ID, "ABC", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateListForUser(newOwner.ID, "ABC", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateListForUser(newOwner.ID, "ABC (from User A)", ""); err != nil {
		t.Fatal(err)
	}

	transferred, err = db.TransferListOwnership(transferred.ID, newOwner.ID, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.OwnerUserID != newOwner.ID || transferred.Name != "ABC (from User A 2)" {
		t.Fatalf("transferred list=%#v", transferred)
	}
	permission, err := db.GetUserListPermission(owner.ID, transferred.ID, false)
	if err != nil || permission != db.ListPermissionManage {
		t.Fatalf("previous owner permission=%v err=%v, want manager", permission, err)
	}
	permission, err = db.GetUserListPermission(newOwner.ID, transferred.ID, false)
	if err != nil || permission != db.ListPermissionOwner {
		t.Fatalf("new owner permission=%v err=%v, want owner", permission, err)
	}
}

func TestConcurrentWebSocketBroadcastsUseSingleWriter(t *testing.T) {
	detector := &concurrentWriteDetector{}
	client := &webSocketClient{conn: detector}

	clientsMu.Lock()
	originalClients := clients
	clients = map[*websocket.Conn]*webSocketClient{
		nil: client,
	}
	clientsMu.Unlock()
	t.Cleanup(func() {
		clientsMu.Lock()
		clients = originalClients
		clientsMu.Unlock()
	})

	var waitGroup sync.WaitGroup
	for i := 0; i < 20; i++ {
		waitGroup.Add(2)
		go func(index int) {
			defer waitGroup.Done()
			BroadcastUpdate("test", map[string]int{"index": index})
		}(i)
		go func() {
			defer waitGroup.Done()
			_ = client.writeJSON(map[string]string{"type": "pong"})
		}()
	}
	waitGroup.Wait()

	if detector.concurrent.Load() {
		t.Fatal("websocket writes overlapped")
	}
}

func TestItemHistoryIsPrivatePerUser(t *testing.T) {
	initTestDatabase(t)
	userA, err := db.CreateLocalUser("history-a", "History A", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	userB, err := db.CreateLocalUser("history-b", "History B", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SaveItemHistoryForUser(userA.ID, "Private product", 0); err != nil {
		t.Fatal(err)
	}
	results, err := db.GetItemSuggestionsForUser(userB.ID, "Private", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("another user's private history leaked: %#v", results)
	}
	results, err = db.GetItemSuggestionsForUser(userA.ID, "Private", 10)
	if err != nil || len(results) != 1 || results[0].Name != "Private product" {
		t.Fatalf("owner suggestions=%#v err=%v", results, err)
	}
}

func TestListOrderingIsPrivatePerUser(t *testing.T) {
	initTestDatabase(t)
	userA, _ := db.CreateLocalUser("order-a", "Order A", "password123", false)
	userB, _ := db.CreateLocalUser("order-b", "Order B", "password123", false)
	first, _ := db.CreateListForUser(userA.ID, "First", "")
	second, _ := db.CreateListForUser(userA.ID, "Second", "")
	if err := db.SetListMember(first.ID, userB.ID, userA.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetListMember(second.ID, userB.ID, userA.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if err := db.MoveListForUser(userA.ID, false, first.ID, 1); err != nil {
		t.Fatal(err)
	}
	listsA, _ := db.GetListsForUser(userA.ID, false)
	listsB, _ := db.GetListsForUser(userB.ID, false)
	if len(listsA) != 2 || listsA[0].ID != second.ID || len(listsB) != 2 || listsB[0].ID != first.ID {
		t.Fatalf("orders are not independent: A=%#v B=%#v", listsA, listsB)
	}
}

func TestWebSocketListFilteringAndUserDisconnect(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("ws-owner", "WS Owner", "password123", false)
	outsider, _ := db.CreateLocalUser("ws-outsider", "WS Outsider", "password123", false)
	list, _ := db.CreateListForUser(owner.ID, "Private", "")
	ownerWriter := &concurrentWriteDetector{}
	outsiderWriter := &concurrentWriteDetector{}
	ownerConn, outsiderConn := &websocket.Conn{}, &websocket.Conn{}

	clientsMu.Lock()
	originalClients := clients
	clients = map[*websocket.Conn]*webSocketClient{
		ownerConn:    {conn: ownerWriter, userID: owner.ID},
		outsiderConn: {conn: outsiderWriter, userID: outsider.ID},
	}
	clientsMu.Unlock()
	t.Cleanup(func() {
		clientsMu.Lock()
		clients = originalClients
		clientsMu.Unlock()
	})

	BroadcastListUpdate(list.ID, "private_event", map[string]int64{"list_id": list.ID})
	if ownerWriter.writes.Load() != 1 || outsiderWriter.writes.Load() != 0 {
		t.Fatalf("writes owner=%d outsider=%d", ownerWriter.writes.Load(), outsiderWriter.writes.Load())
	}
	DisconnectUserWebSockets(owner.ID)
	if ownerWriter.closes.Load() != 1 || outsiderWriter.closes.Load() != 0 {
		t.Fatalf("closes owner=%d outsider=%d", ownerWriter.closes.Load(), outsiderWriter.closes.Load())
	}
}

func TestListActivityRetainsActorAndDeletedItemDetails(t *testing.T) {
	initTestDatabase(t)
	actor, _ := db.CreateLocalUser("activity-user", "Activity User", "password123", false)
	list, _ := db.CreateListForUser(actor.ID, "Activity list", "")
	section, _ := db.CreateSectionForList(list.ID, "Pantry")
	item, _ := db.CreateItem(section.ID, "Strawberry Jam", "", 1)
	if err := db.LogListActivity(list.ID, item.ID, actor.ID, "item_created", map[string]interface{}{"name": item.Name}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteItem(item.ID); err != nil {
		t.Fatal(err)
	}
	activity, err := db.GetListActivity(list.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 1 || activity[0].ActorName != "Activity User" || activity[0].ItemID != nil || !bytes.Contains(activity[0].MetadataJSON, []byte("Strawberry Jam")) {
		t.Fatalf("activity did not retain audit details: %#v", activity)
	}
}

func TestActivityAndOfflineVersionEndpointsEnforceViewPermission(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("audit-owner", "Audit Owner", "password123", false)
	outsider, _ := db.CreateLocalUser("audit-outsider", "Audit Outsider", "password123", false)
	list, _ := db.CreateListForUser(owner.ID, "Private audit", "")
	section, _ := db.CreateSectionForList(list.ID, "Private section")
	item, _ := db.CreateItem(section.ID, "Private item", "", 1)
	_ = db.LogListActivity(list.ID, item.ID, owner.ID, "item_created", map[string]string{"name": item.Name})

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", outsider); return c.Next() })
	app.Get("/lists/:id/activity", RequireListView, GetListActivity)
	app.Get("/api/item/:id/version", RequireItemView, GetItemVersion)
	for _, path := range []string{"/lists/" + strconv.FormatInt(list.ID, 10) + "/activity", "/api/item/" + strconv.FormatInt(item.ID, 10) + "/version"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("outsider status for %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestItemMoveCannotCrossListBoundary(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("move-owner", "Move Owner", "password123", false)
	firstList, _ := db.CreateListForUser(owner.ID, "First move list", "")
	secondList, _ := db.CreateListForUser(owner.ID, "Second move list", "")
	fromSection, _ := db.CreateSectionForList(firstList.ID, "From")
	toSection, _ := db.CreateSectionForList(firstList.ID, "To")
	otherListSection, _ := db.CreateSectionForList(secondList.ID, "Private target")
	item, _ := db.CreateItem(fromSection.ID, "Move me", "", 1)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", owner); return c.Next() })
	app.Post("/items/:id/move", RequireItemEdit, MoveItemToSection)
	requestMove := func(sectionID int64) *http.Response {
		body := strings.NewReader("section_id=" + strconv.FormatInt(sectionID, 10))
		req := httptest.NewRequest(http.MethodPost, "/items/"+strconv.FormatInt(item.ID, 10)+"/move", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := requestMove(otherListSection.ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-list move status=%d, want 403", resp.StatusCode)
	}
	resp = requestMove(toSection.ID)
	resp.Body.Close()
	// This lightweight test app has no HTML renderer, so a successful mutation
	// reaches the render step and returns 500. The persisted section verifies
	// that the same-list operation itself was accepted.
	if resp.StatusCode == http.StatusForbidden {
		t.Fatal("same-list move was incorrectly forbidden")
	}
	moved, _ := db.GetItemByID(item.ID)
	if moved.SectionID != toSection.ID {
		t.Fatalf("item section=%d, want %d", moved.SectionID, toSection.ID)
	}
}

func TestCompleteListRolePermissionMatrix(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("matrix-owner", "Owner", "password123", false)
	editor, _ := db.CreateLocalUser("matrix-editor", "Editor", "password123", false)
	viewer, _ := db.CreateLocalUser("matrix-viewer", "Viewer", "password123", false)
	manager, _ := db.CreateLocalUser("matrix-manager", "Manager", "password123", false)
	outsider, _ := db.CreateLocalUser("matrix-outsider", "Outsider", "password123", false)
	admin, _ := db.GetUserByUsername("admin")
	list, _ := db.CreateListForUser(owner.ID, "Role matrix", "")
	_ = db.SetListMember(list.ID, editor.ID, owner.ID, "editor")
	_ = db.SetListMember(list.ID, viewer.ID, owner.ID, "viewer")
	_ = db.SetListMember(list.ID, manager.ID, owner.ID, "manager")

	tests := []struct {
		name  string
		user  *db.User
		admin bool
		want  db.ListPermission
	}{
		{"owner", owner, false, db.ListPermissionOwner},
		{"editor", editor, false, db.ListPermissionEdit},
		{"viewer", viewer, false, db.ListPermissionView},
		{"manager", manager, false, db.ListPermissionManage},
		{"unrelated", outsider, false, db.ListPermissionNone},
		{"administrator", admin, true, db.ListPermissionOwner},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := db.GetUserListPermission(test.user.ID, list.ID, test.admin)
			if err != nil || got != test.want {
				t.Fatalf("permission=%v err=%v, want %v", got, err, test.want)
			}
			for _, threshold := range []db.ListPermission{db.ListPermissionView, db.ListPermissionEdit, db.ListPermissionManage, db.ListPermissionOwner} {
				if (got >= threshold) != (test.want >= threshold) {
					t.Fatalf("threshold %v mismatch for permission %v", threshold, got)
				}
			}
		})
	}
}

func TestOfflineReplayIsRejectedAfterAccessRemoval(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("offline-owner", "Offline Owner", "password123", false)
	editor, _ := db.CreateLocalUser("offline-editor", "Offline Editor", "password123", false)
	list, _ := db.CreateListForUser(owner.ID, "Offline permissions", "")
	section, _ := db.CreateSectionForList(list.ID, "Section")
	item, _ := db.CreateItem(section.ID, "Original", "", 1)
	_ = db.SetListMember(list.ID, editor.ID, owner.ID, "editor")
	_ = db.RemoveListMember(list.ID, editor.ID)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", editor); return c.Next() })
	app.Put("/items/:id", RequireItemEdit, UpdateItem)
	body := strings.NewReader("name=Queued+offline+edit")
	req := httptest.NewRequest(http.MethodPut, "/items/"+strconv.FormatInt(item.ID, 10), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stale offline mutation status=%d, want 403", resp.StatusCode)
	}
	unchanged, _ := db.GetItemByID(item.ID)
	if unchanged.Name != "Original" {
		t.Fatalf("stale offline mutation changed item: %#v", unchanged)
	}
}

func TestPrivateHTMLPartialsRequireListAccess(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("partial-owner", "Partial Owner", "password123", false)
	outsider, _ := db.CreateLocalUser("partial-outsider", "Partial Outsider", "password123", false)
	list, _ := db.CreateListForUser(owner.ID, "Private partials", "")
	section, _ := db.CreateSectionForList(list.ID, "Secret section")
	item, _ := db.CreateItem(section.ID, "Secret item", "", 1)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", outsider); return c.Next() })
	app.Get("/sections/:id/html", RequireSectionView, GetSectionHTML)
	app.Get("/items/:id/html", RequireItemView, GetItemHTML)
	for _, path := range []string{
		"/sections/" + strconv.FormatInt(section.ID, 10) + "/html",
		"/items/" + strconv.FormatInt(item.ID, 10) + "/html",
	} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("private partial %s returned %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestBatchSectionDeleteIsAuthorizedAtomically(t *testing.T) {
	initTestDatabase(t)
	owner, _ := db.CreateLocalUser("batch-owner", "Batch Owner", "password123", false)
	attacker, _ := db.CreateLocalUser("batch-attacker", "Batch Attacker", "password123", false)
	ownerList, _ := db.CreateListForUser(owner.ID, "Owner list", "")
	attackerList, _ := db.CreateListForUser(attacker.ID, "Attacker list", "")
	privateSection, _ := db.CreateSectionForList(ownerList.ID, "Private")
	allowedSection, _ := db.CreateSectionForList(attackerList.ID, "Allowed")

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error { c.Locals("user", attacker); return c.Next() })
	app.Post("/sections/batch-delete", BatchDeleteSections)
	body := "ids=" + strconv.FormatInt(allowedSection.ID, 10) + "," + strconv.FormatInt(privateSection.ID, 10)
	req := httptest.NewRequest(http.MethodPost, "/sections/batch-delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mixed batch status=%d, want 403", resp.StatusCode)
	}
	if _, err = db.GetSectionByID(privateSection.ID); err != nil {
		t.Fatalf("private section was deleted: %v", err)
	}
	if _, err = db.GetSectionByID(allowedSection.ID); err != nil {
		t.Fatalf("authorized section was partially deleted: %v", err)
	}
}

func TestUserSearchReturnsOnlySharingIdentity(t *testing.T) {
	initTestDatabase(t)
	user, _ := db.CreateLocalUser("search-private", "Search User", "password123", false)
	email := "private@example.test"
	_, _ = db.DB.Exec(`UPDATE users SET email=?,last_login_at=CURRENT_TIMESTAMP WHERE id=?`, email, user.ID)
	app := fiber.New()
	app.Get("/users/search", SearchUsers)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/users/search?q=search-private", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	for _, forbidden := range []string{"email", "is_admin", "disabled", "auth_source", "last_login_at", "created_at"} {
		if bytes.Contains(payload, []byte(forbidden)) {
			t.Fatalf("user search exposed %q: %s", forbidden, payload)
		}
	}
}

func TestWebSocketOriginValidation(t *testing.T) {
	app := fiber.New()
	app.Get("/ws", ValidateWebSocketOrigin, func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	for _, test := range []struct {
		origin string
		want   int
	}{{"http://example.com", http.StatusNoContent}, {"https://evil.example", http.StatusForbidden}, {"", http.StatusForbidden}} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
		if test.origin != "" {
			req.Header.Set("Origin", test.origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != test.want {
			t.Fatalf("origin %q status=%d, want %d", test.origin, resp.StatusCode, test.want)
		}
	}
}
