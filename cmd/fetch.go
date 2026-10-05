package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/config"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/fetcher"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/ibmdocs"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/kb"
)

var fetchFlags struct {
	recursive  bool
	depth      int
	refresh    bool
	jsonOutput bool
	quiet      bool
	maxTopics  int
}

var fetchCmd = &cobra.Command{
	Use:   "fetch <URL>",
	Short: "Fetch a single IBM Docs URL",
	Long: `Fetches the IBM documentation topic identified by an IBM Docs browser URL.

Without --recursive: fetches and caches the single topic, prints Markdown to stdout.
With    --recursive: walks the TOC tree up to --depth levels.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFetch(args[0], fetchFlags.recursive, fetchFlags.depth,
			fetchFlags.refresh, fetchFlags.jsonOutput, fetchFlags.quiet, fetchFlags.maxTopics)
	},
}

func init() {
	f := fetchCmd.Flags()
	f.BoolVarP(&fetchFlags.recursive, "recursive", "r", false, "Follow child topics in the TOC")
	f.IntVarP(&fetchFlags.depth, "depth", "D", config.DefaultDepth, "Maximum recursion depth")
	f.BoolVar(&fetchFlags.refresh, "refresh", false, "Re-fetch even if a valid cache entry exists")
	f.BoolVar(&fetchFlags.jsonOutput, "json", false, "Print a JSON summary to stdout")
	f.BoolVarP(&fetchFlags.quiet, "quiet", "q", false, "Suppress progress output")
	f.IntVar(&fetchFlags.maxTopics, "max-topics", config.DefaultMaxTopics,
		"Maximum child topics per TOC node (0 = unlimited)")
	rootCmd.AddCommand(fetchCmd)
}

func runFetch(rawURL string, recursive bool, depth int, refresh, jsonOut, quiet bool, maxTopics int) error {
	if cfg.HTTPDebug && quiet {
		return fmt.Errorf("--http-debug and --quiet cannot be used together (exit code 2)")
	}

	logger := buildLogger(cfg.Verbose)
	ibmdocs.ToolVersion = version

	debugDir := ""
	if cfg.HTTPDebug {
		debugDir = filepath.Join(cfg.DataDir, "debug")
	}

	requestLogPath := filepath.Join(cfg.DataDir, "requests.log")

	client := ibmdocs.New(cfg.CDNBaseURL, config.DefaultRequestTimeout, debugDir, requestLogPath, logger)
	w := fetcher.NewWalker(client, cfg.DataDir, cfg.Lang, maxTopics, cfg.Delay, cfg.CacheTTL, refresh, logger)

	maxDepth := 0
	if recursive {
		maxDepth = depth
	}

	type jsonResult struct {
		URL    string `json:"url"`
		Topics []struct {
			Href   string `json:"href"`
			Depth  int    `json:"depth"`
			Cached bool   `json:"cached"`
			Error  string `json:"error,omitempty"`
		} `json:"topics"`
		TopicsFetched int    `json:"topics_fetched"`
		TopicsFailed  int    `json:"topics_failed"`
		Status        string `json:"status"`
	}

	var results []fetcher.TopicResult
	fetched, failed := 0, 0

	for r := range w.Fetch(rawURL, maxDepth) {
		results = append(results, r)
		if r.Err != nil {
			failed++
			if !quiet {
				fmt.Fprintf(os.Stderr, "[ERROR] depth=%d href=%s: %v\n", r.Depth, r.Href, r.Err)
			}
		} else {
			fetched++
			if !quiet {
				cacheTag := ""
				if r.Cached {
					cacheTag = " (cached)"
				}
				fmt.Fprintf(os.Stderr, "  ✓ depth=%d %s%s\n", r.Depth, r.Href, cacheTag)
			}

			// Write to KB and print to stdout for single non-quiet fetch.
			topicID := topicIDFromHref(r.Href)
			var content string
			var md string
			if strings.HasSuffix(strings.ToLower(r.Href), ".yaml") || strings.HasSuffix(strings.ToLower(r.Href), ".yml") || strings.HasSuffix(strings.ToLower(r.Href), ".json") {
				// Static spec asset (OpenAPI, JSON schema, etc.)
				content = r.HTML
				md = r.HTML
			} else {
				var lastUpdated string
				var err error
				md, lastUpdated, err = kb.ConvertHTML(r.HTML)
				if err != nil {
					logger.Warn("HTML conversion failed", "href", r.Href, "err", err)
					continue
				}
				fm := kb.ExtractFrontmatter(md, r.ProductKey, topicID, lastUpdated)
				if kb.EffectiveLen(md) < config.MinContentChars {
					fm["stub"] = true
				}
				content = kb.RenderFrontmatter(fm) + md
			}

			if wErr := kb.WriteTopicFile(cfg.DataDir, r.ProductKey, topicID, r.Lang, content); wErr != nil {
				logger.Warn("kb write failed", "err", wErr)
			}

			if !recursive && !jsonOut && !quiet {
				fmt.Println(md)
			}
		}
	}

	status := "ok"
	if failed > 0 && fetched == 0 {
		status = "failed"
	} else if failed > 0 {
		status = "partial"
	}

	// Update index.json
	productKey, _, _ := fetcher.ParseIBMDocsURL(rawURL)
	if productKey != "" {
		idx, _ := fetcher.LoadIndex(cfg.DataDir)
		if idx == nil {
			idx = &fetcher.Index{Version: 1}
		}
		idx.Upsert(fetcher.IndexEntry{
			URL:           rawURL,
			ProductKey:    productKey,
			FetchedAt:     time.Now().UTC(),
			Depth:         depth,
			TopicsFetched: fetched,
			TopicsFailed:  failed,
			Status:        status,
		})
		if sErr := fetcher.SaveIndex(cfg.DataDir, idx); sErr != nil {
			logger.Warn("index save failed", "err", sErr)
		}
	}

	if jsonOut {
		jr := jsonResult{
			URL:           rawURL,
			TopicsFetched: fetched,
			TopicsFailed:  failed,
			Status:        status,
		}
		for _, r := range results {
			item := struct {
				Href   string `json:"href"`
				Depth  int    `json:"depth"`
				Cached bool   `json:"cached"`
				Error  string `json:"error,omitempty"`
			}{Href: r.Href, Depth: r.Depth, Cached: r.Cached}
			if r.Err != nil {
				item.Error = r.Err.Error()
			}
			jr.Topics = append(jr.Topics, item)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jr)
	}

	if !quiet {
		fmt.Fprintf(os.Stderr, "\nFetched %d topics, %d failed — status: %s\n", fetched, failed, status)
	}

	if failed > 0 {
		// Exit code 1: partial or total failure. Use a sentinel so Cobra does
		// not print usage, but Execute() can still call os.Exit with the right code.
		return errPartialFailure
	}
	return nil
}

// topicIDFromHref extracts a clean topic identifier from an IBM Docs href.
// Strips the ?cp= query param, takes the basename, removes .html.
func topicIDFromHref(href string) string {
	clean := href
	if q := strings.IndexByte(clean, '?'); q >= 0 {
		clean = clean[:q]
	}
	if i := strings.LastIndexByte(clean, '/'); i >= 0 {
		clean = clean[i+1:]
	}
	return strings.TrimSuffix(clean, ".html")
}

// buildLogger returns a slog.Logger at the appropriate level.
func buildLogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
