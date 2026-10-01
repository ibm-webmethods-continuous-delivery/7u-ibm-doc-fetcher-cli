package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/cache"
	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/kb"
)

var buildKBFlags struct {
	indexOnly bool
	dryRun    bool
	product   string
}

var buildKBCmd = &cobra.Command{
	Use:   "build-kb",
	Short: "Export cache to agent-ready Markdown",
	Long: `Transforms all entries in cache/content/ into structured Markdown files
under kb/, applying YAML frontmatter and updating INDEX.md files. Idempotent.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := buildLogger(cfg.Verbose)
		contentDir := filepath.Join(cfg.DataDir, "cache", "content")

		type entry struct {
			productKey string
			topicID    string
			lang       string
			cachePath  string
		}

		// Walk cache/content/<product>/<topic>/<lang>.json
		var entries []entry
		err := filepath.WalkDir(contentDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(path, ".json") {
				return nil
			}
			rel, _ := filepath.Rel(contentDir, path)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			// parts: [product...] [topicID] [lang.json]
			if len(parts) < 3 {
				return nil
			}
			lang := strings.TrimSuffix(parts[len(parts)-1], ".json")
			topicID := parts[len(parts)-2]
			productKey := strings.Join(parts[:len(parts)-2], "/")

			if buildKBFlags.product != "" && productKey != buildKBFlags.product {
				return nil
			}
			if cfg.Lang != "" && cfg.Lang != "en" && lang != cfg.Lang {
				return nil
			}

			entries = append(entries, entry{productKey, topicID, lang, path})
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk cache: %w", err)
		}

		if buildKBFlags.indexOnly {
			logger.Info("--index-only: rebuilding indexes from existing kb/")
			return rebuildIndexes(cfg.DataDir)
		}

		// Process entries, group by product for stub merging.
		type product struct {
			topics []kb.Topic
		}
		products := map[string]*product{}

		written, skipped := 0, 0
		for _, e := range entries {
			type cacheEntry struct {
				Href       string `json:"href"`
				ProductKey string `json:"product_key"`
				HTML       string `json:"html"`
			}
			var cached cacheEntry
			if err := cache.LoadJSON(e.cachePath, &cached); err != nil {
				logger.Warn("cache load failed", "path", e.cachePath, "err", err)
				skipped++
				continue
			}

			// Use the authoritative productKey and topicID
			effectiveProductKey := cached.ProductKey
			if effectiveProductKey == "" {
				effectiveProductKey = e.productKey
			}
			effectiveTopicID := topicIDFromHref(cached.Href)
			if effectiveTopicID == "" {
				effectiveTopicID = e.topicID
			}

			var content string
			var md string
			var fm map[string]any
			var isStub bool

			cleanHref := cached.Href
			if q := strings.Index(cleanHref, "?"); q >= 0 {
				cleanHref = cleanHref[:q]
			}
			isStaticSpec := strings.HasSuffix(strings.ToLower(cleanHref), ".yaml") ||
				strings.HasSuffix(strings.ToLower(cleanHref), ".yml") ||
				strings.HasSuffix(strings.ToLower(cleanHref), ".json")

			if isStaticSpec {
				content = cached.HTML
				md = cached.HTML
				fm = map[string]any{
					"product": effectiveProductKey,
					"topic":   effectiveTopicID,
					"spec":    true,
				}
			} else {
				var lastUpdated string
				var err error
				md, lastUpdated, err = kb.ConvertHTML(cached.HTML)
				if err != nil {
					logger.Warn("HTML conversion failed", "topic", effectiveTopicID, "err", err)
					skipped++
					continue
				}

				isStub = kb.EffectiveLen(md) < kb.MinContentChars
				fm = kb.ExtractFrontmatter(md, effectiveProductKey, effectiveTopicID, lastUpdated)
				if isStub {
					fm["stub"] = true
				}

				content = kb.RenderFrontmatter(fm) + md
			}

			if !buildKBFlags.dryRun {
				if wErr := kb.WriteTopicFile(cfg.DataDir, effectiveProductKey, effectiveTopicID, e.lang, content); wErr != nil {
					logger.Warn("kb write failed", "err", wErr)
					skipped++
					continue
				}
			} else {
				fmt.Fprintf(os.Stderr, "  DRY  kb/%s/%s/%s.md\n", effectiveProductKey, effectiveTopicID, e.lang)
			}
			written++

			p := products[effectiveProductKey]
			if p == nil {
				p = &product{}
				products[effectiveProductKey] = p
			}
			p.topics = append(p.topics, kb.Topic{
				ProductKey:  effectiveProductKey,
				TopicID:     effectiveTopicID,
				Lang:        e.lang,
				Markdown:    md,
				Frontmatter: fm,
				IsStub:      isStub,
			})
		}

		// Write stub merged files and product indexes.
		productCounts := map[string]int{}
		for pk, p := range products {
			productCounts[pk] = len(p.topics)
			if !buildKBFlags.dryRun {
				if err := kb.WriteIndexMD(cfg.DataDir, pk, p.topics); err != nil {
					logger.Warn("index write failed", "product", pk, "err", err)
				}
				if err := writeStubsMerged(cfg.DataDir, pk, p.topics); err != nil {
					logger.Warn("stubs merged write failed", "product", pk, "err", err)
				}
			}
		}
		if !buildKBFlags.dryRun {
			if err := kb.WriteTopLevelIndex(cfg.DataDir, productCounts); err != nil {
				logger.Warn("top-level index write failed", "err", err)
			}
		}

		fmt.Fprintf(os.Stderr, "build-kb: %d topics written, %d skipped\n", written, skipped)
		return nil
	},
}

func init() {
	f := buildKBCmd.Flags()
	f.BoolVar(&buildKBFlags.indexOnly, "index-only", false,
		"Skip Markdown export; only rebuild INDEX.md files from existing kb/ content")
	f.BoolVar(&buildKBFlags.dryRun, "dry-run", false,
		"Print what would be written without writing any files")
	f.StringVar(&buildKBFlags.product, "product", "",
		"Limit processing to a specific product key")
	rootCmd.AddCommand(buildKBCmd)
}

func writeStubsMerged(dataDir, productKey string, topics []kb.Topic) error {
	var stubs []kb.Topic
	for _, t := range topics {
		if t.IsStub {
			stubs = append(stubs, t)
		}
	}
	if len(stubs) == 0 {
		return nil
	}

	fm := map[string]any{
		"product": productKey,
		"topic":   "_stubs_merged",
		"note":    fmt.Sprintf("Auto-merged %d navigation stub topics", len(stubs)),
	}
	var sections []string
	for _, s := range stubs {
		sections = append(sections, fmt.Sprintf("<!-- source: %s -->\n%s", s.TopicID, s.Markdown))
	}
	content := kb.RenderFrontmatter(fm) + strings.Join(sections, "\n\n---\n\n")

	dir := filepath.Join(dataDir, "kb", filepath.FromSlash(productKey))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "_stubs_merged.md"), []byte(content), 0o644)
}

func rebuildIndexes(dataDir string) error {
	kbDir := filepath.Join(dataDir, "kb")
	products := map[string][]kb.Topic{}

	err := filepath.WalkDir(kbDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, _ := filepath.Rel(kbDir, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 3 || parts[len(parts)-2] == "" {
			return nil
		}
		lang := strings.TrimSuffix(parts[len(parts)-1], ".md")
		topicID := parts[len(parts)-2]
		if topicID == "INDEX" || topicID == "_stubs_merged" {
			return nil
		}
		pk := strings.Join(parts[:len(parts)-2], "/")

		text, _ := os.ReadFile(path)
		isStub := strings.Contains(string(text), "stub: true")
		fm := map[string]any{}
		if strings.Contains(string(text), "http_methods:") {
			fm["http_methods"] = []string{}
		}

		products[pk] = append(products[pk], kb.Topic{
			ProductKey:  pk,
			TopicID:     topicID,
			Lang:        lang,
			Markdown:    string(text),
			Frontmatter: fm,
			IsStub:      isStub,
		})
		return nil
	})
	if err != nil {
		return err
	}

	counts := map[string]int{}
	for pk, topics := range products {
		counts[pk] = len(topics)
		if err := kb.WriteIndexMD(dataDir, pk, topics); err != nil {
			return err
		}
	}
	return kb.WriteTopLevelIndex(dataDir, counts)
}

// jsonEncode writes v as indented JSON to stdout.
func jsonEncode(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
