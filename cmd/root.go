package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/config"
)

// errPartialFailure is returned when one or more topics fail but others succeed.
// Execute() maps this to exit code 1 without printing usage or the error string.
var errPartialFailure = errors.New("one or more topics failed")

// Injected at link time via -ldflags.
var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "unknown"
)

// cfg is populated from global flags and passed into subcommands.
var cfg config.Config

var rootCmd = &cobra.Command{
	Use:   "ibmdocs",
	Short: "Fetch and cache IBM product documentation as agent-ready Markdown",
	Long: `ibmdocs fetches IBM product documentation from the IBM Docs public API,
caches it locally, and exports it as agent-ready Markdown.

Data folder defaults to ./ibmdocs-data in the current directory.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errPartialFailure) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

func init() {
	f := rootCmd.PersistentFlags()

	f.StringVarP(&cfg.DataDir, "data", "d", config.DefaultDataDir,
		"Path to the data folder (cache/, kb/, index.json)")

	f.StringVar(&cfg.Lang, "lang", config.DefaultLang,
		"Language code for IBM Docs API requests (e.g. en, fr, de, ja)")

	f.BoolVarP(&cfg.Verbose, "verbose", "v", false,
		"Enable debug-level logging to stderr")

	f.BoolVar(&cfg.NoColor, "no-color", false,
		"Disable ANSI colour in terminal output")

	f.BoolVar(&cfg.HTTPDebug, "http-debug", false,
		"Dump every HTTP request/response to <data>/debug/")

	f.StringVar(&cfg.CDNBaseURL, "cdn-base-url",
		envOr(config.EnvCDNBaseURL, config.DefaultCDNBaseURL),
		"IBM Docs CDN base URL (env: IBMDOCS_CDN_BASE_URL)")

	f.DurationVar(&cfg.Delay, "delay", config.DefaultDelay,
		"Inter-request delay for polite crawling (e.g. 200ms, 0 to disable)")

	f.DurationVar(&cfg.CacheTTL, "cache-ttl",
		envDurationOr(config.EnvCacheTTL, config.DefaultCacheTTL),
		"How long a cached entry stays valid before a plain fetch re-hits the network "+
			"(e.g. 720h = 30 days, 24h, 0 to always treat cache as stale). env: IBMDOCS_CACHE_TTL. "+
			"Use --refresh on fetch/fetch-file to force a re-fetch for a single invocation regardless of this value.")
}

// envOr returns the environment variable value or fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envDurationOr returns the parsed environment variable duration or fallback
// if unset or invalid.
func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
