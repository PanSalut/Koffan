package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed *.json
var localesFS embed.FS

// LocaleMeta contains language metadata
type LocaleMeta struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Flag string `json:"flag"`
}

// Locale represents a complete set of translations
type Locale struct {
	Meta LocaleMeta
	Raw  map[string]interface{}
}

var (
	locales           = make(map[string]*Locale)
	localesMu         sync.RWMutex
	defaultLang       = "en"
	cachedAllLocales  map[string]map[string]interface{}
)

// rebuildCachedAllLocales rebuilds the cached map returned by GetAllLocales.
// Caller MUST hold localesMu write lock.
func rebuildCachedAllLocales() {
	result := make(map[string]map[string]interface{}, len(locales))
	for code, locale := range locales {
		result[code] = locale.Raw
	}
	cachedAllLocales = result
}

// SetDefaultLang sets the default language (must be called after Init)
func SetDefaultLang(lang string) {
	localesMu.Lock()
	defer localesMu.Unlock()
	if _, exists := locales[lang]; exists {
		defaultLang = lang
	}
}

// GetDefaultLang returns the current default language
func GetDefaultLang() string {
	localesMu.RLock()
	defer localesMu.RUnlock()
	return defaultLang
}

// Init loads all available translations
func Init() error {
	files, err := localesFS.ReadDir(".")
	if err != nil {
		return err
	}

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		data, err := localesFS.ReadFile(file.Name())
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file.Name(), err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("failed to parse %s: %w", file.Name(), err)
		}

		locale := &Locale{Raw: raw}

		// Parse meta
		if meta, ok := raw["meta"].(map[string]interface{}); ok {
			if code, ok := meta["code"].(string); ok {
				locale.Meta.Code = code
			}
			if name, ok := meta["name"].(string); ok {
				locale.Meta.Name = name
			}
			if flag, ok := meta["flag"].(string); ok {
				locale.Meta.Flag = flag
			}
		}

		localesMu.Lock()
		locales[locale.Meta.Code] = locale
		localesMu.Unlock()
	}

	// Build cached all-locales map once after initial load.
	localesMu.Lock()
	rebuildCachedAllLocales()
	localesMu.Unlock()

	return nil
}

// Get retrieves a translation for a key in format "section.key"
func Get(lang, key string) string {
	localesMu.RLock()
	locale, ok := locales[lang]
	if !ok {
		locale = locales[defaultLang]
	}
	localesMu.RUnlock()

	if locale == nil {
		return key
	}

	parts := strings.Split(key, ".")
	current := locale.Raw

	for i, part := range parts {
		val, exists := current[part]
		if !exists {
			return key
		}

		if i == len(parts)-1 {
			if str, ok := val.(string); ok {
				return str
			}
			return key
		}

		if next, ok := val.(map[string]interface{}); ok {
			current = next
		} else {
			return key
		}
	}

	return key
}

// GetWithParams retrieves a translation with parameter substitution {{param}}
func GetWithParams(lang, key string, params map[string]string) string {
	text := Get(lang, key)
	for k, v := range params {
		text = strings.ReplaceAll(text, "{{"+k+"}}", v)
	}
	return text
}

// GetAll returns all translations for a language (for passing to JS)
func GetAll(lang string) map[string]interface{} {
	localesMu.RLock()
	defer localesMu.RUnlock()

	if locale, ok := locales[lang]; ok {
		return locale.Raw
	}
	if locale, ok := locales[defaultLang]; ok {
		return locale.Raw
	}
	return nil
}

// GetAllLocales returns all translations for all languages.
// Returns a cached map built once during Init (read-only; do not mutate).
func GetAllLocales() map[string]map[string]interface{} {
	localesMu.RLock()
	defer localesMu.RUnlock()
	return cachedAllLocales
}

// AvailableLocales returns a list of available languages
func AvailableLocales() []LocaleMeta {
	localesMu.RLock()
	defer localesMu.RUnlock()

	result := make([]LocaleMeta, 0, len(locales))
	for _, locale := range locales {
		result = append(result, locale.Meta)
	}
	return result
}

// T is a shorthand function for use in templates
func T(lang, key string) string {
	return Get(lang, key)
}
