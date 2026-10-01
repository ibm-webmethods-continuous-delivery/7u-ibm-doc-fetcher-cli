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
	CacheTTLHours         = 24
	MinContentChars       = 350

	EnvCDNBaseURL = "IBMDOCS_CDN_BASE_URL"
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
}
