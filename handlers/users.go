package handlers

import (
	"github.com/gofiber/fiber/v2"
	"shopping-list/db"
	"shopping-list/i18n"
	"strconv"
	"strings"
)

func AdminUsersPage(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	users, e := db.ListUsers()
	if e != nil {
		return e
	}
	groups, e := db.ListGroups()
	if e != nil {
		return e
	}
	lists, e := db.GetAllLists()
	if e != nil {
		return e
	}
	access, e := db.ListGroupAccess()
	if e != nil {
		return e
	}
	u, _ := CurrentUser(c)
	return c.Render("admin_users", fiber.Map{"Users": users, "Groups": groups, "Lists": lists, "GroupAccess": access, "CurrentUser": u, "Translations": i18n.GetAllLocales(), "Locales": i18n.AvailableLocales(), "DefaultLang": i18n.GetDefaultLang()})
}

func GetUsers(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	users, e := db.ListUsers()
	if e != nil {
		return e
	}
	return c.JSON(users)
}
func CreateUser(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		IsAdmin     bool   `json:"is_admin"`
	}
	if e := c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.DisplayName) == "" {
		return fiber.ErrBadRequest
	}
	u, e := db.CreateLocalUser(req.Username, req.DisplayName, req.Password, req.IsAdmin)
	if e != nil {
		return c.Status(409).JSON(fiber.Map{"error": e.Error()})
	}
	return c.Status(201).JSON(u)
}
func UpdateUser(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		IsAdmin     bool   `json:"is_admin"`
		Disabled    bool   `json:"disabled"`
	}
	if e = c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	target, e := db.GetUserByID(id)
	if e != nil {
		return fiber.ErrNotFound
	}
	if target.AuthSource == "oidc" {
		// OIDC account availability is controlled by the identity provider.
		req.Disabled = false
	}
	actor, _ := CurrentUser(c)
	u, e := db.UpdateUserByAdmin(actor.ID, id, req.Username, req.DisplayName, req.IsAdmin, req.Disabled, req.Password)
	if e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	// The connection caches the user's administrator flag. Reconnect after any
	// account update so demotions and disables take effect immediately.
	DisconnectUserWebSockets(id)
	return c.JSON(u)
}

// InvalidateUserSessions immediately logs an OIDC user out of every Koffan
// session without changing their account or identity-provider state.
func InvalidateUserSessions(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	user, e := db.GetUserByID(id)
	if e != nil {
		return fiber.ErrNotFound
	}
	if user.AuthSource != "oidc" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "session invalidation is only available for OIDC users"})
	}
	if e = db.InvalidateUserSessions(id); e != nil {
		return e
	}
	DisconnectUserWebSockets(id)
	return c.JSON(fiber.Map{"invalidated": true})
}
func DeleteUser(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	actor, _ := CurrentUser(c)
	count, e := db.DeleteUserPermanently(actor.ID, id)
	if e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	DisconnectUserWebSockets(id)
	return c.JSON(fiber.Map{"deleted": true, "deleted_lists": count})
}
func GetGroups(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	v, e := db.ListGroups()
	if e != nil {
		return e
	}
	return c.JSON(v)
}
func CreateGroup(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		IsAdmin     bool   `json:"is_admin"`
	}
	if e := c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	g, e := db.CreateGroupWithAdmin(req.Name, req.Description, nil, req.IsAdmin)
	if e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	return c.Status(201).JSON(g)
}
func UpdateGroupAdmin(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if e = c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	users, e := db.GetGroupUsers(id)
	if e != nil {
		return e
	}
	if e = db.SetGroupAdmin(id, req.IsAdmin); e != nil {
		return e
	}
	for _, user := range users {
		DisconnectUserWebSockets(user.ID)
	}
	return c.SendStatus(204)
}
func UpdateGroupDetails(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if e = c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	group, e := db.UpdateGroup(id, req.Name, req.Description)
	if e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	return c.JSON(group)
}
func DeleteGroup(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	users, e := db.GetGroupUsers(id)
	if e != nil {
		return e
	}
	if e = db.DeleteGroup(id); e != nil {
		return e
	}
	for _, user := range users {
		DisconnectUserWebSockets(user.ID)
	}
	return c.SendStatus(204)
}
func AddGroupUser(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	gid, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	uid, _ := strconv.ParseInt(c.Params("userId"), 10, 64)
	if e := db.AddUserToGroup(uid, gid, "local"); e != nil {
		return e
	}
	return c.SendStatus(204)
}
func RemoveGroupUser(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	gid, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	uid, _ := strconv.ParseInt(c.Params("userId"), 10, 64)
	if e := db.RemoveUserFromGroup(uid, gid); e != nil {
		return e
	}
	DisconnectUserWebSockets(uid)
	return c.SendStatus(204)
}
func SetGroupListAccess(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	gid, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	listID, _ := strconv.ParseInt(c.Params("listId"), 10, 64)
	var req struct {
		Role string `json:"role"`
	}
	if e := c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	u, _ := CurrentUser(c)
	if e := db.SetGroupListAccess(listID, gid, u.ID, req.Role); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	return c.SendStatus(204)
}
func DeleteGroupListAccess(c *fiber.Ctx) error {
	if e := RequireAdmin(c); e != nil {
		return e
	}
	gid, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	listID, _ := strconv.ParseInt(c.Params("listId"), 10, 64)
	users, e := db.GetGroupUsers(gid)
	if e != nil {
		return e
	}
	if e := db.RemoveGroupListAccess(listID, gid); e != nil {
		return e
	}
	for _, user := range users {
		DisconnectUserWebSockets(user.ID)
	}
	return c.SendStatus(204)
}
func SearchUsers(c *fiber.Ctx) error {
	users, e := db.SearchUsers(c.Query("q"), 20)
	if e != nil {
		return e
	}
	type searchResult struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}
	results := make([]searchResult, 0, len(users))
	for _, user := range users {
		results = append(results, searchResult{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName})
	}
	return c.JSON(results)
}
func GetListMembers(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	members, e := db.GetListMembers(id)
	if e != nil {
		return e
	}
	list, e := db.GetListByID(id)
	if e != nil {
		return e
	}
	owner, e := db.GetUserByID(list.OwnerUserID)
	if e != nil {
		return e
	}
	return c.JSON(fiber.Map{"owner": owner, "members": members})
}

func ListAccessPage(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	list, e := db.GetListByID(id)
	if e != nil {
		return fiber.ErrNotFound
	}
	owner, e := db.GetUserByID(list.OwnerUserID)
	if e != nil {
		return e
	}
	members, e := db.GetListMembers(id)
	if e != nil {
		return e
	}
	users, e := db.SearchUsers("", 50)
	if e != nil {
		return e
	}
	groups, e := db.ListGroups()
	if e != nil {
		return e
	}
	groupAccess, e := db.GetListGroupAccess(id)
	if e != nil {
		return e
	}
	u, _ := CurrentUser(c)
	canTransfer := u != nil && (u.IsAdmin || u.ID == list.OwnerUserID)
	return c.Render("list_access", fiber.Map{"List": list, "Owner": owner, "Members": members, "Users": users, "Groups": groups, "GroupAccess": groupAccess, "CanTransfer": canTransfer, "CurrentUser": u, "Translations": i18n.GetAllLocales(), "Locales": i18n.AvailableLocales(), "DefaultLang": i18n.GetDefaultLang()})
}

func TransferListOwnership(c *fiber.Ctx) error {
	listID, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	var req struct {
		UserID int64 `json:"user_id"`
	}
	if e = c.BodyParser(&req); e != nil || req.UserID < 1 {
		return fiber.ErrBadRequest
	}
	u, e := CurrentUser(c)
	if e != nil {
		return e
	}
	list, e := db.TransferListOwnership(listID, req.UserID, u.ID)
	if e != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": e.Error()})
	}
	return c.JSON(list)
}

func SetListGroupAccess(c *fiber.Ctx) error {
	listID, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	groupID, e := strconv.ParseInt(c.Params("groupId"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	var req struct {
		Role string `json:"role"`
	}
	if e = c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	u, e := CurrentUser(c)
	if e != nil {
		return e
	}
	if e = db.SetGroupListAccess(listID, groupID, u.ID, req.Role); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	return c.SendStatus(204)
}
func DeleteListGroupAccess(c *fiber.Ctx) error {
	listID, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	groupID, e := strconv.ParseInt(c.Params("groupId"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	users, e := db.GetGroupUsers(groupID)
	if e != nil {
		return e
	}
	if e = db.RemoveGroupListAccess(listID, groupID); e != nil {
		return e
	}
	for _, user := range users {
		DisconnectUserWebSockets(user.ID)
	}
	return c.SendStatus(204)
}
func AddListMember(c *fiber.Ctx) error    { return setListMember(c) }
func UpdateListMember(c *fiber.Ctx) error { return setListMember(c) }
func setListMember(c *fiber.Ctx) error {
	listID, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	var req struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if e := c.BodyParser(&req); e != nil {
		return fiber.ErrBadRequest
	}
	if p := c.Params("userId"); p != "" {
		req.UserID, _ = strconv.ParseInt(p, 10, 64)
	}
	u, e := CurrentUser(c)
	if e != nil {
		return e
	}
	if e = db.SetListMember(listID, req.UserID, u.ID, req.Role); e != nil {
		return c.Status(400).JSON(fiber.Map{"error": e.Error()})
	}
	return c.SendStatus(204)
}
func DeleteListMember(c *fiber.Ctx) error {
	listID, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	userID, _ := strconv.ParseInt(c.Params("userId"), 10, 64)
	if e := db.RemoveListMember(listID, userID); e != nil {
		return e
	}
	DisconnectUserWebSockets(userID)
	return c.SendStatus(204)
}
