// Package ibmdocs provides a client for the IBM Docs public CDN API.
//
// Three unauthenticated endpoints on 1.www.s81c.com (default):
//
//	TOC:     GET /docs/api/v1/toc/{product_key}?lang={lang}
//	Content: GET /docs/api/v1/content/{href}?parsebody=true&lang={lang}
//	Search:  GET /docs/api/v1/search?query={q}&lang={lang}&limit={n}&start={s}
package ibmdocs

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// maxRateLimitRetries is how many times a request answered with HTTP 429
	// is retried before giving up.
	maxRateLimitRetries = 3
	// defaultRetryBase is the first backoff step (doubles each retry) used
	// when the server sends no Retry-After header.
	defaultRetryBase = 2 * time.Second
	// maxRetryWait caps any single wait, including a server Retry-After.
	maxRetryWait = 30 * time.Second
	// minRatchetDelay is the minimum inter-request delay applied when ratcheting up after a 429.
	minRatchetDelay = 1000 * time.Millisecond
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
	Href              string          `json:"href"`
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
	baseURL        string
	httpClient     *http.Client
	logger         *slog.Logger
	debugDir       string // non-empty when --http-debug is active
	requestLogPath string // path to requests.log file; empty disables request logging

	retryBase   time.Duration // first 429 backoff step
	rateRetries atomic.Int64  // 429 responses that were retried
	rateFailed  atomic.Int64  // requests that still got 429 after all retries

	mu           sync.Mutex
	delay        time.Duration   // base / ratcheted inter-request delay
	lastRequest  time.Time       // timestamp when last request was dispatched
	searchMisses map[string]bool // memoized search misses (negative cache)
}

// RateLimitStats reports how often IBM Docs answered HTTP 429 during this
// client's lifetime: retried = 429s that were retried, failed = requests that
// were still throttled after all retries (their data is missing).
func (c *Client) RateLimitStats() (retried, failed int64) {
	return c.rateRetries.Load(), c.rateFailed.Load()
}

// CurrentDelay returns the client's active inter-request delay.
func (c *Client) CurrentDelay() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.delay
}

// BuildTransport constructs an *http.Transport configured for the given proxy
// URL and optional extra CA bundle.
//
//   - proxyURL: explicit proxy, e.g. "http://proxy.corp:3128". Empty means
//     fall back to the standard HTTP_PROXY / HTTPS_PROXY environment variables.
//   - tlsCABundle: path to a PEM file containing one or more extra CA
//     certificates to append to the system pool. Needed when a TLS-inspecting
//     proxy presents a certificate signed by a private CA that is not already
//     in /etc/ssl/certs/. Pass "" to use only the system pool.
//
// Returns a transport with the requested settings, or an error if the proxy
// URL cannot be parsed or the CA bundle cannot be read.
func BuildTransport(proxyURL, tlsCABundle string) (*http.Transport, error) {
	// Start from a clone of the default transport so we inherit all tuning
	// (keep-alives, idle connections, etc.) rather than getting bare defaults.
	t := http.DefaultTransport.(*http.Transport).Clone()

	// ── Proxy ────────────────────────────────────────────────────────────────
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("--proxy: invalid URL %q: %w", proxyURL, err)
		}
		t.Proxy = http.ProxyURL(u)
	}
	// When proxyURL is empty, t.Proxy stays as ProxyFromEnvironment (the
	// default), so HTTP_PROXY / HTTPS_PROXY are honoured automatically.

	// ── TLS CA bundle ────────────────────────────────────────────────────────
	if tlsCABundle != "" {
		pemData, err := os.ReadFile(tlsCABundle)
		if err != nil {
			return nil, fmt.Errorf("--tls-ca-bundle: cannot read %q: %w", tlsCABundle, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			// SystemCertPool can fail on some platforms; start with an empty pool.
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("--tls-ca-bundle: %q contains no valid PEM certificates", tlsCABundle)
		}
		if t.TLSClientConfig == nil {
			t.TLSClientConfig = &tls.Config{}
		}
		t.TLSClientConfig.RootCAs = pool
	}

	return t, nil
}

// ValidateTransportConfig parses the proxy URL and checks that the CA bundle
// file exists and contains at least one certificate. It is called during
// PersistentPreRunE so the user gets a clear error before any network activity.
func ValidateTransportConfig(proxyURL, tlsCABundle string) error {
	_, err := BuildTransport(proxyURL, tlsCABundle)
	return err
}

// New creates a Client.
//
//   - baseURL: CDN base, e.g. "https://1.www.s81c.com"
//   - timeout: per-request timeout
//   - delay: inter-request pacing delay (e.g. 200ms)
//   - transport: custom HTTP transport (nil → http.DefaultTransport).
//     Build one with BuildTransport to configure a proxy and/or extra CA certs.
//   - debugDir: directory for HTTP debug dumps; empty string disables dumps
//   - requestLogPath: path to requests.log file; empty string disables request logging
//   - logger: structured logger (pass slog.Default() if none)
func New(baseURL string, timeout, delay time.Duration, transport http.RoundTripper, debugDir, requestLogPath string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	hc := &http.Client{Timeout: timeout}
	if transport != nil {
		hc.Transport = transport
	}
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		httpClient:     hc,
		logger:         logger,
		debugDir:       debugDir,
		requestLogPath: requestLogPath,
		retryBase:      defaultRetryBase,
		delay:          delay,
		searchMisses:   make(map[string]bool),
	}
}

// pace enforces the centralized inter-request delay across all outbound calls.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.delay > 0 && !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < c.delay {
			time.Sleep(c.delay - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// ratchetDelay ratchets up the session's base inter-request delay upon HTTP 429.
func (c *Client) ratchetDelay() {
	c.mu.Lock()
	defer c.mu.Unlock()

	newDelay := c.delay * 2
	if newDelay < minRatchetDelay {
		newDelay = minRatchetDelay
	}
	if newDelay > c.delay {
		c.logger.Warn("IBM Docs rate limit (HTTP 429) — ratcheting up base inter-request delay",
			"previous_delay", c.delay.String(),
			"new_delay", newDelay.String())
		c.delay = newDelay
	}
}

func (c *Client) hasSearchMiss(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.searchMisses[key]
}

func (c *Client) markSearchMiss(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.searchMisses[key] = true
}

// retryWait returns how long to wait before retry number attempt (0-based):
// the server's Retry-After (seconds) when present, else exponential backoff.
func (c *Client) retryWait(resp *http.Response, attempt int) time.Duration {
	wait := c.retryBase << attempt
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && secs >= 0 {
			wait = time.Duration(secs) * time.Second
		}
	}
	if wait > maxRetryWait {
		wait = maxRetryWait
	}
	return wait
}

// userAgent returns the tool's User-Agent header value.
// Injected at link time via ldflags from cmd package.
var ToolVersion = "dev"

func userAgent() string {
	return "Mozilla/5.0 (compatible; ibmdocs/" + ToolVersion + ")"
}

func (c *Client) do(req *http.Request, command, productKey, topicHref string) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Referer", "https://www.ibm.com/docs/")
	req.Header.Set("Accept", "*/*")

	// Log the request URL to requests.log before sending
	if c.requestLogPath != "" {
		c.logRequest(req, command, productKey, topicHref)
	}

	if c.debugDir != "" {
		dumpRequest(c.debugDir, req)
	}

	for attempt := 0; ; attempt++ {
		c.pace()

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if c.debugDir != "" {
			dumpResponse(c.debugDir, req, resp)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		c.ratchetDelay()
		if attempt >= maxRateLimitRetries {
			c.rateFailed.Add(1)
			c.logger.Error("IBM Docs rate limit (HTTP 429) persists after retries — data for this request is MISSING",
				"url", req.URL.String(), "retries", maxRateLimitRetries)
			return resp, nil // caller turns the 429 into an *APIError
		}
		wait := c.retryWait(resp, attempt)
		_ = resp.Body.Close()
		c.rateRetries.Add(1)
		c.logger.Warn("IBM Docs rate limit (HTTP 429) — backing off",
			"wait", wait.String(), "attempt", attempt+1, "of", maxRateLimitRetries)
		time.Sleep(wait)
	}
}

// logRequest writes a JSON Lines entry to the request log file.
func (c *Client) logRequest(req *http.Request, command, productKey, topicHref string) {
	entry := map[string]string{
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"method":      req.Method,
		"url":         req.URL.String(),
		"command":     command,
		"product_key": productKey,
	}
	if topicHref != "" {
		entry["topic_href"] = topicHref
	}

	data, err := json.Marshal(entry)
	if err != nil {
		c.logger.Warn("failed to marshal request log entry", "err", err)
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(c.requestLogPath), 0o755); err != nil {
		c.logger.Warn("failed to create request log directory", "err", err)
		return
	}

	// Append to log file (opened with O_APPEND | O_CREATE | O_WRONLY)
	f, err := os.OpenFile(c.requestLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		c.logger.Warn("failed to open request log file", "err", err)
		return
	}
	defer f.Close() //nolint:errcheck

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		c.logger.Warn("failed to write request log entry", "err", err)
	}
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
	resp, err := c.do(req, "fetch", productKey, "")
	if err != nil {
		return nil, fmt.Errorf("FetchTOC %s: %w", productKey, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response already consumed/discarded below

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
	cleanHref := href
	if idx := strings.Index(cleanHref, "?"); idx >= 0 {
		cleanHref = cleanHref[:idx]
	}
	isStaticSpec := strings.HasSuffix(strings.ToLower(cleanHref), ".yaml") ||
		strings.HasSuffix(strings.ToLower(cleanHref), ".yml") ||
		strings.HasSuffix(strings.ToLower(cleanHref), ".json")

	if isStaticSpec {
		rawURL = fmt.Sprintf("%s/docs/api/v1/content/%s?lang=%s", c.baseURL, href, lang)
	} else if idx := strings.Index(href, "?"); idx >= 0 {
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
	resp, err := c.do(req, "fetch", "", href)
	if err != nil {
		return "", fmt.Errorf("FetchContent %s: %w", href, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response already consumed/discarded below

	if resp.StatusCode != http.StatusOK {
		return "", &APIError{URL: rawURL, StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("FetchContent read %s: %w", href, err)
	}
	return string(body), nil
}

// IsStaticAsset reports whether href is a spec, image, or binary asset rather than a doc page.
func IsStaticAsset(href string) bool {
	clean := strings.ToLower(href)
	if idx := strings.Index(clean, "?"); idx >= 0 {
		clean = clean[:idx]
	}
	for _, ext := range []string{".yaml", ".yml", ".json", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".pdf", ".zip"} {
		if strings.HasSuffix(clean, ext) {
			return true
		}
	}
	return false
}

// ResolveHrefBySlug maps a real ibm.com ?topic= slug back to the TOC href it
// belongs to. Deterministic: the scoped search is queried with the slug words
// and only a hit whose own fullurl carries exactly that slug is accepted.
// Returns "" when no hit matches.
func (c *Client) ResolveHrefBySlug(slug, lang, productID string) (string, error) {
	if slug == "" || productID == "" {
		return "", nil
	}
	missKey := "slug:" + productID + ":" + slug + ":" + lang
	if c.hasSearchMiss(missKey) {
		return "", nil
	}

	query := strings.ReplaceAll(slug, "-", " ")
	for start, page := 0, 0; page < 3; page++ {
		resp, err := c.SearchInProduct(query, lang, productID, 50, start)
		if err != nil {
			return "", err
		}
		for _, h := range resp.Items {
			u, perr := url.Parse(h.FullURL)
			if perr == nil && u.Query().Get("topic") == slug {
				href, _, _ := strings.Cut(h.Href, "?")
				return href, nil
			}
		}
		if resp.Next < 0 || resp.Next >= resp.Hits || len(resp.Items) == 0 {
			break
		}
		start = resp.Next
	}
	c.markSearchMiss(missKey)
	return "", nil
}

// Search executes a keyword search and returns one page of results.
func (c *Client) Search(query, lang string, limit, start int) (*SearchResponse, error) {
	return c.search(query, lang, "", limit, start)
}

// SearchInProduct is Search scoped to one IBM-internal product ID
// (the first path segment of a TOC href, e.g. "SSGOVO" or "SSFQ7G1_12.1.0").
func (c *Client) SearchInProduct(query, lang, productID string, limit, start int) (*SearchResponse, error) {
	return c.search(query, lang, productID, limit, start)
}

// ResolveSourceURL returns the canonical ibm.com browser URL of the topic
// identified by href. IBM's ?topic= slug is NOT derivable from the href, so
// it is looked up deterministically: search the product scope for the topic
// title and accept only the hit whose own href equals the requested one
// (ignoring any ?cp= suffix). Returns "" when no hit matches exactly.
func (c *Client) ResolveSourceURL(href, title, lang string) (string, error) {
	if IsStaticAsset(href) {
		return "", nil
	}
	clean := href
	if i := strings.Index(clean, "?"); i >= 0 {
		clean = clean[:i]
	}
	productID, _, ok := strings.Cut(clean, "/")
	if !ok || productID == "" || title == "" {
		return "", nil
	}

	missKey := "source_url:" + clean + ":" + lang
	if c.hasSearchMiss(missKey) {
		return "", nil
	}

	const pageSize = 50
	for start, page := 0, 0; page < 3; page++ {
		resp, err := c.SearchInProduct(title, lang, productID, pageSize, start)
		if err != nil {
			return "", err
		}
		for _, h := range resp.Items {
			hh, _, _ := strings.Cut(h.Href, "?")
			if hh == clean {
				return h.FullURL, nil
			}
		}
		if resp.Next < 0 || resp.Next >= resp.Hits || len(resp.Items) == 0 {
			break
		}
		start = resp.Next
	}
	c.markSearchMiss(missKey)
	return "", nil
}

func (c *Client) search(query, lang, productID string, limit, start int) (*SearchResponse, error) {
	rawURL := fmt.Sprintf("%s/docs/api/v1/search?query=%s&lang=%s&limit=%d&start=%d",
		c.baseURL,
		url.QueryEscape(query),
		url.QueryEscape(lang),
		limit,
		start,
	)
	if productID != "" {
		rawURL += "&products=" + url.QueryEscape(productID)
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Search build request: %w", err)
	}

	c.logger.Debug("search", "query", query, "lang", lang, "limit", limit, "start", start)
	resp, err := c.do(req, "search", "", "")
	if err != nil {
		return nil, fmt.Errorf("Search %q: %w", query, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response already consumed/discarded below

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
	if e.StatusCode == http.StatusTooManyRequests {
		return msg + " — RATE LIMITED; wait a few minutes and re-run, or raise --delay (e.g. --delay 1s)"
	}
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
