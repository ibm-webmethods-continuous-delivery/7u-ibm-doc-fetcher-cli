// Package cache manages on-disk JSON cache for TOC and Content API responses.
//
// Directory layout:
//
//	<data>/cache/toc/<product-key>/<lang>.json
//	<data>/cache/content/<product-key>/<topic-filename>/<lang>.json
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TTL is the cache validity window. Entries older than this are stale.
const TTL = 24 * time.Hour

var cpParamRE = regexp.MustCompile(`\?cp=[^&]*(&.*)?$`)

// KeyTOC returns the filesystem path for a TOC cache entry.
//
//	product key: "wm-integration-ipaas"  lang: "en"
//	→ <dataDir>/cache/toc/wm-integration-ipaas/en.json
func KeyTOC(dataDir, productKey, lang string) string {
	return filepath.Join(dataDir, "cache", "toc", filepath.FromSlash(productKey), lang+".json")
}

// KeyContent returns the filesystem path for a Content cache entry.
// The ?cp=... cross-version query parameter is stripped from href before
// constructing the path so the same topic fetched from different versions
// maps to the same file.
//
// The IBM internal product-ID prefix (first path segment of the href, e.g.
// "SSZMH3N_1.0.325" or "SSGOVO") is stripped and the remaining path is used
// as the cache sub-key. This preserves uniqueness for topics that share a
// filename (e.g. multiple "index.html" under different parent directories).
//
//	href: "SSZMH3N_1.0.325/src/pages/ecosystem/action-ansible/index.html"
//	→ <dataDir>/cache/content/<productKey>/src/pages/ecosystem/action-ansible/index/en.json
//
//	href: "SSGOVO/wmint_public_apis.html"
//	→ <dataDir>/cache/content/<productKey>/wmint_public_apis/en.json
func KeyContent(dataDir, productKey, href, lang string) string {
	clean := cpParamRE.ReplaceAllString(href, "")
	clean = strings.TrimSuffix(clean, ".html")

	// Strip the IBM internal product-ID prefix (first path segment).
	// It looks like "SSZMH3N_1.0.325" or "SSGOVO" — all-caps alphanumeric with optional _.
	if idx := strings.Index(clean, "/"); idx >= 0 {
		clean = clean[idx+1:]
	}

	return filepath.Join(dataDir, "cache", "content", filepath.FromSlash(productKey), filepath.FromSlash(clean), lang+".json")
}

// Valid reports whether the cache file at path is present and within TTL.
func Valid(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < TTL
}

// LoadJSON reads and JSON-decodes the cache file at path into v.
func LoadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// SaveJSON atomically writes v as JSON to path (write to temp + rename).
func SaveJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cache mkdir: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cache write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cache rename: %w", err)
	}
	return nil
}
