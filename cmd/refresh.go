package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/fetcher"
)

var refreshFlags struct {
	olderThan  int
	depth      int
	maxTopics  int
	dryRun     bool
	jsonOutput bool
}

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-fetch stale cache entries",
	Long: `Scans index.json and re-fetches all entries whose fetched_at timestamp
is older than --older-than days.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := fetcher.LoadIndex(cfg.DataDir)
		if err != nil {
			return err
		}

		threshold := time.Duration(refreshFlags.olderThan) * 24 * time.Hour
		var toRefresh []fetcher.IndexEntry
		for _, e := range idx.Entries {
			if time.Since(e.FetchedAt) > threshold {
				toRefresh = append(toRefresh, e)
			}
		}

		if len(toRefresh) == 0 {
			fmt.Fprintf(os.Stderr, "no entries older than %d days\n", refreshFlags.olderThan)
			return nil
		}

		fmt.Fprintf(os.Stderr, "%d entries to refresh\n", len(toRefresh))

		for i, e := range toRefresh {
			depth := e.Depth
			if refreshFlags.depth > 0 {
				depth = refreshFlags.depth
			}
			if refreshFlags.dryRun {
				fmt.Fprintf(os.Stderr, "  DRY [%d/%d] %s (depth=%d)\n", i+1, len(toRefresh), e.URL, depth)
				continue
			}
			fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", i+1, len(toRefresh), e.URL)
			if rErr := runFetch(e.URL, true, depth, true, refreshFlags.jsonOutput, false, refreshFlags.maxTopics); rErr != nil {
				fmt.Fprintf(os.Stderr, "    [ERROR] %v\n", rErr)
			}
		}
		return nil
	},
}

func init() {
	f := refreshCmd.Flags()
	f.IntVarP(&refreshFlags.olderThan, "older-than", "o", 7, "Re-fetch entries older than N days")
	f.IntVarP(&refreshFlags.depth, "depth", "D", 0, "Override stored recursion depth (0 = use stored)")
	f.IntVar(&refreshFlags.maxTopics, "max-topics", 0,
		"Maximum child topics per TOC node (0 = unlimited)")
	f.BoolVar(&refreshFlags.dryRun, "dry-run", false, "Print what would be re-fetched without making network requests")
	f.BoolVar(&refreshFlags.jsonOutput, "json", false, "Print JSON summary of refresh results")
	rootCmd.AddCommand(refreshCmd)
}
