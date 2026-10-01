// Package ibmdocs provides a client for the IBM Docs public CDN API.
//
// Three unauthenticated endpoints on 1.www.s81c.com (default):
//
//	TOC:     GET /docs/api/v1/toc/{product_key}?lang={lang}
//	Content: GET /docs/api/v1/content/{href}?parsebody=true&lang={lang}
//	Search:  GET /docs/api/v1/search?query={q}&lang={lang}&limit={n}&start={s}
package ibmdocs

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TOCNode mirrors the IBM Docs TOC API shape.
type TOCNode struct {
	Label  string    `json:"label"`
	Href   string    `json:"href"`
	Topics []TOCNode `json:"topics"`
}

// TOCResponse is the top-level envelope returned by the TOC endpoint.
type TOCResponse struct {
	TOC TOCNode `json:"toc"`
}

// SearchHit is one result from the Search endpoint.
type SearchHit struct {
	Title             string          `json:"title"`
	FullURL           string          `json:"fullurl"`
	Snippet           string          `json:"snippet"`
	ProductBreadCrumb string          `json:"productBreadCrumb"`
	ReadTime          json.RawMessage `json:"readTime"` // int or "" depending on topic
	Date              string          `json:"date"`
	Product           struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	} `json:"product"`
}

// SearchResponse is the top-level envelope from the Search endpoint.
type SearchResponse struct {
	Hits  int         `json:"hits"`
	Next  int         `json:"next"`
	Start int         `json:"start"`
	Items []SearchHit `json:"topics"`
}

// Client calls the IBM Docs CDN API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	debugDir   string // non-empty when --http-debug is active
}

// New creates a Client.
//
//   - baseURL: CDN base, e.g. "https://1.www.s81c.com"
//   - timeout: per-request timeout
//   - debugDir: directory for HTTP debug dumps; empty string disables dumps
//   - logger: structured logger (pass slog.Default() if none)
func New(baseURL string, timeout time.Duration, debugDir string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
		debugDir:   debugDir,
	}
}

// userAgent returns the tool's User-Agent header value.
// Injected at link time via ldflags from cmd package.
var ToolVersion = "dev"

func userAgent() string {
	return "Mozilla/5.0 (compatible; ibmdocs/" + ToolVersion + ")"
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Referer", "https://www.ibm.com/docs/")
	req.Header.Set("Accept", "*/*")

	if c.debugDir != "" {
		dumpRequest(c.debugDir, req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if c.debugDir != "" {
		dumpResponse(c.debugDir, req, resp)
	}

	return resp, nil
}

// FetchTOC fetches the product Table of Contents.
func (c *Client) FetchTOC(productKey, lang string) (*TOCResponse, error) {
	// Product key contains slashes that must be preserved as path separators.
	// Build the URL manually — url.PathEscape would encode the slashes.
	rawURL := fmt.Sprintf("%s/docs/api/v1/toc/%s?lang=%s",
		c.baseURL, productKey, url.QueryEscape(lang))

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("FetchTOC build request: %w", err)
	}

	c.logger.Debug("fetching TOC", "product", productKey, "lang", lang)
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("FetchTOC %s: %w", productKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{URL: rawURL, StatusCode: resp.StatusCode}
	}

	var result TOCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("FetchTOC decode %s: %w", productKey, err)
	}
	return &result, nil
}

// FetchContent fetches an HTML fragment for a topic href and returns the raw body.
// The href is passed as-is (including ?cp= if present) — it is URL-path-encoded
// for the request but the original is used for cache-key derivation by the caller.
func (c *Client) FetchContent(href, lang string) (string, error) {
	// href may contain / which must stay as path separators, and ?cp= which
	// must be preserved as a query string. Encode only the path component.
	var rawURL string
	if idx := strings.Index(href, "?"); idx >= 0 {
		pathPart := href[:idx]
		queryPart := href[idx:]
		rawURL = fmt.Sprintf("%s/docs/api/v1/content/%s%s&parsebody=true&lang=%s",
			c.baseURL, pathPart, queryPart, lang)
	} else {
		rawURL = fmt.Sprintf("%s/docs/api/v1/content/%s?parsebody=true&lang=%s",
			c.baseURL, href, lang)
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("FetchContent build request: %w", err)
	}

	c.logger.Debug("fetching content", "href", href, "lang", lang)
	resp, err := c.do(req)
	if err != nil {
		return "", fmt.Errorf("FetchContent %s: %w", href, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &APIError{URL: rawURL, StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("FetchContent read %s: %w", href, err)
	}
	return string(body), nil
}

// Search executes a keyword search and returns one page of results.
func (c *Client) Search(query, lang string, limit, start int) (*SearchResponse, error) {
	rawURL := fmt.Sprintf("%s/docs/api/v1/search?query=%s&lang=%s&limit=%d&start=%d",
		c.baseURL,
		url.QueryEscape(query),
		url.QueryEscape(lang),
		limit,
		start,
	)

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Search build request: %w", err)
	}

	c.logger.Debug("search", "query", query, "lang", lang, "limit", limit, "start", start)
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("Search %q: %w", query, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{URL: rawURL, StatusCode: resp.StatusCode}
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Search decode: %w", err)
	}
	return &result, nil
}

// APIError is returned when the CDN returns a non-200 status.
type APIError struct {
	URL        string
	StatusCode int
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("IBM Docs API returned HTTP %d for %s", e.StatusCode, e.URL)
	if e.StatusCode != http.StatusNotFound {
		msg += " — if this persists, check --cdn-base-url"
	}
	return msg
}

// IsNotFound returns true when the API returned 404.
func IsNotFound(err error) bool {
	ae, ok := err.(*APIError)
	return ok && ae.StatusCode == http.StatusNotFound
}

// --------------------------------------------------------------------------
// HTTP debug dump helpers
// --------------------------------------------------------------------------

func dumpRequest(dir string, req *http.Request) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := debugFilename(dir, req.Method, req.URL, "req")
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s HTTP/1.1\n", req.Method, req.URL.RequestURI())
	fmt.Fprintf(&sb, "Host: %s\n", req.URL.Host)
	req.Header.Write(&sb) //nolint:errcheck
	sb.WriteString("\n[no body]\n")
	os.WriteFile(name, []byte(sb.String()), 0o644) //nolint:errcheck
}

func dumpResponse(dir string, req *http.Request, resp *http.Response) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := debugFilename(dir, req.Method, req.URL, "res")

	body, _ := io.ReadAll(resp.Body)
	// Restore body for the caller.
	resp.Body = io.NopCloser(strings.NewReader(string(body)))

	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP/1.1 %s\n", resp.Status)
	resp.Header.Write(&sb) //nolint:errcheck
	sb.WriteString("\n")
	sb.Write(body)
	os.WriteFile(name, []byte(sb.String()), 0o644) //nolint:errcheck
}

func debugFilename(dir, method string, u *url.URL, suffix string) string {
	ts := time.Now().UTC().Format("20060102-150405")
	// Sanitise URL: replace / and ? with -
	slug := u.Host + u.Path
	slug = strings.NewReplacer("/", "-", "?", "-", "&", "-", "=", "-", ":", "").Replace(slug)
	name := fmt.Sprintf("%s-%s-%s.%s.txt", ts, method, slug, suffix)
	if len(name) > 200 {
		name = name[:196] + ".txt"
	}
	return filepath.Join(dir, name)
}
