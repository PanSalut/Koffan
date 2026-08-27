package handlers

import (
	"database/sql"
	"github.com/gofiber/fiber/v2"
	"shopping-list/db"
	"strconv"
)

func currentIdentity(c *fiber.Ctx) (int64, bool, error) {
	u, e := CurrentUser(c)
	if e != nil {
		return 0, false, e
	}
	return u.ID, u.IsAdmin, nil
}
func requirePermission(c *fiber.Ctx, listID int64, want db.ListPermission, errorAsNotFound bool) error {
	uid, admin, e := currentIdentity(c)
	if e != nil {
		return e
	}
	p, e := db.GetUserListPermission(uid, listID, admin)
	if e == sql.ErrNoRows || (e == nil && p < want) {
		if errorAsNotFound {
			return fiber.ErrNotFound
		}
		return fiber.ErrForbidden
	}
	if e != nil {
		return e
	}
	return c.Next()
}
func RequireListView(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	return requirePermission(c, id, db.ListPermissionView, true)
}
func RequireListEdit(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	return requirePermission(c, id, db.ListPermissionEdit, false)
}
func RequireListManage(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	return requirePermission(c, id, db.ListPermissionManage, false)
}
func RequireListOwner(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	return requirePermission(c, id, db.ListPermissionOwner, false)
}
func RequireSectionEdit(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	lid, e := db.ListIDForSection(id)
	if e == sql.ErrNoRows {
		return fiber.ErrNotFound
	}
	if e != nil {
		return e
	}
	return requirePermission(c, lid, db.ListPermissionEdit, false)
}

func RequireSectionView(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	lid, e := db.ListIDForSection(id)
	if e == sql.ErrNoRows {
		return fiber.ErrNotFound
	}
	if e != nil {
		return e
	}
	return requirePermission(c, lid, db.ListPermissionView, true)
}
func RequireItemEdit(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	lid, e := db.ListIDForItem(id)
	if e == sql.ErrNoRows {
		return fiber.ErrNotFound
	}
	if e != nil {
		return e
	}
	return requirePermission(c, lid, db.ListPermissionEdit, false)
}

func RequireItemView(c *fiber.Ctx) error {
	id, e := strconv.ParseInt(c.Params("id"), 10, 64)
	if e != nil {
		return fiber.ErrBadRequest
	}
	lid, e := db.ListIDForItem(id)
	if e == sql.ErrNoRows {
		return fiber.ErrNotFound
	}
	if e != nil {
		return e
	}
	return requirePermission(c, lid, db.ListPermissionView, true)
}
