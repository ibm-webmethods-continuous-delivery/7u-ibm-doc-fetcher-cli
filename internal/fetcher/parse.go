// Package fetcher implements URL parsing, slug matching, TOC walking, and
// the index.json record-keeping layer.
package fetcher

import (
	"net/url"
	"regexp"
	"strings"
)

// stopWords are filtered out when deriving a slug from a topic label.
var stopWords = map[string]bool{
	"and": true, "the": true, "a": true, "an": true,
	"of": true, "in": true, "for": true, "to": true,
	"by": true, "with": true, "from": true,
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9 ]`)

// labelToSlug normalises a TOC label to the slug format IBM uses in browser URLs.
// Mirrors the Python spike's _label_to_slug exactly.
func labelToSlug(label string) string {
	words := strings.Fields(nonAlphaNum.ReplaceAllString(strings.ToLower(label), ""))
	filtered := words[:0]
	for _, w := range words {
		if !stopWords[w] {
			filtered = append(filtered, w)
		}
	}
	return strings.Join(filtered, "-")
}

var dashOrUnderscore = regexp.MustCompile(`[-_]`)

// SlugMatches reports whether a URL topic slug matches a TOC node's href
// filename or label. Implements the three-strategy algorithm from the spike:
//
//  1. Label-derived slug match
//  2. Exact filename match
//  3. Progressive suffix match (handles "references-public-apis" → "public-apis")
func SlugMatches(slug, href, label string) bool {
	slugNorm := dashOrUnderscore.ReplaceAllString(strings.ToLower(slug), "-")

	// Strategy 1: label-derived slug
	if label != "" && labelToSlug(label) == slugNorm {
		return true
	}

	if href == "" {
		return false
	}

	// Strip ?cp= and any query params, take the basename, strip .html
	clean := href
	if idx := strings.Index(clean, "?"); idx >= 0 {
		clean = clean[:idx]
	}
	fname := strings.TrimSuffix(filepath_Base(clean), ".html")
	fnameNorm := dashOrUnderscore.ReplaceAllString(strings.ToLower(fname), "-")

	// Strategy 2: exact filename match
	if fnameNorm == slugNorm {
		return true
	}

	// Strategy 3: progressive suffix — slug may have a section prefix
	// e.g. "references-public-apis" → try "public-apis" (min 6 chars)
	parts := strings.Split(slugNorm, "-")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "-")
		if len(suffix) >= 6 && (fnameNorm == suffix || strings.HasSuffix(fnameNorm, "-"+suffix)) {
			return true
		}
	}
	return false
}

// filepath_Base is strings.LastIndex("/") equivalent to avoid importing path/filepath
// (which is OS-specific for separators). IBM hrefs always use forward slashes.
func filepath_Base(s string) string {
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// ParseIBMDocsURL parses a standard IBM Docs browser URL into its components.
//
// Handles:
//
//	https://www.ibm.com/docs/en/<product>?topic=<slug>
//	https://www.ibm.com/docs/en/<a>/<b>/<c>?topic=<slug>   (multi-segment key)
//
// Returns productKey, topicSlug, lang. topicSlug is empty when not present.
// Returns ("", "", "") on parse failure.
func ParseIBMDocsURL(rawURL string) (productKey, topicSlug, lang string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", ""
	}
	// path: /docs/en/wm-integration-ipaas  or  /docs/en/a/b/c
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	// parts[0]="docs", parts[1]="en", parts[2:]= product key segments
	if len(parts) < 3 || parts[0] != "docs" {
		return "", "", ""
	}
	lang = parts[1]
	if lang == "" {
		lang = "en"
	}
	productKey = strings.Join(parts[2:], "/")
	topicSlug = u.Query().Get("topic")
	return productKey, topicSlug, lang
}
