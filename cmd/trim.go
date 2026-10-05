package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/kb"
)

var trimFlags struct {
	maxKBSize  int64
	stubsOnly  bool
	product    string
	dryRun     bool
	jsonOutput bool
}

var trimCmd = &cobra.Command{
	Use:   "trim",
	Short: "Trim KB to fit agent context constraints",
	Long: `Removes low-value content from kb/ to keep total Markdown size within a
target budget. Never modifies cache/. Run build-kb to restore.

Trim order: stubs first, then oldest, then smallest substantive topics.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if trimFlags.maxKBSize == 0 && !trimFlags.stubsOnly && trimFlags.product == "" {
			return fmt.Errorf("provide --max-kb-size, --stubs-only, or --product to limit scope")
		}

		kbDir := filepath.Join(cfg.DataDir, "kb")

		type mdFile struct {
			path    string
			product string
			size    int64
			isStub  bool
		}

		var files []mdFile
		err := filepath.WalkDir(kbDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(path, ".md") {
				return err
			}
			rel, _ := filepath.Rel(kbDir, path)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) < 2 {
				return nil
			}
			// Skip INDEX.md and _stubs_merged.md
			base := parts[len(parts)-1]
			if base == "INDEX.md" || base == "_stubs_merged.md" {
				return nil
			}
			pk := strings.Join(parts[:len(parts)-2], "/")
			if trimFlags.product != "" && pk != trimFlags.product {
				return nil
			}
			info, _ := d.Info()
			text, _ := os.ReadFile(path)
			isStub := strings.Contains(string(text), "stub: true")
			files = append(files, mdFile{path, pk, info.Size(), isStub})
			return nil
		})
		if err != nil {
			return err
		}

		// Trim order: stubs → oldest (by mtime) → smallest.
		sort.SliceStable(files, func(i, j int) bool {
			if files[i].isStub != files[j].isStub {
				return files[i].isStub // stubs first
			}
			return files[i].size < files[j].size
		})

		type removed struct {
			Path   string `json:"path"`
			Bytes  int64  `json:"bytes"`
			IsStub bool   `json:"stub"`
		}
		var removedFiles []removed
		var totalSize int64
		for _, f := range files {
			info, _ := os.Stat(f.path)
			if info != nil {
				totalSize += info.Size()
			}
		}

		budgetBytes := trimFlags.maxKBSize * 1024
		freedBytes := int64(0)

		for _, f := range files {
			if trimFlags.stubsOnly && !f.isStub {
				continue
			}
			if budgetBytes > 0 && totalSize-freedBytes <= budgetBytes {
				break
			}
			removedFiles = append(removedFiles, removed{f.path, f.size, f.isStub})
			freedBytes += f.size
			if !trimFlags.dryRun {
				os.Remove(f.path) //nolint:errcheck
			} else {
				fmt.Fprintf(os.Stderr, "  DRY REMOVE %s (%d bytes)\n", f.path, f.size)
			}
		}

		if trimFlags.jsonOutput {
			return jsonEncode(map[string]any{
				"removed":     removedFiles,
				"freed_bytes": freedBytes,
				"dry_run":     trimFlags.dryRun,
			})
		}

		action := "Removed"
		if trimFlags.dryRun {
			action = "Would remove"
		}
		fmt.Fprintf(os.Stderr, "%s %d files, freeing %d KB\n",
			action, len(removedFiles), freedBytes/1024)

		if trimFlags.maxKBSize > 0 && !trimFlags.dryRun {
			remaining := totalSize - freedBytes
			if remaining > budgetBytes {
				fmt.Fprintf(os.Stderr,
					"NOTE: %d KB remaining still exceeds --max-kb-size %d KB\n",
					remaining/1024, trimFlags.maxKBSize)
				fmt.Fprintln(os.Stderr,
					"Run with --dry-run to preview before destructive trim, or lower --max-kb-size")
			}
		}

		_ = kb.MinContentChars // ensure kb imported
		return nil
	},
}

func init() {
	f := trimCmd.Flags()
	f.Int64Var(&trimFlags.maxKBSize, "max-kb-size", 0,
		"Target maximum total size of kb/ in kilobytes (0 = no budget)")
	f.BoolVar(&trimFlags.stubsOnly, "stubs-only", false,
		"Remove only stub topics")
	f.StringVar(&trimFlags.product, "product", "",
		"Restrict trimming to a single product key")
	f.BoolVar(&trimFlags.dryRun, "dry-run", false,
		"Print what would be removed without deleting anything")
	f.BoolVar(&trimFlags.jsonOutput, "json", false,
		"Output a JSON summary of removed files and bytes freed")
	rootCmd.AddCommand(trimCmd)
}
