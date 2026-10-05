package cmd

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/config"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/ibmdocs"
)

var searchFlags struct {
	limit      int
	all        bool
	latestOnly bool
	jsonOutput bool
	product    string
	urlOnly    bool
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search IBM Docs by keyword",
	Long: `Performs a full-text search across IBM product documentation using the
IBM Docs Search API (1.www.s81c.com — no auth required).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		logger := buildLogger(cfg.Verbose)
		ibmdocs.ToolVersion = version

		debugDir := ""
		if cfg.HTTPDebug {
			debugDir = filepath.Join(cfg.DataDir, "debug")
		}

		requestLogPath := filepath.Join(cfg.DataDir, "requests.log")

		client := ibmdocs.New(cfg.CDNBaseURL, config.DefaultRequestTimeout, debugDir, requestLogPath, logger)

		var allHits []ibmdocs.SearchHit
		start := 0
		for {
			resp, err := client.Search(query, cfg.Lang, searchFlags.limit, start)
			if err != nil {
				return fmt.Errorf("search failed: %w", err)
			}
			allHits = append(allHits, resp.Items...)
			if !searchFlags.all || resp.Next < 0 || resp.Next >= resp.Hits || len(resp.Items) == 0 {
				break
			}
			start = resp.Next
		}

		// Filter by product if requested.
		if searchFlags.product != "" {
			filtered := allHits[:0]
			for _, h := range allHits {
				if strings.Contains(h.FullURL, "/docs/en/"+searchFlags.product) {
					filtered = append(filtered, h)
				}
			}
			allHits = filtered
		}

		// Deduplicate cross-version results.
		if searchFlags.latestOnly {
			allHits = deduplicateLatest(allHits)
		}

		if searchFlags.jsonOutput {
			return jsonEncode(allHits)
		}

		if searchFlags.urlOnly {
			for _, h := range allHits {
				fmt.Println(h.FullURL)
			}
			return nil
		}

		for i, h := range allHits {
			title := tagRE.ReplaceAllString(html.UnescapeString(h.Title), "")
			snippet := tagRE.ReplaceAllString(html.UnescapeString(h.Snippet), "")
			fmt.Printf("%d. %s\n   %s\n   %s\n\n", i+1, title, h.FullURL, truncateStr(snippet, 120))
		}

		if !searchFlags.urlOnly && !searchFlags.jsonOutput {
			fmt.Fprintf(os.Stderr, "\n%d results\n", len(allHits))
		}
		return nil
	},
}

func init() {
	f := searchCmd.Flags()
	f.IntVarP(&searchFlags.limit, "limit", "n", 10, "Maximum results per page")
	f.BoolVar(&searchFlags.all, "all", false, "Paginate through all results")
	f.BoolVar(&searchFlags.latestOnly, "latest-only", false,
		"Keep only the most recent version per unique title per product")
	f.BoolVar(&searchFlags.jsonOutput, "json", false, "Output as JSON array")
	f.StringVar(&searchFlags.product, "product", "", "Filter results to a specific product key")
	f.BoolVar(&searchFlags.urlOnly, "url-only", false, "Print only fullurl, one per line")
	rootCmd.AddCommand(searchCmd)
}

// deduplicateLatest keeps only the highest-versioned result per unique title
// per product family. Version is assumed to appear in productBreadCrumb.
func deduplicateLatest(hits []ibmdocs.SearchHit) []ibmdocs.SearchHit {
	type key struct{ product, title string }
	best := map[key]ibmdocs.SearchHit{}
	// Preserve encounter order: later duplicate replaces earlier (sort ensures highest version wins).
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].ProductBreadCrumb < hits[j].ProductBreadCrumb
	})
	for _, h := range hits {
		title := tagRE.ReplaceAllString(h.Title, "")
		k := key{h.Product.Key, title}
		if _, ok := best[k]; !ok {
			best[k] = h
		} else if h.ProductBreadCrumb > best[k].ProductBreadCrumb {
			best[k] = h
		}
	}
	// Rebuild in original relative order.
	out := make([]ibmdocs.SearchHit, 0, len(best))
	seen := map[key]bool{}
	for _, h := range hits {
		title := tagRE.ReplaceAllString(h.Title, "")
		k := key{h.Product.Key, title}
		if v, ok := best[k]; ok && !seen[k] && v.FullURL == h.FullURL {
			out = append(out, h)
			seen[k] = true
		}
	}
	return out
}
