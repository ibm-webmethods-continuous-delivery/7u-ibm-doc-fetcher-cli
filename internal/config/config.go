// Package config holds compile-time defaults and the runtime Config struct
// shared across all commands.
package config

import "time"

// Compile-time defaults — overridable via ldflags or runtime flags.
const (
	DefaultDataDir        = "./ibmdocs-data"
	DefaultLang           = "en"
	DefaultDepth          = 3
	DefaultMaxTopics      = 10
	DefaultDelay          = 200 * time.Millisecond
	DefaultCDNBaseURL     = "https://1.www.s81c.com"
	DefaultRequestTimeout = 15 * time.Second
	MinContentChars       = 350

	// DefaultCacheTTL is how long a cached TOC/content entry is considered
	// fresh before a plain fetch re-hits the network. 30 days favours KB
	// reuse (fewer CDN requests, fewer tokens re-ingested) for documentation
	// that changes infrequently; use --cache-ttl or --refresh to shorten
	// this for fast-moving products or a specific invocation.
	DefaultCacheTTL = 30 * 24 * time.Hour

	EnvCDNBaseURL = "IBMDOCS_CDN_BASE_URL"
	EnvCacheTTL   = "IBMDOCS_CACHE_TTL"
)

// Config is populated from global flags and passed into every command.
type Config struct {
	DataDir    string
	Lang       string
	Verbose    bool
	NoColor    bool
	HTTPDebug  bool
	CDNBaseURL string
	Delay      time.Duration
	CacheTTL   time.Duration
}
