package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/fetcher"
)

var listFlags struct {
	jsonOutput bool
	stale      bool
	olderThan  int
	product    string
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached entries from index.json",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := fetcher.LoadIndex(cfg.DataDir)
		if err != nil {
			return err
		}

		threshold := time.Duration(listFlags.olderThan) * 24 * time.Hour

		type row struct {
			URL           string    `json:"url"`
			ProductKey    string    `json:"product_key"`
			FetchedAt     time.Time `json:"fetched_at"`
			Depth         int       `json:"depth"`
			TopicsFetched int       `json:"topics_fetched"`
			Status        string    `json:"status"`
			Stale         bool      `json:"stale"`
		}
		var rows []row
		for _, e := range idx.Entries {
			if listFlags.product != "" && e.ProductKey != listFlags.product {
				continue
			}
			stale := time.Since(e.FetchedAt) > threshold
			if listFlags.stale && !stale {
				continue
			}
			rows = append(rows, row{
				URL:           e.URL,
				ProductKey:    e.ProductKey,
				FetchedAt:     e.FetchedAt,
				Depth:         e.Depth,
				TopicsFetched: e.TopicsFetched,
				Status:        e.Status,
				Stale:         stale,
			})
		}

		if listFlags.jsonOutput {
			return jsonEncode(rows)
		}

		if len(rows) == 0 {
			fmt.Fprintln(os.Stderr, "no entries found")
			return nil
		}
		fmt.Printf("%-60s %-12s %6s %8s %s\n", "URL", "PRODUCT", "DEPTH", "TOPICS", "FETCHED_AT")
		fmt.Printf("%-60s %-12s %6s %8s %s\n",
			"------------------------------------------------------------",
			"------------", "------", "--------", "-------------------")
		for _, r := range rows {
			staleTag := ""
			if r.Stale {
				staleTag = " [STALE]"
			}
			fmt.Printf("%-60s %-12s %6d %8d %s%s\n",
				truncateStr(r.URL, 60),
				truncateStr(r.ProductKey, 12),
				r.Depth,
				r.TopicsFetched,
				r.FetchedAt.Format("2006-01-02T15:04Z"),
				staleTag,
			)
		}
		return nil
	},
}

func init() {
	f := listCmd.Flags()
	f.BoolVar(&listFlags.jsonOutput, "json", false, "Output as JSON array")
	f.BoolVar(&listFlags.stale, "stale", false, "Show only stale entries")
	f.IntVar(&listFlags.olderThan, "older-than", 7, "Age threshold in days for --stale")
	f.StringVar(&listFlags.product, "product", "", "Filter by product key")
	rootCmd.AddCommand(listCmd)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
