package handlers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"sort"

	"github.com/gofiber/fiber/v2"
)

// AssetHash is an 8-char hex SHA256 digest over all files in the embedded
// static FS. Computed once at startup by ComputeAssetHash and used as a
// cache-buster in asset URLs (e.g. app.js?v=<AssetHash>).
var AssetHash string

// ServiceWorkerBytes holds sw.js with __CACHE_VERSION__ and __ASSET_HASH__
// placeholders replaced by AssetHash. Built once at startup by
// BuildServiceWorker and served by ServeServiceWorker.
var ServiceWorkerBytes []byte

// ComputeAssetHash computes a deterministic 8-char hex digest from the
// content of every file in fsys. Paths are sorted so the hash is stable
// across runs of the same build.
func ComputeAssetHash(fsys fs.FS) (string, error) {
	var paths []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return "", err
		}
		h.Write(data)
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:8], nil
}

// BuildServiceWorker reads sw.js from fsys and replaces the placeholders
// __CACHE_VERSION__ and __ASSET_HASH__ with hash.
func BuildServiceWorker(fsys fs.FS, hash string) ([]byte, error) {
	raw, err := fs.ReadFile(fsys, "sw.js")
	if err != nil {
		return nil, err
	}
	out := bytes.ReplaceAll(raw, []byte("__CACHE_VERSION__"), []byte(hash))
	out = bytes.ReplaceAll(out, []byte("__ASSET_HASH__"), []byte(hash))
	return out, nil
}

// ServeServiceWorker serves the pre-built ServiceWorkerBytes. SW must not be
// hard-cached by the browser because it controls all other caches; use
// no-cache so browsers revalidate on every navigation.
func ServeServiceWorker(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/javascript; charset=utf-8")
	c.Set("Cache-Control", "no-cache")
	return c.Send(ServiceWorkerBytes)
}
