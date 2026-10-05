# ibmdocs — IBM Documentation, Directly Into Your Agent

---

Spending too much time with IBM docs?
Try out the `ibmdocs` CLI tool and the associated AI Agent skill today. Have sophisticated answers in minutes using your agent of choice with no intermediaries whatsoever!

---

## Part I — The Quick Case (for everyone)

### Four problems, one tool

**Problem 1: IBM Docs is vast and finding the right page takes time.**

IBM has hundreds of products, each with deep documentation trees — API references,
deployment guides, configuration references, release notes. The information is there,
maintained by IBM, publicly accessible, no account required. But navigating to the right
section of the right product version, reading it, and extracting what you need is slow.
You click through a TOC, land on a page that links to three others, follow the trail, and
ten minutes later you have an answer — or you gave up and guessed.

**Problem 2: Documentation pages are written for the page's question, not yours.**

Even when you find the right page, it is written to be comprehensive and general — it
covers every option, every edge case, every version caveat. It is not written for your
specific context: your version, your constraint, your immediate task. The answer you need
is usually scattered across three pages, two of which you haven't found yet. Reading and
synthesising that into something actionable takes as long as the navigation did.

**Problem 3: Asking your AI agent doesn't solve it — it trades one problem for another.**

AI agents can synthesise and reason across multiple sources, which is exactly what
problem 2 calls for. But they answer from training data. That data has a cut-off date,
may be incomplete, and describes versions that may not match what you are running. The
agent sounds confident. The answer may be subtly wrong. You still have to verify it
against the actual documentation — which brings you back to problems 1 and 2.

**Problem 4: Existing MCP and intermediary tools introduce their own costs.**

Several tools exist that try to bridge AI agents to live documentation. Most require a
server process running somewhere — in your environment, in a cloud, in a sidecar. That
is infrastructure to provision, secure, and maintain. Worse, they transform the
documentation through their own logic before it reaches the agent: chunking it, embedding
it, summarising it, filtering it. Every transformation is a lossy operation. The agent
does not read what IBM wrote — it reads what the tool decided IBM meant. Errors and
omissions introduced by the intermediary are invisible.

---

### The answer: minimum hops to the authoritative source

`ibmdocs` is a single binary. It fetches IBM Docs directly from IBM's own CDN API —
the same API the browser uses, no intermediary, no transformation layer beyond stripping
HTML navigation chrome and converting to clean Markdown. What the agent reads is what IBM
published, minus the surrounding page furniture.

No server to run. No service to manage. No third-party transformation. One binary, one
data folder, plain Markdown files on disk.

**Without this tool:** question → agent draws on training data → answer of uncertain currency, unverified.

**With this tool:** question → agent fetches IBM Docs directly → agent synthesises across
the fetched pages from your specific standpoint → answer grounded in IBM-published text,
version-stamped, inspectable, and shaped to your context.

The path from your question to the authoritative source is as short as it can be. The
synthesis that was always the expensive part — reading across pages, extracting what
applies to your case — is what the agent does once the right pages are in front of it.

---

### The minimum viable workflow

```sh
# 1. Find the right IBM Docs URL for a product
ibmdocs search "webmethods integration public apis" --latest-only

# 2. Fetch it (and two levels of child topics)
ibmdocs fetch "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis" \
  --recursive --depth 2

# 3. The agent now has IBM-sourced, grounded documentation to answer from
```

That is the entire workflow for a first fetch. Subsequent runs on the same topic return
from local cache instantly — no network, no CDN, no wait.

---

### Why it matters

| | `ibmdocs` | Manual browsing | Agent training data | MCP/intermediary tools |
|---|---|---|---|---|
| Source | IBM CDN directly | IBM Docs browser | Training cut-off | Third-party transformation |
| Accuracy | IBM-published text | IBM-published text | Uncertain | Lossy — depends on chunking/filtering logic |
| Speed (warm) | Instant (local cache) | Slow — manual navigation | Instant | Varies — server latency |
| Infrastructure | None — single binary | None | None | Server process required |
| Auditability | Versioned Markdown files | None | None | Opaque — transformation is not visible |
| Offline use | Yes — cache survives network loss | No | Yes | No — server dependency |

The cached knowledge base (`kb/`) is designed to be committed to version control alongside
your project. Your team shares the same documentation snapshot. You can diff it. You can
see when it was last refreshed. You know exactly what the agent was reading when it
generated a code change — and you can open the same file in your editor and read it
yourself.

---

### Getting started: two things to install

Two components work together and both are required:

**1. The `ibmdocs` binary** — this is the CLI that actually talks to IBM Docs, manages
the cache, and writes the Markdown knowledge base. Without it, the skill has nothing to
run. Download the binary for your platform from [GitHub Releases](../../releases) and
place it on your `PATH`:

```sh
# macOS Apple Silicon
curl -Lo ibmdocs https://github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/releases/latest/download/ibmdocs-darwin-arm64
chmod +x ibmdocs && mv ibmdocs /usr/local/bin/

# Linux amd64
curl -Lo ibmdocs .../ibmdocs-linux-amd64
chmod +x ibmdocs && mv ibmdocs /usr/local/bin/
```

Verify with `ibmdocs version`. Full platform table and container alias in the
[README installation section](../README.md#installation).

**2. The `ibmdocs` skill** — a Markdown file that tells your AI agent how to use the
binary. It encodes the seven-step retrieval protocol so the agent runs the workflow
automatically whenever an IBM product question arises.

The skill lives at [`util/skills/ibmdocs/SKILL.md`](../util/skills/ibmdocs/SKILL.md).
Drop it into your agent's skill directory. It is agent-agnostic — it works with IBM Bob,
Cursor, GitHub Copilot with tool use, or any local LLM that can execute shell commands
and read files.

The skill's Step 1 checks for the binary at startup. If `ibmdocs version` fails, the
agent surfaces the README installation link rather than silently proceeding — so a
missing binary is caught immediately, not mid-fetch.

---

---

## Part II — For the AI Engineer: Advanced Usage

*You know how agents and skills work. This section covers the full command surface,
KB management patterns, and how to get the most out of the tool in a real project
workflow — without detailing the implementation internals.*

---

### Full command reference

| Command | Purpose |
|---|---|
| `ibmdocs search "<term>" --latest-only` | Discover IBM Docs URLs for a product by keyword |
| `ibmdocs search "<term>" --url-only --latest-only` | URL-only output for piping into `fetch-file` |
| `ibmdocs fetch "<url>"` | Fetch a single topic |
| `ibmdocs fetch "<url>" --recursive --depth 2` | Fetch topic + 2 levels of TOC children (up to 10 per node) |
| `ibmdocs fetch "<url>" --recursive --refresh` | Force re-fetch, bypassing the cache TTL |
| `ibmdocs fetch "<url>" --recursive --cache-ttl 24h` | Treat anything older than 24h as stale for this run only |
| `ibmdocs fetch-file <manifest.txt> --depth 2` | Batch fetch from a URL manifest (one URL per line, `#` comments ok) |
| `ibmdocs build-kb` | Rebuild all KB Markdown from cached JSON — idempotent, no network calls |
| `ibmdocs list` | Show what is cached (URL, fetched_at, topic count, status) |
| `ibmdocs list --stale --older-than 7` | Show entries older than 7 days |
| `ibmdocs list --json \| jq '.[].url'` | Machine-readable cache inventory |
| `ibmdocs refresh --older-than 7` | Re-fetch all entries whose index record is older than N days |
| `ibmdocs refresh --older-than 7 --dry-run` | Preview what would be re-fetched without doing it |
| `ibmdocs trim --stubs-only` | Remove thin navigation stub files from the KB |
| `ibmdocs trim --max-kb-size 500` | Trim KB to 500 KB total (stubs first, then oldest, then smallest) |
| `ibmdocs trim --dry-run` | Preview what trim would remove |
| `ibmdocs version` | Print version, commit, and build date |

Global flags available on every command:

| Flag | Default | Description |
|---|---|---|
| `--data`, `-d` | `./ibmdocs-data` | Path to the data folder |
| `--lang` | `en` | Language code (`fr`, `de`, `ja`, `zh-cn`, `es`, `pt-br`, …) |
| `--verbose`, `-v` | false | Debug-level logging to stderr |
| `--delay` | `200ms` | Inter-request delay (polite crawling; `0` to disable) |
| `--cache-ttl` | `720h` (30 days) | Cache validity window; `0` always re-fetches; env: `IBMDOCS_CACHE_TTL` |
| `--cdn-base-url` | `https://1.www.s81c.com` | CDN base URL override; env: `IBMDOCS_CDN_BASE_URL` |
| `--http-debug` | false | Dump full HTTP request/response pairs to `<data>/debug/` |

---

### KB management patterns

**Pipe search into batch fetch:**

```sh
# Discover and fetch in one pipeline
ibmdocs search "kubecost" --url-only --latest-only | ibmdocs fetch-file /dev/stdin --depth 2
```

**Commit the KB to version control:**

The `kb/` and `cache/` directories contain only JSON and Markdown. Commit them. Your
team then shares the same documentation snapshot and no one needs to run a cold fetch.
The `requests.log` and `debug/` directories should be gitignored.

**Keep the KB fresh on a schedule:**

```sh
# Re-fetch anything older than a week, then rebuild KB
ibmdocs refresh --older-than 7 && ibmdocs build-kb
```

**Control KB size for context budget:**

```sh
# Remove stub-only navigation files first (safe — cache is untouched)
ibmdocs trim --stubs-only

# Hard budget: trim to 500 KB total
ibmdocs trim --max-kb-size 500 --dry-run   # preview first
ibmdocs trim --max-kb-size 500

# Restore the full KB from cache at any time
ibmdocs build-kb
```

**Multi-language knowledge bases:**

```sh
# Fetch the same product in French (separate cache keys, separate kb/ files)
ibmdocs fetch "<url>" --recursive --depth 2 --lang fr --data ./ibmdocs-data-fr
```

---

### The skill protocol

The skill at [`util/skills/ibmdocs/SKILL.md`](../util/skills/ibmdocs/SKILL.md) encodes
seven steps the agent follows automatically:

1. **Confirm binary** — `ibmdocs version`; surface installation link if missing.
2. **Identify product and topic** — from the user's request.
3. **Discover URL** — `ibmdocs search "<term>" --latest-only` if no direct URL is known.
4. **Fetch** — `ibmdocs fetch "<url>" --recursive --depth 2`.
5. **Build KB** — `ibmdocs build-kb` (idempotent; skipped if fetch already wrote KB files).
6. **Read** — navigate `kb/INDEX.md`, load relevant `en.md` files into context in full.
7. **Answer grounded in KB** — cite KB content, provide clickable IBM Docs browser links.

The skill is intentionally agent-agnostic. It references "the terminal/shell capability
available to the agent" and "the file-reading capability available to the agent" rather
than naming any specific platform. The same `SKILL.md` works unchanged in IBM Bob,
Cursor, Copilot Workspace, and local open-weight model tool-use setups.

The agent always links users back to canonical IBM Docs browser URLs
(`https://www.ibm.com/docs/{lang}/{product}?topic={slug}`), derived from KB frontmatter
at answer time — never raw CDN API URLs that are not human-navigable.

---

### Performance characteristics

| Scenario | Typical wall time | Network calls |
|---|---|---|
| Warm run, all topics cached | < 1 second | 0 |
| Partial warm (1–2 stale topics) | ~1 second | 1–2 |
| Cold run, 165 topics, `--delay 200ms` | ~94 seconds | 166 (1 TOC + 165 content) |
| `build-kb` re-run from warm cache | ~2–5 seconds | 0 |
| `search` (no data mount) | ~300ms | 1 |

The 94-second cold-run is the realistic worst case for a large IBM product at `--depth 2`
across a 21-URL manifest. Every subsequent run on the same topic set costs nothing — the
30-day TTL means the cache stays valid for a month of daily development work without any
CDN traffic. The warm-run performance regression test (< 3 seconds for 21 URLs) is part
of the test suite.

---

---

## Part III — Under the Hood and Contributing

*How `ibmdocs` works internally, why the key design decisions were made, and how to
build, extend, or contribute to the project.*

---

### How the data pipeline works

```
IBM Docs browser URL
       │
       ▼
  ParseIBMDocsURL()          ← extracts product key + topic slug from URL
       │
       ▼
  Fetch TOC (cached)         ← GET /docs/api/v1/toc/<product-key>
       │
       ▼
  SlugMatches()              ← four-strategy algorithm: exact → normalised →
       │                        progressive suffix → bidirectional containment
       ▼
  Fetch Content (cached)     ← GET /docs/api/v1/content/<href>?parsebody=true
       │
       ▼
  stripNoise() + HTML→MD     ← removes navigation chrome, lastModifiedDate,
       │                        related-links; converts to clean Markdown
       ▼
  cache/content/<product>    ← raw JSON response, 30-day TTL, atomic writes
  kb/<product>/<topic>/en.md ← agent-ready Markdown + YAML frontmatter
```

Recursive fetch walks the TOC tree from the matched node downward, up to `--depth`
levels and `--max-topics` children per node (default 10). The TOC is fetched once per
product per TTL window regardless of depth.

---

### The IBM Docs CDN API

`ibmdocs` drives three unauthenticated endpoints on `1.www.s81c.com` — IBM's public CDN
origin, the same host the browser SPA uses:

| Endpoint | Path | Purpose |
|---|---|---|
| TOC | `/docs/api/v1/toc/<product-key>?lang=<lang>` | Full Table of Contents tree for a product |
| Content | `/docs/api/v1/content/<href>?parsebody=true&lang=<lang>` | HTML fragment for a single topic |
| Search | `/docs/api/v1/search?query=<q>&lang=<lang>` | Search across all IBM Docs products |

No authentication. No rate-limiting headers observed. The `User-Agent`, `Referer`, and
`Accept` headers match a standard browser request. `robots.txt` compliance is deliberate:
`www.ibm.com/robots.txt` disallows `/docs/api` (the browser proxy path), but the
CDN-direct host has no Disallow rules. Polite crawling delay (default 200ms) is enforced
and configurable.

The CDN base URL is runtime-overridable via `--cdn-base-url` or `IBMDOCS_CDN_BASE_URL`
so the tool adapts without a new release if IBM restructures the origin.

---

### The cache and knowledge base design

Everything persistent lives in a single `--data` directory:

```
ibmdocs-data/
  cache/
    toc/<product-key>/en.json             ← TOC responses (30-day TTL)
    content/<product-key>/<topic>/en.json ← Content responses (30-day TTL)
  kb/
    <product-key>/<topic>/en.md           ← Agent-ready Markdown + YAML frontmatter
    <product-key>/_stubs_merged.md        ← Thin navigation topics merged into one file
    <product-key>/INDEX.md
    INDEX.md
  index.json                              ← Fetch history (URL → fetched_at, depth, status)
  requests.log                            ← Append-only JSON Lines audit log of CDN requests
  debug/                                  ← Full HTTP request/response dumps (--http-debug only)
```

Cache keys strip the `?cp=` cross-version parameter so fetching the same topic at
different version paths reuses the same cache entry. Cache writes are atomic (write to a
temp file, then rename) so a crash mid-write never leaves a corrupt entry.

The `kb/` tree is designed to be **committed to version control** — JSON and Markdown
only, no binary blobs. A team that commits it alongside their project gets a reproducible,
auditable documentation snapshot that any agent can load without running `ibmdocs` at all.

**The KB is also a faster reading experience for humans.** The browser wraps every topic
in a full SPA shell: left-hand TOC, breadcrumbs, feedback widgets, related-links panels,
version banners. None of that travels into the KB — the conversion retains only the topic
body. The result is a compact, plain-text view of exactly the sub-domain you fetched, one
file per topic, browsable with any editor or `grep`, with no adjacent content pulling
focus away from the task at hand.

---

### The YAML frontmatter

Every `kb/` file begins with a YAML frontmatter block that both agents and humans can
use to navigate without reading the full body:

```yaml
---
product: "wm-integration-ipaas"
topic: "wmint_public_apis"
last_updated: "2026-09-16"
http_methods:
  - "GET"
  - "POST"
api_paths:
  - "/apis/v1/rest/projects/:project/workflows"
auth:
  - "x-instance-api-key"
  - "Bearer"
requires_admin: true
stub: false
---
```

Fields are only emitted when detectable from the HTML content. An agent uses them to
route retrieval intelligently: read `stub: false` topics for substantive content, use
`http_methods` and `api_paths` to find API reference pages, skip navigation stubs. A
human scanning the KB with a text editor gets the same benefit — the frontmatter is a
machine-readable and human-readable index at the top of every file.

---

### Binary characteristics

- **Language:** Go 1.23+, compiled with `CGO_ENABLED=0`.
- **Static linking:** no dependency on OS `libc`, `libssl`, `libxml`, or any shared library.
- **Container:** `FROM scratch` final image — binary + CA certificates only, no shell, no OS layer.
- **Platforms:** `linux/amd64`, `linux/arm64`, `darwin/arm64`, `darwin/amd64`, `windows/amd64`.
- **Dependencies:** Cobra (CLI framework), `golang.org/x/net/html` (HTML parsing), `github.com/JohannesKaufmann/html-to-markdown`. No database, no HTTP server, no background goroutines, no vector embedding.
- **Build:** single `Makefile` target; no local Go toolchain required — dev container via `dev-compose.yml`.

---

### Security posture — SBOM and vulnerability analysis

The `FROM scratch` container design has a direct, measurable security benefit. Because
the final image contains only the statically-linked binary and the CA certificate bundle
— no OS, no shell, no package manager, no libc — the attack surface is minimal by
construction. An SBOM analysis of the container image (CycloneDX 1.7, generated with
Trivy 0.75.0) confirms this:

| | |
|---|---|
| **Total components** | 9 |
| **Critical vulnerabilities** | 0 |
| **High vulnerabilities** | 0 |
| **Medium vulnerabilities** | 0 |
| **Low vulnerabilities** | 0 |
| **Informational findings** | 0 |
| **Max EPSS score** | 0.0 |
| **CISA KEV entries** | 0 |

Zero vulnerabilities at any severity across the entire image. There is no OS layer to
patch, no shell interpreter to exploit, no unused system library carrying a CVE. The
image is exactly as small as the purpose requires, and nothing more.

The 9 components counted by the SBOM are the Go standard library modules and direct
dependencies compiled into the binary — Cobra, the HTML parser, the Markdown converter,
and the Go runtime itself. None carry known vulnerabilities at the time of analysis.

---

### Why Go, why a CLI, why not an MCP server

The design is deliberately a batch CLI rather than a long-lived service:

- **No daemon, no port, no process to manage.** Run it, get files, done.
- **The KB is the integration surface.** Agents read Markdown files — the same interface
  available to every other tool that reads files. No protocol binding, no agent-specific
  adapter required.
- **Static binary maximises portability.** The same binary runs in a CI pipeline, a
  devcontainer, a macOS terminal, and a Windows developer machine without installation
  ceremony.
- **Cache reuse is the primary cost control.** A single cold fetch populates the KB for
  30 days. The cost of the batch run is amortised across every question asked in that
  window. An always-on service would tempt per-request live fetches and remove this
  amortisation entirely.

The decision to use Go over Python (the language of the original spike) was driven by the
static binary requirement: a Go binary compiled with `CGO_ENABLED=0` runs anywhere with
no runtime. A Python tool requires a Python interpreter and dependency management at every
deployment target — unacceptable for a tool that needs to drop into arbitrary CI
environments and developer machines.

---

### Provenance

This tool was built entirely with [IBM Bob](https://www.ibm.com/products/ibm-bob), IBM's
AI software engineering assistant. The process followed a deliberate sequence:

1. **Python spike** to validate the IBM Docs CDN API endpoints, edge cases, and performance.
2. **Structured requirements** ([`PRODUCT_REQUIREMENTS.md`](../PRODUCT_REQUIREMENTS.md),
   19 sections) capturing every design decision before a line of Go was written.
3. **Full Go implementation** generated by Bob from the requirements document — module
   scaffold, all internal packages, all Cobra commands, test suite, Dockerfile, Makefile.

No code in this repository was written by hand. The `PRODUCT_REQUIREMENTS.md` is the
single source of truth and remains a living specification.

---

### Contributing

The codebase is intentionally small and easy to navigate:

```
cmd/          ← one file per Cobra subcommand (fetch, fetch-file, build-kb, list, …)
internal/
  config/     ← CLI flag structs, defaults
  cache/      ← cache read/write, TTL, atomic writes
  fetcher/    ← URL parser, TOC walker, slug matcher
  ibmdocs/    ← CDN API client, HTML fetcher, request logger
  kb/         ← HTML→Markdown conversion, noise stripping, frontmatter, KB writer
util/
  skills/ibmdocs/SKILL.md  ← agent skill definition
```

**To build locally** (Docker required, no local Go toolchain needed):

```sh
git clone https://github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli
cd 7u-ibm-doc-fetcher-cli

# Build the dev image once
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml build

# Run tests
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make test

# Run linter
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make lint

# Build all five platform targets → target/bin/
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-all
```

The requirements document (`PRODUCT_REQUIREMENTS.md`) is the specification. Every design
decision in the code traces back to a rationale documented there. If you are adding a
feature or fixing a behaviour, update the requirements first — the code is the
implementation of the spec, not the other way around.
