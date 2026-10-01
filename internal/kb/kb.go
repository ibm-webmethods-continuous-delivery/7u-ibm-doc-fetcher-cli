// Package kb converts cached HTML content to agent-ready Markdown and
// manages the kb/ directory structure.
package kb

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	htmlmd "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// MinContentChars is the threshold below which a topic is considered a stub.
// Matches config.MinContentChars but kept here to avoid circular imports.
const MinContentChars = 350

// --------------------------------------------------------------------------
// Regex patterns for frontmatter extraction (ported from spike)
// --------------------------------------------------------------------------

var (
	methodRE = regexp.MustCompile(`\b(GET|POST|PUT|DELETE|PATCH)\b`)
	urlRE    = regexp.MustCompile(`(?:/apis/v\d/rest/[^\s"'` + "`" + `\n]+)`)
	authRE   = regexp.MustCompile(`(?i)x-instance-api-key|Authorization.*Bearer|mcsp_or_isv_token|instance_api_key`)
	adminRE  = regexp.MustCompile(`(?i)admin access|Only admin|need an admin`)
	dateRE   = regexp.MustCompile(`Last Updated:\s*([\d-]{8,10})`)

	// Strips the lastModifiedDate line from Markdown output.
	lastUpdatedLineRE = regexp.MustCompile(`\n?Last Updated:.*\n?`)
)

// Topic holds the Markdown and metadata for one cached content entry.
type Topic struct {
	ProductKey string
	TopicID    string // cache filename without extension, e.g. "wmint_public_apis"
	Lang       string
	Markdown   string // after HTML→MD conversion
	Frontmatter map[string]any
	IsStub     bool
}

// ConvertHTML converts a raw IBM Docs HTML fragment to Markdown.
// It strips noise elements (lastModifiedDate div, related-links nav) and
// decodes IBM redirect links before conversion.
func ConvertHTML(html string) (string, string, error) {
	// Pre-process: strip noise elements before conversion.
	cleaned := stripNoise(html)

	// Extract last-updated date from original HTML before stripping.
	lastUpdated := ""
	if m := regexp.MustCompile(`id="lastModifiedDate"[^>]*>.*?Last Updated.*?([\d]{4}-[\d]{2}-[\d]{2})`).FindStringSubmatch(html); len(m) > 1 {
		lastUpdated = m[1]
	}

	// Decode IBM redirect links: ibm.com/links?url=<encoded-real-url>
	cleaned = decodeIBMLinks(cleaned)

	md, err := htmlmd.ConvertString(cleaned)
	if err != nil {
		return "", lastUpdated, fmt.Errorf("html-to-markdown: %w", err)
	}

	// Strip the "Last Updated: YYYY-MM-DD" line that trafilatura/the converter
	// may have preserved as plain text.
	md = lastUpdatedLineRE.ReplaceAllString(md, "\n")
	md = strings.TrimSpace(md)

	return md, lastUpdated, nil
}

// ExtractFrontmatter derives YAML frontmatter fields from Markdown content.
func ExtractFrontmatter(md, productKey, topicID, lastUpdated string) map[string]any {
	fm := map[string]any{
		"product": productKey,
		"topic":   topicID,
	}
	if lastUpdated != "" {
		fm["last_updated"] = lastUpdated
	} else if m := dateRE.FindStringSubmatch(md); len(m) > 1 {
		fm["last_updated"] = m[1]
	}

	methods := uniqueStrings(methodRE.FindAllString(md, -1))
	if len(methods) > 0 {
		fm["http_methods"] = methods
	}

	var paths []string
	for _, p := range urlRE.FindAllString(md, -1) {
		if len(p) > 5 && !strings.HasSuffix(p, "=") {
			paths = append(paths, p)
		}
	}
	paths = uniqueStrings(paths)
	if len(paths) > 5 {
		paths = paths[:5]
	}
	if len(paths) > 0 {
		fm["api_paths"] = paths
	}

	if authRE.MatchString(md) {
		fm["auth"] = []string{"x-instance-api-key", "Bearer"}
	}
	if adminRE.MatchString(md) {
		fm["requires_admin"] = true
	}

	return fm
}

// RenderFrontmatter serialises a frontmatter map to a YAML block.
// No external YAML dependency — simple hand-rolled serialisation matching
// the spike's output format exactly.
func RenderFrontmatter(fm map[string]any) string {
	// Canonical key order.
	order := []string{"product", "topic", "last_updated", "http_methods", "api_paths", "auth", "requires_admin", "stub", "note"}
	var sb strings.Builder
	sb.WriteString("---\n")
	for _, k := range order {
		v, ok := fm[k]
		if !ok {
			continue
		}
		switch val := v.(type) {
		case []string:
			sb.WriteString(k + ":\n")
			for _, item := range val {
				fmt.Fprintf(&sb, "  - %q\n", item)
			}
		case bool:
			if val {
				fmt.Fprintf(&sb, "%s: true\n", k)
			}
		default:
			fmt.Fprintf(&sb, "%s: %q\n", k, fmt.Sprint(val))
		}
	}
	sb.WriteString("---\n\n")
	return sb.String()
}

// EffectiveLen returns the length of md after stripping the Last Updated line.
func EffectiveLen(md string) int {
	clean := lastUpdatedLineRE.ReplaceAllString(md, "\n")
	return len(strings.TrimSpace(clean))
}

// WriteTopicFile writes a Markdown file with frontmatter to the kb/ tree.
//
//	<dataDir>/kb/<productKey>/<topicID>/<lang>.md
func WriteTopicFile(dataDir, productKey, topicID, lang, content string) error {
	dir := filepath.Join(dataDir, "kb", filepath.FromSlash(productKey), topicID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, lang+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// WriteIndexMD writes kb/<productKey>/INDEX.md for one product.
func WriteIndexMD(dataDir, productKey string, topics []Topic) error {
	now := time.Now().UTC().Format("2006-01-02 15:04 UTC")

	var api, doc, stub []Topic
	for _, t := range topics {
		if t.IsStub {
			stub = append(stub, t)
		} else if _, ok := t.Frontmatter["http_methods"]; ok {
			api = append(api, t)
		} else {
			doc = append(doc, t)
		}
	}

	sortTopics := func(ts []Topic) {
		sort.Slice(ts, func(i, j int) bool {
			return strings.ToLower(firstHeading(ts[i].Markdown)) <
				strings.ToLower(firstHeading(ts[j].Markdown))
		})
	}
	sortTopics(api)
	sortTopics(doc)
	sortTopics(stub)

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Index — %s\n\n", productKey)
	fmt.Fprintf(&sb, "_Generated: %s · %d topics (%d API reference, %d documentation, %d stubs)_\n\n",
		now, len(topics), len(api), len(doc), len(stub))

	writeTable := func(heading string, ts []Topic) {
		if len(ts) == 0 {
			return
		}
		fmt.Fprintf(&sb, "## %s\n\n| Topic | Summary |\n|---|---|\n", heading)
		for _, t := range ts {
			h := firstHeading(t.Markdown)
			if h == "" {
				h = t.TopicID
			}
			sum := truncate(oneLiner(t.Markdown), 90)
			fmt.Fprintf(&sb, "| `%s` | %s — %s |\n", t.TopicID, h, sum)
		}
		sb.WriteString("\n")
	}

	writeTable("API Reference Topics", api)
	writeTable("Documentation Topics", doc)

	if len(stub) > 0 {
		sb.WriteString("## Navigation Stubs\n_These are short section-title pages merged into `_stubs_merged.md`._\n\n")
		names := make([]string, len(stub))
		for i, t := range stub {
			names[i] = "`" + t.TopicID + "`"
		}
		sort.Strings(names)
		sb.WriteString(strings.Join(names, ", ") + "\n\n")
	}

	dir := filepath.Join(dataDir, "kb", filepath.FromSlash(productKey))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "INDEX.md"), []byte(sb.String()), 0o644)
}

// WriteTopLevelIndex writes kb/INDEX.md across all products.
func WriteTopLevelIndex(dataDir string, productTopicCounts map[string]int) error {
	now := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	var sb strings.Builder
	sb.WriteString("# IBM Documentation — Knowledge Base Index\n\n")
	fmt.Fprintf(&sb, "_Generated: %s_\n\n## Products\n\n| Product key | Topics | Index |\n|---|---|---|\n", now)

	keys := make([]string, 0, len(productTopicCounts))
	for k := range productTopicCounts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, pk := range keys {
		n := productTopicCounts[pk]
		fmt.Fprintf(&sb, "| `%s` | %d | [INDEX.md](%s/INDEX.md) |\n", pk, n, pk)
	}
	sb.WriteString("\n")

	if err := os.MkdirAll(filepath.Join(dataDir, "kb"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "kb", "INDEX.md"), []byte(sb.String()), 0o644)
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// stripNoise removes IBM Docs boilerplate HTML before conversion.
func stripNoise(html string) string {
	// Strip lastModifiedDate div.
	html = regexp.MustCompile(`(?s)<div[^>]*id="lastModifiedDate"[^>]*>.*?</div>`).ReplaceAllString(html, "")
	// Strip related-links / bottom-section-parent nav blocks.
	html = regexp.MustCompile(`(?s)<nav[^>]*class="[^"]*(?:related-links|bottom-section-parent)[^"]*"[^>]*>.*?</nav>`).ReplaceAllString(html, "")
	// Strip img tags entirely.
	html = regexp.MustCompile(`(?i)<img[^>]*/?>|<img[^>]*>.*?</img>`).ReplaceAllString(html, "")
	// Unwrap <span class="ph"> product name placeholders — keep text.
	html = regexp.MustCompile(`(?i)<span[^>]*class="ph"[^>]*>(.*?)</span>`).ReplaceAllStringFunc(html, func(m string) string {
		inner := regexp.MustCompile(`(?i)<span[^>]*>(.*?)</span>`).FindStringSubmatch(m)
		if len(inner) > 1 {
			return inner[1]
		}
		return ""
	})
	return html
}

// decodeIBMLinks replaces ibm.com/links?url=<encoded> with the decoded target URL.
func decodeIBMLinks(html string) string {
	return regexp.MustCompile(`href="https?://(?:www\.)?ibm\.com/links\?url=([^"]+)"`).
		ReplaceAllStringFunc(html, func(m string) string {
			sub := regexp.MustCompile(`url=([^"]+)`).FindStringSubmatch(m)
			if len(sub) < 2 {
				return m
			}
			decoded := strings.ReplaceAll(sub[1], "%3A", ":") // minimal decode for common case
			decoded = strings.ReplaceAll(decoded, "%2F", "/")
			decoded = strings.ReplaceAll(decoded, "%3F", "?")
			decoded = strings.ReplaceAll(decoded, "%3D", "=")
			decoded = strings.ReplaceAll(decoded, "%26", "&")
			return `href="` + decoded + `"`
		})
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func firstHeading(md string) string {
	for _, line := range strings.SplitN(md, "\n", 20) {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func oneLiner(md string) string {
	for _, line := range strings.Split(md, "\n") {
		s := strings.TrimSpace(line)
		if s != "" && !strings.HasPrefix(s, "#") && !strings.HasPrefix(s, "Last Updated") {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
