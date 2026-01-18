package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// AppVersion is set at build time via ldflags
var AppVersion = "dev"

const (
	githubTagsURL   = "https://api.github.com/repos/PanSalut/Koffan/tags"
	versionCacheTTL = 1 * time.Hour
)

var (
	cachedVersion     string
	cachedVersionTime time.Time
	versionMutex      sync.RWMutex
)

type githubTag struct {
	Name string `json:"name"`
}

type versionResponse struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
}

// GetVersion returns current version and checks for updates
func GetVersion(c *fiber.Ctx) error {
	latest := getCachedVersion()
	updateAvailable := isNewerVersion(latest, AppVersion)

	response := versionResponse{
		Current:         AppVersion,
		Latest:          latest,
		UpdateAvailable: updateAvailable,
	}

	if updateAvailable && latest != "unknown" {
		response.ReleaseURL = "https://github.com/PanSalut/Koffan/releases/tag/" + latest
	}

	return c.JSON(response)
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

// isNewerVersion compares semver strings, returns true if latest > current
func isNewerVersion(latest, current string) bool {
	if latest == "unknown" || latest == "" || current == "dev" {
		return false
	}

	latestParts := parseVersion(latest)
	currentParts := parseVersion(current)

	for i := 0; i < 3; i++ {
		if latestParts[i] > currentParts[i] {
			return true
		}
		if latestParts[i] < currentParts[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		var n int
		for _, c := range parts[i] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			} else {
				break
			}
		}
		result[i] = n
	}
	return result
}
