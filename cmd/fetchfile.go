package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var fetchFileFlags struct {
	depth      int
	refresh    bool
	jsonOutput bool
	quiet      bool
	failFast   bool
	maxTopics  int
}

var fetchFileCmd = &cobra.Command{
	Use:   "fetch-file <manifest>",
	Short: "Batch fetch IBM Docs URLs from a manifest file",
	Long: `Reads a plain-text file of IBM Docs URLs (one per line) and fetches
each recursively. Lines beginning with # and blank lines are skipped.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		urls, err := readManifest(args[0])
		if err != nil {
			return err
		}
		if len(urls) == 0 {
			fmt.Fprintln(os.Stderr, "manifest contains no URLs")
			return nil
		}

		anyFailed := false
		for i, u := range urls {
			if !fetchFileFlags.quiet {
				fmt.Fprintf(os.Stderr, "\n[%d/%d] %s\n", i+1, len(urls), u)
			}
			err := runFetch(u, true, fetchFileFlags.depth,
				fetchFileFlags.refresh, fetchFileFlags.jsonOutput,
				fetchFileFlags.quiet, fetchFileFlags.maxTopics)
			if err != nil {
				anyFailed = true
				fmt.Fprintf(os.Stderr, "[ERROR] %s: %v\n", u, err)
				if fetchFileFlags.failFast {
					return fmt.Errorf("fetch failed (--fail-fast): %w", err)
				}
			}
		}
		if anyFailed {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	f := fetchFileCmd.Flags()
	f.IntVarP(&fetchFileFlags.depth, "depth", "D", 3, "Recursion depth applied to all URLs")
	f.BoolVar(&fetchFileFlags.refresh, "refresh", false, "Ignore cache for all URLs")
	f.BoolVar(&fetchFileFlags.jsonOutput, "json", false, "Print a JSON summary to stdout")
	f.BoolVarP(&fetchFileFlags.quiet, "quiet", "q", false, "Suppress per-URL progress")
	f.BoolVar(&fetchFileFlags.failFast, "fail-fast", false, "Stop on first fetch failure")
	f.IntVar(&fetchFileFlags.maxTopics, "max-topics", 10,
		"Maximum child topics per TOC node (0 = unlimited)")
	rootCmd.AddCommand(fetchFileCmd)
}

// readManifest reads a URL manifest file, skipping blank lines and # comments.
func readManifest(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only handle; close error is not actionable

	var urls []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	return urls, sc.Err()
}
