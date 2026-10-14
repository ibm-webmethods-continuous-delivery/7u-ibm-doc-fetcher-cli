# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.0.5] - 2026-10-14

### Security
- **stdlib CVE remediation**: Bumped minimum Go toolchain to `1.26.9` in `go.mod` and added `apk upgrade --no-cache` before `apk add go` in the builder stage of `Dockerfile`. This ensures the Alpine package index provides Go ≥ 1.26.9, resolving all 13 stdlib CVEs reported by Trivy (3× HIGH, 5× MEDIUM, 1× LOW, 4× UNKNOWN) present in the v0.0.4 image which was built with Go 1.26.8.

### Changed
- `go.mod`: `go 1.26.0` → `go 1.26.9`
- `Dockerfile`: builder `RUN apk add` now preceded by `apk upgrade --no-cache` to pull the latest patched Alpine packages at build time.

---

## [v0.0.4] - 2026-10-14

### Added
- **Deterministic Canonical URL Resolution (`source_url`)**:
  - Implemented `ResolveSourceURL` in `ibmdocs.Client` to resolve verified IBM Docs browser URLs via product-scoped Search API queries.
  - Added `source_url` to cache entries (`cache.ContentEntry`) and Markdown frontmatter.
  - Implemented `findBySlugExact` / `ResolveHrefBySlug` to map real browser `?topic=` slugs back to exact TOC nodes.
  - Added `BrowserURLFromFrontmatter` helper prioritizing verified `source_url`.
- **Adaptive HTTP 429 Rate Limiting & Backoff**:
  - Automatic retry loop for `HTTP 429 Too Many Requests` (up to 3 retries, exponential backoff starting at 2s up to 30s cap, honoring `Retry-After` headers).
  - **Adaptive Sticky Delay ("Ratchet" Effect)**: Ratchets active inter-request delay up to $\max(\text{current\_delay} \times 2, 1000\text{ms})$ for the entire session upon encountering a 429 response.
  - **Centralized Rate Pacing**: Moved rate pacing directly into `ibmdocs.Client.do()`, uniformly pacing all outbound HTTP calls (TOC, Content, Search, pagination) while keeping local cache hits instantaneous.
  - **Negative Search Caching**: Memoizes search misses in-memory during crawls to suppress duplicate Search API calls.
  - **Static Asset Skipping**: Added `IsStaticAsset` helper to skip Search API queries for specs (`.yaml`, `.yml`, `.json`), images, and binary files.
  - **Unsuppressed Rate Limit Diagnostics**: Structured `[RATE LIMIT]` stderr output reporting retried vs. throttled request counts (never suppressed by `--quiet` or `--json`).
- **Agent Skill Guidelines**:
  - Added rate limiting interrupt/pause protocol and canonical URL citation rules in `SKILL.md`.
- **Documentation**:
  - Added `docs/ToolComparison.md` comparing `ibmdocs` with generic doc scrapers.
  - Updated `PRODUCT_REQUIREMENTS.md` with rate limit backoff policies and deterministic URL requirements.

### Changed
- Refactored `fetcher.Walker` to delegate all network pacing to `ibmdocs.Client`.
- Updated `Makefile` `build-all` target to ensure `deps` are checked first.
- Cleaned and updated `go.mod` / `go.sum` dependencies.

### Deprecated
- Hand-constructed `?topic=` slug generation via `kb.BrowserURL` (IBM Docs slugs are non-derivable and redirect on mismatch; `source_url` must be used instead).

---

## [v0.0.3] - 2026-10-05

### Added
- Cross-platform release targets (`linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`).
- Noise reduction filters for HTML-to-Markdown conversion.
- `trim` subcommand for context budget management.
- Request logging to `requests.log`.

---

## [v0.0.2] - 2026-10-04

### Added
- Initial batch crawling via `fetch-file`.
- Search command (`ibmdocs search`) with version deduplication.
- Cache TTL management and index tracking.

---

## [v0.0.1] - 2026-10-02

### Added
- Initial release with basic `fetch` and `build-kb` commands.
