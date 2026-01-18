package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

const (
	githubTagsURL    = "https://api.github.com/repos/PanSalut/Koffan/tags"
	versionCacheTTL  = 1 * time.Hour
)

var (
	cachedVersion     string
	cachedVersionTime time.Time
	versionMutex      sync.RWMutex
)

type githubTag struct {
	Name string `json:"name"`
}

// GetVersion returns the current version from GitHub tags (cached)
func GetVersion(c *fiber.Ctx) error {
	version := getCachedVersion()

	return c.JSON(fiber.Map{
		"version": version,
	})
}

func getCachedVersion() string {
	versionMutex.RLock()
	if cachedVersion != "" && time.Since(cachedVersionTime) < versionCacheTTL {
		v := cachedVersion
		versionMutex.RUnlock()
		return v
	}
	versionMutex.RUnlock()

	// Fetch fresh version
	version := fetchLatestVersion()

	versionMutex.Lock()
	cachedVersion = version
	cachedVersionTime = time.Now()
	versionMutex.Unlock()

	return version
}

func fetchLatestVersion() string {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(githubTagsURL)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "unknown"
	}

	var tags []githubTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "unknown"
	}

	if len(tags) == 0 {
		return "unknown"
	}

	return tags[0].Name
}
