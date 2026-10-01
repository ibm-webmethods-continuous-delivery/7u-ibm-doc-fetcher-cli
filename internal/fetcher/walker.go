package fetcher

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/cache"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/ibmdocs"
)

// TopicResult holds the outcome of fetching a single topic.
type TopicResult struct {
	URL        string
	ProductKey string
	Href       string
	Lang       string
	Depth      int
	HTML       string // raw HTML fragment from Content API
	CacheFile  string // path written to cache
	Cached     bool   // true if served from cache
	Err        error
}

// Walker orchestrates recursive TOC fetching for a single product.
type Walker struct {
	client    *ibmdocs.Client
	dataDir   string
	lang      string
	maxTopics int
	delay     time.Duration
	refresh   bool
	logger    *slog.Logger
	visited   map[string]bool
}

// NewWalker creates a Walker.
func NewWalker(
	client *ibmdocs.Client,
	dataDir, lang string,
	maxTopics int,
	delay time.Duration,
	refresh bool,
	logger *slog.Logger,
) *Walker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Walker{
		client:    client,
		dataDir:   dataDir,
		lang:      lang,
		maxTopics: maxTopics,
		delay:     delay,
		refresh:   refresh,
		logger:    logger,
		visited:   make(map[string]bool),
	}
}

// Fetch fetches a single IBM Docs URL, optionally recursing to depth.
// Results (one per topic) are sent on the returned channel; close signals done.
func (w *Walker) Fetch(rawURL string, maxDepth int) <-chan TopicResult {
	ch := make(chan TopicResult, 64)
	go func() {
		defer close(ch)
		w.visited = make(map[string]bool)

		productKey, topicSlug, lang := ParseIBMDocsURL(rawURL)
		if productKey == "" {
			ch <- TopicResult{URL: rawURL, Err: fmt.Errorf("cannot parse IBM Docs URL: %s", rawURL)}
			return
		}
		// lang from URL overrides the walker's lang if explicitly set in URL
		if lang != "" && lang != "en" {
			w.lang = lang
		}

		toc, err := w.fetchTOC(productKey)
		if err != nil {
			ch <- TopicResult{URL: rawURL, ProductKey: productKey, Err: err}
			return
		}

		// Find starting node
		var startNode *ibmdocs.TOCNode
		if topicSlug != "" {
			startNode = findInTOC(&toc.TOC, topicSlug)
			if startNode == nil {
				w.logger.Warn("topic slug not found in TOC, falling back to TOC root",
					"slug", topicSlug, "product", productKey)
			}
		}
		if startNode == nil {
			startNode = &toc.TOC
		}

		w.walk(startNode, rawURL, productKey, 0, maxDepth, ch)
	}()
	return ch
}

// fetchTOC retrieves the TOC from cache or network.
func (w *Walker) fetchTOC(productKey string) (*ibmdocs.TOCResponse, error) {
	cachePath := cache.KeyTOC(w.dataDir, productKey, w.lang)
	if !w.refresh && cache.Valid(cachePath) {
		w.logger.Debug("TOC cache hit", "product", productKey)
		var toc ibmdocs.TOCResponse
		if err := cache.LoadJSON(cachePath, &toc); err == nil {
			return &toc, nil
		}
	}
	w.logger.Info("fetching TOC", "product", productKey)
	toc, err := w.client.FetchTOC(productKey, w.lang)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, fmt.Errorf("%w (product key '%s' may be an umbrella collection or invalid; try 'ibmdocs search <keyword>' to locate concrete product leaf URLs)", err, productKey)
		}
		return nil, err
	}
	if err := cache.SaveJSON(cachePath, toc); err != nil {
		w.logger.Warn("TOC cache write failed", "err", err)
	}
	return toc, nil
}

// walk recursively fetches a TOC node and its children.
func (w *Walker) walk(
	node *ibmdocs.TOCNode,
	nodeURL, productKey string,
	depth, maxDepth int,
	ch chan<- TopicResult,
) {
	href := node.Href

	// Skip empty-href navigation containers (§11: would 400 on Content API).
	if strings.TrimSpace(href) == "" {
		w.logger.Debug("skipping empty-href node", "label", node.Label)
		// Still recurse into children if within depth.
		if depth < maxDepth {
			w.recurseChildren(node, nodeURL, productKey, depth, maxDepth, ch)
		}
		return
	}

	// Detect root-only ID hrefs (no slash, no .html) and fall back to first child.
	if isRootIDHref(href) {
		w.logger.Debug("root href is bare product ID, falling back to first content child",
			"href", href)
		firstContent := firstContentChild(node)
		if firstContent != nil {
			w.walk(firstContent, nodeURL, productKey, depth, maxDepth, ch)
		} else {
			w.logger.Warn("no content child found under root node", "product", productKey)
		}
		return
	}

	// Dedup by href.
	if w.visited[href] {
		return
	}
	w.visited[href] = true

	result := w.fetchContent(node, nodeURL, productKey, depth)
	ch <- result

	if result.Err == nil && depth < maxDepth {
		// Apply inter-request delay before descending.
		if w.delay > 0 {
			time.Sleep(w.delay)
		}
		w.recurseChildren(node, nodeURL, productKey, depth, maxDepth, ch)
	}
}

func (w *Walker) recurseChildren(
	node *ibmdocs.TOCNode,
	parentURL, productKey string,
	depth, maxDepth int,
	ch chan<- TopicResult,
) {
	children := node.Topics
	if w.maxTopics > 0 && len(children) > w.maxTopics {
		children = children[:w.maxTopics]
	}
	for i := range children {
		child := &children[i]
		childURL := buildChildURL(productKey, child.Href, w.lang)
		w.walk(child, childURL, productKey, depth+1, maxDepth, ch)
		if w.delay > 0 {
			time.Sleep(w.delay)
		}
	}
}

func (w *Walker) fetchContent(
	node *ibmdocs.TOCNode,
	nodeURL, productKey string,
	depth int,
) TopicResult {
	href := node.Href
	cachePath := cache.KeyContent(w.dataDir, productKey, href, w.lang)

	base := TopicResult{
		URL:        nodeURL,
		ProductKey: productKey,
		Href:       href,
		Lang:       w.lang,
		Depth:      depth,
		CacheFile:  cachePath,
	}

	type cacheEntry struct {
		Href       string `json:"href"`
		ProductKey string `json:"product_key"`
		HTML       string `json:"html"`
	}

	if !w.refresh && cache.Valid(cachePath) {
		var entry cacheEntry
		if err := cache.LoadJSON(cachePath, &entry); err == nil {
			w.logger.Debug("content cache hit", "href", href)
			base.HTML = entry.HTML
			base.Cached = true
			return base
		}
	}

	w.logger.Info("fetching content", "href", href, "depth", depth)
	html, err := w.client.FetchContent(href, w.lang)
	if err != nil {
		base.Err = err
		return base
	}

	entry := cacheEntry{Href: href, ProductKey: productKey, HTML: html}
	if err := cache.SaveJSON(cachePath, entry); err != nil {
		w.logger.Warn("content cache write failed", "err", err)
	}

	base.HTML = html
	return base
}

// --------------------------------------------------------------------------
// TOC search helpers
// --------------------------------------------------------------------------

// findInTOC does a BFS search for a node matching slug.
func findInTOC(root *ibmdocs.TOCNode, slug string) *ibmdocs.TOCNode {
	queue := []*ibmdocs.TOCNode{root}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if SlugMatches(slug, node.Href, node.Label) {
			return node
		}
		for i := range node.Topics {
			queue = append(queue, &node.Topics[i])
		}
	}
	return nil
}

// isRootIDHref detects bare internal product ID hrefs like "SSW0JQG_3.0.x"
// that have no path separator and no .html extension.
func isRootIDHref(href string) bool {
	return !strings.Contains(href, "/") && !strings.HasSuffix(href, ".html")
}

// firstContentChild returns the first child that has a real content href
// (skips dummy_landing_page_id and empty hrefs).
func firstContentChild(node *ibmdocs.TOCNode) *ibmdocs.TOCNode {
	for i := range node.Topics {
		h := node.Topics[i].Href
		if strings.TrimSpace(h) == "" {
			continue
		}
		if strings.Contains(h, "dummy_landing_page_id") {
			continue
		}
		return &node.Topics[i]
	}
	return nil
}

// buildChildURL constructs a browser URL for a child topic.
func buildChildURL(productKey, href, lang string) string {
	// Extract slug from href basename (strip .html and ?cp=... )
	clean := href
	if idx := strings.Index(clean, "?"); idx >= 0 {
		clean = clean[:idx]
	}
	base := filepath_Base(clean)
	slug := strings.TrimSuffix(base, ".html")
	slug = strings.ReplaceAll(slug, "_", "-")
	langSeg := lang
	if langSeg == "" {
		langSeg = "en"
	}
	return "https://www.ibm.com/docs/" + langSeg + "/" + productKey + "?topic=" + slug
}
