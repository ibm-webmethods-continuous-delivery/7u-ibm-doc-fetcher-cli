module github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli

go 1.26.0

require (
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2
	github.com/spf13/cobra v1.8.1
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	// Security floor — kept explicit to satisfy trivy CVE checks.
	// CVE-2026-56854 / CVE-2026-56855 / CVE-2026-78662: fixed in v0.56.0
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	// CVE-2026-56852: fixed in v0.39.0
	golang.org/x/text v0.41.0 // indirect
)
