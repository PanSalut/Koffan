package handlers

import (
	"shopping-list/i18n"

	"github.com/gofiber/fiber/v2"
)

// getLang extracts the user's language from the lang cookie
func getLang(c *fiber.Ctx) string {
	if lang := c.Cookies("lang"); lang != "" {
		return lang
	}
	return i18n.GetDefaultLang()
}

// sendError sends a translated error message
func sendError(c *fiber.Ctx, status int, key string) error {
	return c.Status(status).SendString(i18n.Get(getLang(c), key))
}
