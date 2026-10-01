# Product Requirements — IBM Documentation Fetcher CLI (`ibmdocs`)

**Version:** 0.1.0-draft
**Status:** Requirements
**Source spike:** `1c-spike-ibm-doc-fetcher`

---

## 1. Purpose

`ibmdocs` is a portable, statically-linked command-line tool that fetches IBM product documentation from the IBM Docs public API, caches it locally, and exports it as agent-ready Markdown. It is designed to be used directly by human operators and by AI agents as a deterministic, dependency-free documentation retrieval tool.

The tool is **not** an MCP server and has no runtime service component. It is a batch CLI — run it, get files, done.

---

## 2. Non-Goals (Explicit Out of Scope)

- **Raw URL fetching** is out of scope. Fetching arbitrary non-IBM-Docs URLs (e.g., GitHub READMEs, OpenAPI specs) is excluded. Operators can use `curl` for those.
- No MCP protocol, no HTTP server, no daemon mode.
- No authentication against IBM APIs (all target endpoints are public, no-auth).
- No vector embedding or indexing (that belongs to the downstream RAG layer).

---

## 3. Platform and Deployment Requirements

### 3.1 Cross-Platform Binary

| Platform | Architecture | Required |
|---|---|---|
| Linux | x86_64 (amd64) | ✅ |
| Linux | ARM64 (aarch64) | ✅ |
| macOS | Apple Silicon (arm64) | ✅ |
| macOS | Intel (amd64) | ✅ |
| Windows | x86_64 | ✅ |

The binary MUST be **fully statically linked**:
- No dependency on OS-provided `libc`, `libssl`, `libxml`, or any shared library.
- On Linux: compile against `musl` libc (via `CGO_ENABLED=0`) to produce a binary that runs on any Linux kernel ≥ 4.x regardless of distro.
- TLS MUST be handled by Go's built-in TLS stack. No OpenSSL linkage.

### 3.2 Container Delivery (Preferred)

The preferred production delivery form is a `FROM scratch` container image:

- Base: `scratch` (no OS layer, no shell, no distro).
- Contents: the `ibmdocs` binary + `/etc/ssl/certs/ca-certificates.crt` (required for HTTPS).
- Multi-arch image: a single manifest MUST support both `linux/amd64` and `linux/arm64`.
- The image is built via a two-stage Dockerfile: `golang:*-alpine` builder → `scratch` final.
- No Alpine shell or package manager is present in the final image.

### 3.3 Binary Release via GitHub Releases

Pre-built binaries for all supported platform/architecture combinations (§3.1) are published to **GitHub Releases** on every tagged version. This is the primary distribution channel for direct binary use on macOS, Windows, and Linux.

- Each release MUST include binaries for all five targets (see §3.1), named `ibmdocs-<os>-<arch>[.exe]`.
- Each release MUST include a `checksums.txt` (SHA-256) so users can verify downloads.
- The container image and the GitHub Release binaries are produced by the same CI pipeline from the same commit.
- The data folder (see §4) is the only external dependency at runtime — no installer is required.

---

## 4. Data Folder

The CLI MUST accept a `--data` flag (or `-d` shorthand) pointing to a local directory. This is the single root for all persistent state.

**Required sub-structure:**

```
<data>/
  cache/          # Raw IBM Docs API responses (JSON), keyed by product + topic
    toc/          # TOC responses, one per product key
    content/      # Content responses, one per topic href
  kb/             # Agent-ready Markdown exports
    <product-key>/
      <topic>/
        en.md
      INDEX.md
    INDEX.md      # Top-level index across all products
  index.json      # Fetch index: URL → { fetched_at, depth, topics_count, status }
  debug/          # HTTP traffic dumps (written only when --http-debug is active)
    <YYYYMMDD-HHMMSS>-<method>-<sanitised-url>.req.txt
    <YYYYMMDD-HHMMSS>-<method>-<sanitised-url>.res.txt
```

- If `--data` is not supplied, the tool MUST default to `./ibmdocs-data` in the current working directory and create it if it does not exist.
- The `cache/` and `kb/` subdirectories are created automatically on first use.
- The data folder is designed to be committed to version control if desired (JSON + Markdown, no binary blobs).

---

## 5. Command Reference

All commands follow the Cobra subcommand pattern:

```
ibmdocs [global flags] <command> [flags] [args]
```

### 5.1 Global Flags

| Flag | Default | Description |
|---|---|---|
| `--data`, `-d` | `./ibmdocs-data` | Path to the data folder (§4) |
| `--lang` | `en` | Language code for IBM Docs API requests (see §17 for supported values) |
| `--verbose`, `-v` | false | Enable debug-level logging to stderr |
| `--no-color` | false | Disable ANSI colour in terminal output |
| `--http-debug` | false | Dump every HTTP request and response to `<data>/debug/` (see §16) |

---

### 5.2 `fetch` — Fetch a single IBM Docs URL

```
ibmdocs fetch <URL> [flags]
```

Fetches the IBM documentation topic identified by a standard IBM Docs browser URL.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--recursive`, `-r` | false | Follow child topics in the TOC |
| `--depth`, `-D` | `3` | Maximum recursion depth (ignored when `--recursive` is not set) |
| `--refresh` | false | Ignore cache and re-fetch even if a valid cached entry exists |
| `--json` | false | Print a JSON summary of fetched topics to stdout |
| `--quiet`, `-q` | false | Suppress progress output; only print errors |

**Behaviour without `--recursive`:** fetches the single topic identified by the URL. Writes content to cache and exports Markdown to `kb/`. Prints the extracted Markdown to stdout unless `--quiet` is set.

**Behaviour with `--recursive`:** fetches the root topic, then walks the TOC tree depth-first up to `--depth` levels. At each level, at most `MAX_TOPICS_PER_LEVEL` children are followed (see §8.1).

**Exit codes:**
- `0` — all requested topics fetched successfully
- `1` — one or more topics failed; partial results may exist in cache
- `2` — fatal error (URL parse failure, network unreachable, bad flags)

---

### 5.3 `fetch-file` — Batch fetch from a URL manifest file

```
ibmdocs fetch-file <manifest-file> [flags]
```

Reads a plain-text file of IBM Docs URLs (one per line) and fetches each with recursion.

**Manifest file format:**

```
# IBM Integration SaaS — platform administration
https://www.ibm.com/docs/en/integration-saas-lib/integration-saas/saas?topic=ipaas-admin

# webMethods Integration public APIs
https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis
```

- Lines beginning with `#` (with optional leading whitespace) are comments and are skipped.
- Blank lines are skipped.
- Inline comments (after a URL on the same line) are NOT supported; the full line after trimming is treated as a URL.

**Flags:** same as `fetch` except `--recursive` is always enabled and cannot be disabled (batch mode always recurses).

| Flag | Default | Description |
|---|---|---|
| `--depth`, `-D` | `3` | Recursion depth applied to all URLs in the file |
| `--refresh` | false | Ignore cache for all URLs |
| `--json` | false | Print a JSON summary of all fetches to stdout |
| `--quiet`, `-q` | false | Suppress per-URL progress; print only the final summary |
| `--fail-fast` | false | Stop on the first fetch failure instead of continuing |

---

### 5.4 `refresh` — Refresh stale cache entries

```
ibmdocs refresh [flags]
```

Scans `index.json` and re-fetches all entries whose `fetched_at` timestamp is older than the given threshold.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--older-than`, `-o` | `7` | Re-fetch entries older than this many days |
| `--depth`, `-D` | (stored depth) | Override the recursion depth used during the original fetch |
| `--dry-run` | false | Print what would be re-fetched without making any network requests |
| `--json` | false | Print a JSON summary of refresh results |

**Behaviour:** uses the stored `depth` from `index.json` for each entry unless `--depth` overrides it. Processes entries in the order they appear in `index.json` (i.e., insertion order — most recently added last).

---

### 5.5 `build-kb` — Export cache to agent-ready Markdown

```
ibmdocs build-kb [flags]
```

Transforms all entries in `cache/content/` into structured Markdown files under `kb/`, applying YAML frontmatter and updating the product INDEX files. Idempotent — safe to re-run.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--index-only` | false | Skip Markdown export; only rebuild INDEX.md files from existing `kb/` content |
| `--dry-run` | false | Print what would be written without writing any files |
| `--product` | (all) | Limit processing to a specific product key (e.g. `wm-integration-ipaas`) |

**Frontmatter fields extracted per topic** (when detectable):

```yaml
---
product: "wm-integration-ipaas"
topic: "wmint_public_apis"
last_updated: "2024-11-01"
http_methods:
  - "GET"
  - "POST"
api_paths:
  - "/apis/v1/rest/workflows"
auth:
  - "x-instance-api-key"
  - "Bearer"
requires_admin: true
stub: false
---
```

Thin topics (content below `MIN_CONTENT_CHARS` threshold, see §8.1) are flagged with `stub: true` and merged into a `_stubs_merged.md` file per product.

---

### 5.6 `list` — List cached entries

```
ibmdocs list [flags]
```

Prints a human-readable or JSON table of all entries in `index.json`.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--json` | false | Output as JSON array |
| `--stale` | false | Show only entries older than `--older-than` days |
| `--older-than` | `7` | Age threshold in days used with `--stale` |
| `--product` | (all) | Filter by product key |

---

### 5.7 `trim` — Trim KB to fit agent context constraints

```
ibmdocs trim [flags]
```

Removes low-value content from the `kb/` directory to keep total Markdown size within a target budget. Operates on the **KB only** — the `cache/` directory is never modified by `trim`, so the KB can be rebuilt in full at any time via `build-kb`.

**Trim criteria (applied in order, stopping when the budget is met):**

1. **Stubs** — topics flagged `stub: true` in frontmatter (below `MIN_CONTENT_CHARS`). These are always the first to go; they add noise without content.
2. **Oldest entries** — topics whose cache entry is oldest by `fetched_at`, on the assumption that stale content is less relevant.
3. **Smallest substantive topics** — topics above the stub threshold but still the smallest by character count. Removed last as they may still carry useful content.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--max-kb-size` | `0` | Target maximum total size of `kb/` in kilobytes. `0` means no size budget; use with `--stubs-only` or `--product` instead. |
| `--stubs-only` | false | Remove only stub topics, regardless of size budget |
| `--product` | (all) | Restrict trimming to a single product key |
| `--dry-run` | false | Print what would be removed without deleting anything |
| `--json` | false | Output a JSON summary of removed files and bytes freed |

**Safety constraints:**
- `trim` never removes `INDEX.md` files.
- `trim` never touches `cache/`.
- `trim` requires `--dry-run` to be explicitly passed before any destructive run where `--max-kb-size` would remove non-stub content (i.e., it always suggests `--dry-run` first in its output).
- Running `build-kb` after `trim` fully restores the KB from cache.

> **Design rationale:** The primary agent use case is feeding the `kb/` tree to an LLM context window or a RAG loader. Both have hard size ceilings. `trim` provides a deterministic, reproducible way to fit the KB into those ceilings without touching the authoritative cache. The operator or agent can always `build-kb` to restore.

---

### 5.8 `search` — Search IBM Docs by keyword

```
ibmdocs search <query> [flags]
```

Performs a full-text search across all IBM product documentation using the IBM Docs Search API, returning ranked topics with their product keys and direct IBM Docs URLs. This is a **network operation** (no local index required).

**⚠️ Authentication constraint (discovered during spike exercise):** The primary search endpoint `https://www.ibm.com/docs/api/v1/search` is the browser-facing proxy. HAR analysis confirms it requires active browser session cookies — specifically `cf_clearance` (Cloudflare bot challenge), `connect.sid` (IBM Docs session), `SESSION_COLLATE_COOKIE`, and `_abck` — to return results from a headless HTTP client. A statically-linked CLI binary cannot satisfy these cookie requirements without a browser.

The CDN-direct endpoint `https://1.www.s81c.com/docs/api/v1/search` was confirmed to respond without cookies in spike testing and returns the same response schema. **The `search` command MUST use `1.www.s81c.com` as its search host**, not `www.ibm.com`.

**API used:** `GET https://1.www.s81c.com/docs/api/v1/search?query={query}&lang={lang}&limit={limit}&start={start}` — same CDN host as TOC/Content, same no-auth, same User-Agent/Referer headers.

**Pagination:** `next` = `start + limit`; `previous` = `start - limit` (-1 when on page 0). Both are integer offsets, not cursors. Iterate by setting `start=next` until `next >= hits`.

**Version noise (critical UX finding):** IBM Docs indexes every supported version of a product separately. A search for "Kubecost" returns the same topic (e.g. "Kubecost Metrics") three times — once for v1.x, v2.x, and v3.x — consuming result slots with duplicate content. Measured: 20 results → only 12 unique titles, 8 cross-version duplicates. The `--latest-only` flag addresses this by keeping only the highest-versioned result per unique title per product family.

**⚠️ `product.key` is an internal ID, not the TOC key:** The `product.key` field in search results is the IBM internal product identifier (e.g. `SSXAAZY`, `SSW0JQG_3.0.x`) — **not** the friendly product key used in the TOC API (e.g. `wm-apigateway-ipaas`). Confirmed by testing: every search result shows a mismatch between `product.key` and the path segment in `fullurl`. The `fullurl` field contains the correct IBM Docs browser URL from which the TOC-compatible product key can be extracted (path after `/docs/en/`). The `search` command output and `--url-only` flag MUST be based on `fullurl`, not `product.key`.

**Response fields used per hit:**

| Field | Use |
|---|---|
| `title` | Display title (may contain `<b>` highlight tags — strip before output) |
| `fullurl` | Complete IBM Docs browser URL — use this as the authoritative product URL; extract TOC product key from its path |
| `product.key` | IBM internal identifier — **do not use as TOC API key**; display only |
| `product.label` | Human-readable product name |
| `snippet` | Short excerpt with `<b>` highlights — strip tags before output |
| `productBreadCrumb` | Navigation path showing product family and version |
| `readTime` | Estimated read time in minutes |
| `date` | Last-updated date |

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--limit`, `-n` | `10` | Maximum number of results to return per page |
| `--all` | false | Paginate through all results automatically (up to `hits` total) |
| `--latest-only` | false | Deduplicate cross-version results, keeping only the most recent version per unique title per product |
| `--json` | false | Output as JSON array of result objects |
| `--product` | (none) | Filter results to a specific product key |
| `--url-only` | false | Print only the `fullurl` of each result, one per line (convenient for piping into `ibmdocs fetch-file`) |

---

### 5.9 `version` — Print version information

```
ibmdocs version
```

Prints tool name, version, commit SHA, and build date. No flags.

---

## 6. Fetch Index (`index.json`)

The `index.json` file in the data root is the canonical record of all fetch operations. It serves as the input for `refresh` and `list`.

**Schema (one entry per originally requested URL):**

```json
{
  "version": 1,
  "entries": [
    {
      "url": "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis",
      "product_key": "wm-integration-ipaas",
      "fetched_at": "2025-07-27T14:32:00Z",
      "depth": 3,
      "topics_fetched": 12,
      "topics_failed": 0,
      "status": "ok"
    }
  ]
}
```

> The timestamp format is RFC 3339 UTC (`2006-01-02T15:04:05Z`). The `fetched_at` field in `index.json` is the authoritative staleness source used by `refresh` and `list --stale`, independent of filesystem mtime.

- `status` values: `"ok"` | `"partial"` (some topics failed) | `"failed"` (root topic failed)
- The file is read/written atomically (write to a temp file, then rename).
- Entries are keyed by `url`; re-fetching the same URL updates the existing entry in place.

---

## 7. IBM Docs API Contract

### 7.1 Endpoints

The tool relies on three public, unauthenticated endpoints. All three were validated against live IBM CDN during spike exploration, including HAR analysis of the IBM Docs browser SPA.

| Endpoint | Base URL | Path pattern |
|---|---|---|
| TOC | `https://1.www.s81c.com` | `/docs/api/v1/toc/{product_key}?lang={lang}` |
| Content | `https://1.www.s81c.com` | `/docs/api/v1/content/{href}?parsebody=true&lang={lang}` |
| Search | `https://1.www.s81c.com` | `/docs/api/v1/search?query={query}&lang={lang}&limit={n}&start={s}` |

All three require no authentication. The browser-facing proxy at `www.ibm.com/docs/api/v1/*` requires session cookies and is explicitly disallowed in `www.ibm.com/robots.txt` (`Disallow: /docs/api`). The CDN-direct host is used instead.

### 7.2 CDN URL Stability Risk and Mitigation

**The `1.www.s81c.com` hostname is not a publicly documented or contractually stable API.** It is IBM's own CDN origin, hardcoded in the IBM Docs browser SPA (`index_bundle.js`) and visible in DevTools on any docs page. It has been used for IBM's static/CDN assets since at least 2016, but:

- There is no IBM-published SLA or versioning contract for it.
- The `v1` path segment implies a version that IBM could supersede with `v2` without announcement.
- IBM could migrate to a different CDN vendor or restructure the origin path at any time.
- No deprecation notice mechanism exists for undocumented internal APIs.

**Risk level: medium.** The hostname is load-bearing for the IBM Docs SPA itself — any change would break IBM's own documentation site for all users until the SPA bundle is updated. This provides strong practical stability, but no formal guarantee.

**Mitigation — runtime-overridable base URL (required):**

The CDN base URL MUST be isolated into a single configuration constant and exposed as a global flag and environment variable so users can adapt without a code change or new release:

| Mechanism | Name | Default |
|---|---|---|
| Global flag | `--cdn-base-url` | `https://1.www.s81c.com` |
| Environment variable | `IBMDOCS_CDN_BASE_URL` | `https://1.www.s81c.com` |

All three API paths (TOC, Content, Search) are constructed relative to this base. Changing `--cdn-base-url` redirects all requests.

**Operational monitoring:** The tool MUST detect and surface CDN base URL failures distinctly from content-not-found errors. When the CDN host returns a non-404 HTTP error (e.g. 301 redirect to a new host, 502/503) on a TOC or Search request, the error message MUST include the full URL attempted and suggest checking `--cdn-base-url`, so operators immediately know what to override rather than diagnosing a generic network failure.

### 7.3 Crawling Policy

Verified via standard crawling-policy discovery mechanisms:

| Check | Finding |
|---|---|
| `1.www.s81c.com/robots.txt` | `User-agent: * / Allow: /common/` — no Disallow rules; docs API paths unrestricted |
| `www.ibm.com/robots.txt` | `Disallow: /docs/api` — explicitly disallows the browser-proxy host; irrelevant to our CDN-direct approach |
| Response `X-Robots-Tag` header | Not present on any CDN response |
| Response rate-limit headers (`X-RateLimit-*`, `Retry-After`) | Not present — no declared throttle |
| `cache-control` on CDN responses | `max-age=3600` — IBM caches responses for 1 hour, consistent with our 24h TTL |
| `ETag` / `Last-Modified` | Present on all responses — enables conditional `GET` optimisation (post-v0.1) |
| CORS | `access-control-allow-origin: https://www.ibm.com` — browser-only restriction; irrelevant to a CLI |

**Polite crawling:** Although no rate limit is declared, the tool MUST implement a configurable inter-request delay to avoid hammering the CDN. This is good engineering practice regardless of policy and protects both IBM's infrastructure and the tool's continued access.

| Constant | Default | Flag |
|---|---|---|
| `REQUEST_DELAY_MS` | `200` | `--delay` (milliseconds between sequential HTTP requests) |

A delay of 200ms means a 16-topic fetch completes in ~3 seconds, which is imperceptible to a user and well within any reasonable rate limit.

### 7.4 Request Contract

**Product key format:** Product keys are **multi-segment path strings**, not simple identifiers. Examples validated in the spike:

| Browser URL | Product key used in TOC API |
|---|---|
| `ibm.com/docs/en/wm-integration-ipaas` | `wm-integration-ipaas` |
| `ibm.com/docs/en/integration-saas-lib/integration-saas/saas` | `integration-saas-lib/integration-saas/saas` |
| `ibm.com/docs/en/kubecost/self-hosted/3.x` | `kubecost/self-hosted/3.x` |

The product key is the path after `/docs/en/` with no transformation. The TOC API accepts the full slash-separated path verbatim.

**Cross-version content pointers (`?cp=` parameter):** TOC `href` fields for content sourced from an older version carry a `?cp=SSW0JQG_3.0.x` query parameter (internal content pointer). The Content API accepts and requires this parameter when present — it must be URL-encoded and passed as-is.

**URL parsing:** Input URLs follow the IBM Docs browser format:
```
https://www.ibm.com/docs/en/<product-key>[/<sub>/<sub>]?topic=<topic-slug>
```

**Topic slug resolution:** The slug in the browser URL is matched against TOC `href` filenames and topic labels using the three-strategy algorithm from the spike (label-derived, exact filename, progressive suffix). This logic must be ported exactly and covered by unit tests with real TOC fixture data.

**Required request headers:** Every request to the CDN MUST include:
```
User-Agent: Mozilla/5.0 (compatible; ibmdocs/{version})
Referer:    https://www.ibm.com/docs/
Accept:     */*
```
The `User-Agent` header should identify the tool by name and version (not impersonate a browser, now that we know the CDN does not enforce a specific UA pattern — the Referer header is the relevant gate).

---

## 8. Configuration Constants

### 8.1 Defaults (tunable at compile time via ldflags or at runtime via flags)

| Constant | Default | Flag / Env var | Description |
|---|---|---|---|
| `MAX_TOPICS_PER_LEVEL` | `10` | `--max-topics` | Maximum child topics followed per TOC node during recursion; `0` = unlimited |
| `DEFAULT_DEPTH` | `3` | `--depth` / `-D` | Default recursion depth for `fetch --recursive` and `fetch-file` |
| `CACHE_TTL_HOURS` | `24` | — | Hours before a cached entry is considered stale (for `--refresh` bypass) |
| `MIN_CONTENT_CHARS` | `350` | — | Below this character count (after stripping metadata), a topic is a stub |
| `REQUEST_TIMEOUT_S` | `15` | — | HTTP request timeout in seconds |
| `REQUEST_DELAY_MS` | `200` | `--delay` | Milliseconds to wait between sequential HTTP requests (polite crawling) |
| `DEFAULT_LANG` | `en` | `--lang` | Default language for IBM Docs API |
| `CDN_BASE_URL` | `https://1.www.s81c.com` | `--cdn-base-url` / `IBMDOCS_CDN_BASE_URL` | Base URL for all IBM Docs CDN API requests |

> **`MAX_TOPICS_PER_LEVEL` rationale:** The webMethods API Gateway Administration APIs TOC node has 16 children — all substantive REST API family references. A value of 5 caused topics 6-16 to be silently dropped, including Policy Management, Service Management, User Management, and Subscription Management (each 1,800–3,800 chars of API reference content). Raising to 10 captures the vast majority of real-world product trees while remaining bounded. Pass `--max-topics 0` to disable the cap entirely.

---

## 9. Output Formats

### 9.1 Human output (default)

Progress is written to **stderr** so that stdout can be piped without contamination.  
Extracted Markdown content is written to **stdout** for single-URL `fetch` without `--quiet`.  
Errors are written to **stderr** with a clear `[ERROR]` prefix.

### 9.2 JSON output (`--json`)

When `--json` is set, a single JSON object is written to stdout at the end of the operation. No progress lines are written to stdout. Stderr logging is unaffected.

### 9.3 Exit codes

| Code | Meaning |
|---|---|
| `0` | Complete success |
| `1` | Partial failure (at least one topic failed, others succeeded) |
| `2` | Fatal / configuration error (bad flags, unparseable URL, etc.) |

---

## 10. HTML-to-Markdown Conversion

The IBM Docs Content API returns a pre-extracted HTML body fragment (`parsebody=true`). The tool converts this fragment to Markdown using a **pure-Go, CGO-free** HTML-to-Markdown library (e.g., `github.com/JohannesKaufmann/html-to-markdown`). No external binaries or native libraries are permitted.

### 10.1 Fragment structure (observed from raw Content API responses)

The raw HTML returned by the Content API is not a full HTML document for concept and task topics — it is an `<article>` fragment starting directly with the topic content. REST API reference topics are wrapped in a plain `<div>`. Two structural patterns were confirmed:

| Pattern | Example products | Outer wrapper |
|---|---|---|
| Fragment (no `<html>`) | IBM Integration SaaS REST APIs | `<div><div><article ...>` |
| Full mini-document | Kubecost, API Gateway | `<html><head></head><body><div><article ...>` |

Both patterns MUST be handled. The HTML-to-Markdown library receives the raw response body directly; the wrapper does not affect conversion.

### 10.2 Content present in every fragment (must be preserved)

- `<h1 class="topictitle1">` — page title
- `<div id="lastModifiedDate"><span>Last Updated</span>: YYYY-MM-DD</div>` — always present, always at top; **strip this line from Markdown output** (it is metadata, not content; kept in the JSON cache for frontmatter `last_updated`)
- `<p class="shortdesc">` — one-sentence summary; preserve as first paragraph
- `<section>`, `<h2>`, `<h3>` — semantic structure; convert to `##` / `###`
- `<pre class="codeblock">` + `<code>` — code blocks; always paired, convert to fenced ` ``` ` blocks
- `<dl>` / `<dt>` / `<dd>` — definition lists used heavily in REST API reference (parameter tables); convert to bold-term + indented definition or a Markdown table

### 10.3 Noise present in every fragment (must be stripped)

| Element / pattern | Location | Action |
|---|---|---|
| `<div id="lastModifiedDate">...</div>` | Top of body | Strip entirely; capture date value for `last_updated` frontmatter field |
| `<nav>` / `<aside>` with class `related-links` or `bottom-section-parent` | Tail of every document | Strip entirely — these are "Related topics" and "Parent topic" navigation blocks with no content value |
| `<svg>` arrow icons inside nav links | Inside stripped `<nav>` | Stripped with parent |
| `<span class="ph">` product name placeholders | Inline throughout | Unwrap — keep text content, discard span |
| IBM redirect links `ibm.com/links?url=...` | `<a href>` attributes | Decode the `url=` query parameter and use as the actual href |
| Internal `/docs/en/SSC74RW_saas/...` links | `<a href>` attributes | Preserve as-is (they are valid IBM Docs browser URLs) |

### 10.4 Conversion settings

- Strip HTML comments.
- Unwrap `<strong>` and `<em>` inline spans — do not strip, convert to `**` and `_`.
- `<img>` tags: strip entirely. The Content API occasionally returns diagrams as `<img>` references; these are not useful in a text-only knowledge base.
- `<table>`: convert to GFM pipe tables. IBM Docs REST API reference rarely uses HTML `<table>` (they use `<dl>` lists instead) but it does occur in configuration reference pages.
- Minimum usable content threshold: `MIN_CONTENT_CHARS` characters **after stripping the `lastModifiedDate` line and all nav blocks** (see §8.1).

---

## 11. Caching Behaviour

- Cache entries are keyed by `{type}:{product_key}/{topic_filename}:{lang}` matching the spike's path layout.
- A cache entry is considered **valid** if its file modification time is within `CACHE_TTL_HOURS`.
- `--refresh` bypasses the TTL check and forces a network fetch; the old cache file is overwritten.
- Cache files are written atomically (temp file + rename) to avoid corrupt partial writes.
- The `refresh` command uses `index.json` timestamps (not file mtime) as the age source for user-visible staleness, keeping the behaviour consistent across filesystem copy/move.

**Cache key safety with `?cp=` cross-version hrefs:** TOC `href` values for content sourced from an older product version carry a `?cp=SSW0JQG_3.0.x` suffix (e.g. `SSW0JQG_2.x/architecture/user-metrics.html?cp=SSW0JQG_3.0.x`). This was confirmed by inspecting live TOC responses — every product with multiple active versions exhibits this pattern. The cache key derivation MUST strip the `?cp=...` query parameter from the href before constructing the filesystem path, otherwise the same topic fetched at different times (once via v2.x entry, once via v3.x entry) would create two separate cache files. The `topic_filename` used in the cache key is the basename of the path component only, with the extension and any query string removed.

**Root-only URL behaviour:** When a URL has no `?topic=` slug (e.g. `https://www.ibm.com/docs/en/kubecost/self-hosted/3.x`), the TOC root `href` is the product's internal ID string (e.g. `SSW0JQG_3.0.x`). Confirmed by fetching root URLs for three different products — all return empty bodies. The Content API has no content for the product root node itself. The tool MUST handle this case explicitly:
- Detect that the root `href` is a bare ID (no path separator, no `.html` extension).
- Fall back to the first child topic in the TOC instead of failing.
- Log a `DEBUG` message explaining the fallback.
- Do NOT treat this as an error (exit code 0 if the first child fetches successfully).

Additionally, the first child of every product TOC is a `dummy_landing_page_id.html` "Welcome" node that also returns a 404. The fallback logic MUST skip stub/dummy hrefs and advance to the first real content topic.

**Empty-href TOC nodes:** Some TOC nodes have a genuinely empty `href` field (`"href": ""`). These are section-header nodes — pure navigation containers with no content page of their own (e.g. `IBM Integration SaaS / Administering / Administering access / IBM SaaS Console`, `topicId=module_*`). Confirmed by inspecting TOC data for `integration-saas-lib/integration-saas/saas` — at least one such node exists in a real product tree, and fetching an empty href produces a `400 Bad Request` from the Content API. The tool MUST skip any TOC node whose `href` is empty or blank before attempting a Content API call, logging it at `DEBUG` level as a skipped navigation node. This is distinct from the root-only fallback above.

---

## 12. Performance Baselines

Measured against the `urls-to-fetch-from-ibm-docs.txt` manifest (21 IBM Docs URLs + 1 raw URL, depth=2), using the Python spike implementation as a reference point.

| Scenario | URLs | Topics | Wall time | Notes |
|---|---|---|---|---|
| Cold run (warm TOC, no content cache) | 21 | ~165 | ~61s | All content fetched from CDN; no delay between requests in spike |
| Warm run (all content cached) | 21 | 165 | <1s | Pure filesystem reads; essentially instant |
| Partial warm (99% cached, 2 network) | 21 | 165 | ~1s | Realistic re-run scenario |

**Per-topic network time:** ~370ms average per content request (observed range 280–730ms).

**Implications for the Go implementation:**
- With `REQUEST_DELAY_MS=200` between requests, a 165-topic cold run takes approximately `165 × (370ms avg + 200ms delay) ≈ 94s`. For a 21-URL manifest at depth=2, this is the realistic worst case.
- Warm runs are unaffected by the delay — cached topics skip the network and the delay entirely.
- The `--delay 0` option disables the inter-request delay for users who want maximum speed and accept the risk.
- **Sequential processing is sufficient for v0.1.** Goroutine-based concurrency would improve cold run speed but introduces complexity around cache writes and rate limiting. Defer to post-v0.1.

**Error rate observed:** 2 errors out of 167 fetches (1.2%) — both caused by empty-href TOC nodes (§11), not network failures. After the empty-href fix is applied, the expected error rate on well-formed manifests is 0%.

---

## 13. Build and Release Requirements

- Language: **Go 1.23+**
- CLI framework: **Cobra** (`github.com/spf13/cobra`)
- Build: `CGO_ENABLED=0` for all Linux targets; standard `go build` for macOS/Windows.
- Multi-arch Docker: built with `docker buildx` and pushed as a multi-arch manifest (`linux/amd64` + `linux/arm64`).
- **GitHub Releases:** the CI pipeline publishes pre-built binaries and the multi-arch container image on every `v*` tag. Release notes are generated from the git log since the previous tag.
- Binary naming: `ibmdocs-linux-amd64`, `ibmdocs-linux-arm64`, `ibmdocs-darwin-arm64`, `ibmdocs-darwin-amd64`, `ibmdocs-windows-amd64.exe`.
- Each GitHub Release MUST include a `checksums.txt` (SHA-256) for all binary artefacts.
- Version information (`ibmdocs version`) injected at build time via `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`.
- The repository MUST contain a `Makefile` (or equivalent `justfile`) with targets: `build`, `build-all`, `docker`, `test`, `lint`, `release-dry-run`.

---

## 14. Testing Requirements

- Unit tests for: URL parsing, slug matching (with real TOC fixture data from the spike's `doc-cache/toc/`), cache key derivation, index read/write, frontmatter extraction, empty-href detection, root-href fallback logic.
- Integration tests (optional, gated behind a build tag `//go:build integration`): real IBM CDN API calls, skipped in CI by default.
- Performance regression test: warm-cache `fetch-file` on the standard 21-URL manifest MUST complete in under 3 seconds (verifies no accidental network calls on cache hits).
- Target coverage: ≥ 80% on `internal/` packages.

---

## 15. Logging

- All progress and diagnostic output goes to **stderr**.
- Default log level: `INFO` (fetch start/end, cache hit/miss, topic counts).
- `--verbose` enables `DEBUG` level (TOC walk steps, slug matching decisions, cache key derivation). HTTP bodies are NOT logged at DEBUG level — use `--http-debug` for full HTTP traffic.
- No third-party logging library required; `log/slog` (stdlib since Go 1.21) is sufficient.

---

## 16. Open Questions / Deferred Items

| # | Item | Status |
|---|---|---|
| 1 | `search` command — validate `/docs/en/products` API shape | ✅ Resolved — use `1.www.s81c.com/docs/api/v1/search`; `/docs/en/products` returns 403; `www.ibm.com` search requires browser cookies |
| 2 | Support for `?lang=` other than `en` — confirm IBM CDN availability | ✅ Resolved — see §17 |
| 3 | `MAX_TOPICS_PER_LEVEL` — assess whether 5 is too conservative for deep product trees | ✅ Resolved — raised to 10; API Gateway has 16-child nodes where limit=5 dropped substantive API families |
| 4 | Cache eviction command (`ibmdocs cache purge [--product]`) | ⏳ Post-v0.1 |
| 5 | Shell completion generation (`ibmdocs completion bash/zsh/fish`) | ⏳ Post-v0.1 (Cobra provides this for free) |
| 6 | `trim --max-kb-size` calibration — what sizes are practical for common LLM context windows | ⏳ Post-v0.1 |
| 7 | Search API pagination — confirm `next`/`start` cursor behaviour for `--all` flag | ✅ Resolved — `next = start + limit`, iterate until `next >= hits` |
| 8 | Search version deduplication — `--latest-only` implementation: sort `productBreadCrumb` version segment descending, keep first per title | ⏳ v0.1 implementation detail |
| 9 | `--max-topics` flag to disable `MAX_TOPICS_PER_LEVEL` cap (pass `0` for unlimited) | ✅ Resolved — added as named constant and flag in §8.1 |
| 10 | CDN URL stability monitoring — consider periodic automated check (e.g. GitHub Actions weekly probe) to detect hostname/path changes before users report failures | ⏳ Post-v0.1 |
| 11 | Conditional GET optimisation — use `ETag`/`If-None-Match` to skip re-downloading unchanged content during `refresh` | ⏳ Post-v0.1 |
| 12 | `search` result `product.key` is an internal ID (e.g. `SSXAAZY`), NOT the friendly TOC key (e.g. `wm-apigateway-ipaas`); extract friendly key from `fullurl` field instead | ✅ Resolved — see §5.8 note and §7.4 |
| 13 | `suggest` API `href` field maps to internal product family IDs; only some resolve via TOC API; use `search` `fullurl` as the reliable product key source | ✅ Resolved — `suggest` is useful for autocomplete hints only, not for fetching |
| 14 | `build-kb` frontmatter extraction across non-API products (Kubecost): 0 API topics detected — correct, no false positives | ✅ Resolved — regexes are product-agnostic and safe; Kubecost content correctly classified as 0 api / 26 doc |
| 15 | INDEX.md topic names contain `?cp=SSW0JQG_3.0.x` suffixes leaked from cache keys — cosmetic but should be stripped for readability | ⏳ v0.1 fix |
| 16 | Root URL with no `?topic=` slug returns empty body from Content API; fallback to first non-dummy child required | ✅ Resolved — specified in §11 |
| 17 | Empty-href TOC nodes (`"href": ""`) cause 400 on Content API; must skip before fetching | ✅ Resolved — specified in §11 |

---

## 18. HTTP Debug Dump (`--http-debug`)

When the global flag `--http-debug` is set, every outbound HTTP request and its corresponding response is written to `<data>/debug/` as a pair of plain-text files.

**File naming:**

```
<data>/debug/
  20250727-143201-GET-1.www.s81c.com-docs-api-v1-toc-wm-integration-ipaas.req.txt
  20250727-143201-GET-1.www.s81c.com-docs-api-v1-toc-wm-integration-ipaas.res.txt
```

- Timestamp prefix: `YYYYMMDD-HHMMSS` (UTC, second granularity).
- URL path segments are sanitised: `/` → `-`, query strings omitted from the filename (but present in the file body).
- Filename is truncated at 200 characters to avoid filesystem limits.

**`.req.txt` format:**

```
GET /docs/api/v1/toc/wm-integration-ipaas?lang=en HTTP/1.1
Host: 1.www.s81c.com
User-Agent: Mozilla/5.0 ...
Accept: */*
Referer: https://www.ibm.com/docs/

[no body]
```

**`.res.txt` format:**

```
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 48291
...

{"toc": {"label": "webMethods Integration", ...}}
```

**Constraints:**
- `--http-debug` is a global flag; it applies to ALL requests made during the command invocation.
- Response bodies are written in full (no truncation) — this is intentional for debugging IBM CDN responses.
- The `debug/` directory is NOT cleaned automatically. Operators must manage its contents. A `--clear-debug` flag on any command or a standalone `ibmdocs debug clear` subcommand is a post-v0.1 convenience item.
- Debug files are never committed to git by default (add `<data>/debug/` to `.gitignore` in project scaffolding).
- `--http-debug` MUST NOT be active simultaneously with `--quiet`; the combination is a configuration error (exit code 2).

---

## 17. Language Support (`--lang`)

Validated by calling the TOC, Content, and Search APIs with seven different `lang` values against two products with different translation coverage (webMethods Integration — fully translated into multiple languages; Kubecost — English only).

### 17.1 Behaviour by API

| API | Behaviour |
|---|---|
| **TOC** | Returns translated TOC node labels when translation exists. Product name (`toc.label`) stays in English regardless of `lang`. Only IBM-authored products with official translations have translated TOC labels (e.g. webMethods Integration has `fr`, `de`, `ja`, `es`; Kubecost returns English for all non-`en` values — it has no translations). |
| **Content** | Returns fully translated HTML body when a translation exists. For IBM Integration SaaS: French body is ~11% larger than English (longer sentences); German similarly. Japanese and Chinese bodies are shorter (character density). For Kubecost: the CDN silently returns the English body for all non-`en` `lang` values — there is no error, just identical content. The only change is that internal `href` links within the body have their `/docs/en/` prefix replaced with `/docs/{lang}/`, even when the content itself is English. |
| **Search** | Returns hits from the language-specific index. `lang=fr` returns 149 hits for "kubecost" vs 668 for `lang=en`; results include French-language topic titles (`Intégration avec IBM Kubecost`). The `--lang` flag on `search` narrows the result set to topics that have been translated into that language — this is intentional and useful. |

### 17.2 Silent fallback behaviour

**The CDN never returns an error for an unsupported language.** When a product has no translation for the requested `lang`, it silently returns the English content. This is consistent across all three APIs and all tested language codes. The tool MUST NOT special-case or validate the `--lang` value before sending — pass it through as-is and let the CDN respond. The user is responsible for knowing whether their target product has translations.

### 17.3 Confirmed supported language codes

Tested against `wm-integration-ipaas` (IBM-translated product):

| Code | Language | TOC translated | Content translated |
|---|---|---|---|
| `en` | English | ✅ (source) | ✅ (source) |
| `fr` | French | ✅ | ✅ |
| `de` | German | ✅ | ✅ |
| `ja` | Japanese | ✅ | ✅ |
| `zh-cn` | Simplified Chinese | ❌ (Welcome not translated) | ✅ |
| `pt-br` | Brazilian Portuguese | ❌ (Welcome not translated) | ✅ |
| `es` | Spanish | ✅ | ✅ |

Kubecost returns English content silently for all non-`en` codes — no TOC label translation, no body translation. This is product-specific, not an API limitation.

### 17.4 Link rewriting side-effect

When `lang=fr` is passed, internal cross-reference `href` attributes within the returned HTML change from `/docs/en/...` to `/docs/fr/...` — **even when the body text is English**. The HTML-to-Markdown conversion MUST NOT attempt to normalise these link paths. They should be preserved as-is, since they are valid IBM Docs browser URLs that will serve the appropriate language version (or fall back to English) when a user clicks them.

### 17.5 Cache key includes lang

Cache entries are per-language: `content:{product_key}/{topic_filename}:{lang}`. Fetching the same topic in English and French produces two separate cache files (`en.json` and `fr.json`). This is correct and intentional.

### 17.6 Performance note

The `--lang` flag has no observable effect on CDN response latency. Translated and English responses are returned in equivalent time (~300–500ms per content request).
