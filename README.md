# ibmdocs — IBM Documentation Fetcher CLI

A portable, statically-linked command-line tool that fetches IBM product
documentation from the IBM Docs public API, caches it locally, and exports
it as agent-ready Markdown.

**No runtime dependencies.** No Go, no Python, no Docker required to run the
binary. One file, drop it anywhere.

---

## Provenance

This project was built entirely with [IBM Bob](https://www.ibm.com/products/ibm-bob),
IBM's AI software engineering assistant.

**How it came to exist:**

1. **Exploratory spike** — The user collaborated with Bob to build
   [`1c-spike-ibm-doc-fetcher`](../1c-spike-ibm-doc-fetcher), a Python proof-of-concept
   that validated the IBM Docs CDN API (`1.www.s81c.com`), confirmed the three
   unauthenticated endpoints (TOC, Content, Search), and established all edge cases
   (empty hrefs, root-ID fallback, `?cp=` cross-version parameters, slug-matching
   algorithm, HTML noise stripping). The spike also produced the performance baseline
   used in the requirements.

2. **Product requirements** — Through a structured series of exploration exercises,
   the user drove a detailed conversation with Bob to turn spike findings into a
   complete, self-contained `PRODUCT_REQUIREMENTS.md` (18 sections). Every
   implementation decision in the code traces back to a rationale documented there.

3. **Implementation** — Bob generated the entire Go codebase from the requirements
   document: module scaffold, devcontainer, Dockerfile, Makefile, all internal
   packages (`config`, `cache`, `fetcher`, `ibmdocs`, `kb`), all Cobra commands,
   and the unit test suite. No code in this repository was written by hand.

The requirements document (`PRODUCT_REQUIREMENTS.md`) is the single source of
truth and remains in the repository as a living specification.

---

## What it does

IBM Docs is a browser SPA backed by a clean REST API on `1.www.s81c.com`.
`ibmdocs` drives that API directly:

1. Parses a standard IBM Docs browser URL to extract the product key and topic slug.
2. Fetches the product Table of Contents (TOC) and resolves the slug to a content href.
3. Fetches the HTML fragment for the topic and converts it to clean Markdown.
4. Optionally recurses through child topics in the TOC up to a configurable depth.
5. Caches everything locally (JSON, 24 h TTL) so re-runs are instant.
6. Exports the cache as agent-ready Markdown with YAML frontmatter under `kb/`.

The Markdown output is designed to be fed directly to an LLM context window,
a RAG loader, or committed to a repository alongside your project.

---

## Installation

### Pre-built binary (recommended)

Download the binary for your platform from [GitHub Releases](../../releases)
and place it on your `PATH`:

```sh
# macOS Apple Silicon
curl -Lo ibmdocs https://github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/releases/latest/download/ibmdocs-darwin-arm64
chmod +x ibmdocs && mv ibmdocs /usr/local/bin/

# macOS Intel
curl -Lo ibmdocs .../ibmdocs-darwin-amd64
chmod +x ibmdocs && mv ibmdocs /usr/local/bin/

# Linux amd64
curl -Lo ibmdocs .../ibmdocs-linux-amd64
chmod +x ibmdocs && mv ibmdocs /usr/local/bin/
```

| Platform | Binary |
|---|---|
| Linux x86\_64 | `ibmdocs-linux-amd64` |
| Linux ARM64 | `ibmdocs-linux-arm64` |
| macOS Apple Silicon | `ibmdocs-darwin-arm64` |
| macOS Intel | `ibmdocs-darwin-amd64` |
| Windows x86\_64 | `ibmdocs-windows-amd64.exe` |

### Container

The production image is built `FROM scratch` — no shell, no OS layer, no package
manager. The binary is the only process. All persistent state lives outside the
container via a bind-mounted data directory.

**Convention used in all examples below:**

```sh
# Create a local data directory once
mkdir -p ~/t/ibmdocs-data

# Alias for convenience (optional)
# --user $(id -u):$(id -g) ensures files written inside the container are
# owned by your host user (required for rootful Docker on Linux and macOS).
alias ibmdocs='docker run --rm \
  --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data'
```

With the alias every command below works identically to the native binary.
Without the alias prepend the following in place of `ibmdocs`:

```
docker run --rm --user $(id -u):$(id -g) -v ~/t/ibmdocs-data:/data ibmdocs:latest --data /data
```

> **Rootless Docker / Podman:** uid/gid mapping is automatic — omit `--user`.

### Build from source

Requires Docker (no local Go toolchain needed). The dev container runs as your
host user — `UID` and `GID` are passed so the Go caches written into `.cache/`
inside the repo are owned by you:

```sh
git clone https://github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli
cd 7u-ibm-doc-fetcher-cli

# Build the dev image once (or after Dockerfile changes)
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml build

# Build all five platform targets
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-all

# Binaries land in target/bin/
```

---

## Quick start

> **Using the container?** Replace `ibmdocs` with the full `docker run` form shown
> in each example, or set the alias from the [Container](#container) section above.

### 1. Fetch a single topic

```sh
# Native binary
ibmdocs fetch "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis"

# Container — data written to ~/t/ibmdocs-data on the host
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  fetch "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis"
```

Fetches the topic, prints Markdown to stdout, writes cache + KB to the data folder.

### 2. Fetch a topic tree recursively

```sh
# Native binary
ibmdocs fetch \
  "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis" \
  --recursive --depth 2

# Container
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  fetch "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis" \
  --recursive --depth 2
```

Walks the TOC tree 2 levels deep, fetching up to 10 child topics per node.

### 3. Batch fetch from a URL manifest

```sh
# Native binary
ibmdocs fetch-file my-urls.txt --depth 2

# Container — manifest must be under a Docker-shared path (e.g. home directory)
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  -v ~/t/my-urls.txt:/manifest.txt:ro \
  ibmdocs:latest \
  --data /data \
  fetch-file /manifest.txt --depth 2
```

`my-urls.txt` format — blank lines and `#` comments are skipped:

```
# webMethods Integration public APIs
https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis

# Kubecost cost monitoring
https://www.ibm.com/docs/en/kubecost/self-hosted/3.x?topic=overview
```

### 4. Search IBM Docs

```sh
# Native binary — find products matching a keyword
ibmdocs search "api gateway policy management" --latest-only

# Container
docker run --rm ibmdocs:latest \
  search "api gateway policy management" --latest-only

# Pipe search URLs directly into fetch-file (native binary)
ibmdocs search "kubecost" --url-only --latest-only | ibmdocs fetch-file /dev/stdin

# Same pipe with container — write URLs to a file under a shared path first
docker run --rm ibmdocs:latest \
  search "kubecost" --url-only --latest-only > ~/t/ibmdocs-data/kubecost-urls.txt
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  fetch-file /data/kubecost-urls.txt --depth 2
```

> Note: `search` makes no writes to disk — no data mount needed for search-only runs.

### 5. Export cache to Markdown (build the KB)

```sh
# Native binary
ibmdocs build-kb

# Container
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  build-kb
```

Converts everything in `cache/content/` to `kb/<product>/<topic>/en.md` with
YAML frontmatter. Idempotent — safe to re-run. Writes per-product `INDEX.md`
files and a top-level `kb/INDEX.md`.

### 6. List what you have cached

```sh
# Native binary
ibmdocs list
ibmdocs list --stale --older-than 7
ibmdocs list --json | jq '.[].url'

# Container
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  list
```

### 7. Refresh stale entries

```sh
# Native binary
ibmdocs refresh --older-than 7
ibmdocs refresh --older-than 7 --dry-run   # preview first

# Container
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  refresh --older-than 7
```

### 8. Trim the KB to fit an LLM context budget

```sh
# Native binary
ibmdocs trim --stubs-only --dry-run
ibmdocs trim --stubs-only

# Container
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  trim --stubs-only --dry-run

# Trim to 500 KB total (stubs first, then oldest, then smallest)
docker run --rm --user $(id -u):$(id -g) \
  -v ~/t/ibmdocs-data:/data \
  ibmdocs:latest \
  --data /data \
  trim --max-kb-size 500 --dry-run
```

Cache is never touched — run `build-kb` at any time to restore the full KB.

---

## Data folder layout

```
ibmdocs-data/
  cache/
    toc/
      <product-key>/en.json          # TOC responses (24 h TTL)
    content/
      <product-key>/<topic>/en.json  # Content responses (24 h TTL)
  kb/
    <product-key>/
      <topic>/en.md                  # Agent-ready Markdown + YAML frontmatter
      _stubs_merged.md               # Thin navigation topics merged together
      INDEX.md                       # Per-product topic index
    INDEX.md                         # Top-level index across all products
  index.json                         # Fetch history (URL → fetched_at, depth, status)
  debug/                             # HTTP traffic dumps (--http-debug only)
```

The data folder is designed to be **committed to version control** — it contains
only JSON and Markdown, no binary blobs.

---

## Global flags

| Flag | Default | Description |
|---|---|---|
| `--data`, `-d` | `./ibmdocs-data` | Path to the data folder |
| `--lang` | `en` | Language code (e.g. `fr`, `de`, `ja`, `zh-cn`) |
| `--verbose`, `-v` | false | Debug-level logging to stderr |
| `--delay` | `200ms` | Inter-request delay (polite crawling; `0` to disable) |
| `--cdn-base-url` | `https://1.www.s81c.com` | CDN base URL (env: `IBMDOCS_CDN_BASE_URL`) |
| `--http-debug` | false | Dump every HTTP request/response to `<data>/debug/` |

---

## YAML frontmatter

Each `kb/` Markdown file begins with a YAML frontmatter block:

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

Fields are only emitted when detectable. Agents can filter by frontmatter keys
without reading the full body — useful for routing retrieval to API-reference
topics vs. concept/task documentation.

---

## API notes

- All three endpoints (`toc`, `content`, `search`) hit `1.www.s81c.com` — IBM's
  public CDN origin, no authentication required.
- `www.ibm.com/robots.txt` disallows `/docs/api` (browser proxy). The CDN-direct
  host has no Disallow rules.
- The CDN base URL is runtime-overridable via `--cdn-base-url` or
  `IBMDOCS_CDN_BASE_URL` so you can adapt without a new release if IBM restructures
  the origin.
- Product keys are multi-segment path strings (e.g.
  `integration-saas-lib/integration-saas/saas`) taken verbatim from the browser URL.
- `search` result `product.key` is an IBM internal ID — always use `fullurl` to
  derive the TOC product key.

---

## Development

The dev container runs as your host user so all build artefacts and Go caches
(written to `.cache/` inside the repo) remain yours. Prefix every
`docker compose` invocation with `UID=$(id -u) GID=$(id -g)`, or export them
once for your session:

```sh
export UID=$(id -u) GID=$(id -g)
```

```sh
# Build the dev image (once, or after .devcontainer/ibmdocs-dev01/Dockerfile changes)
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml build

# Interactive shell in the dev container
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev sh

# Run tests
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make test

# Run linter
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make lint

# Build single native binary (current OS/arch)
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build

# Build all five platform targets — binaries land in target/bin/
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-all

# Update Go dependencies (e.g. after editing go.mod)
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev go mod tidy
```

**Go cache location:** `GOCACHE`, `GOMODCACHE`, and `GOPATH` all resolve to
subdirectories of `.cache/` inside the repo root, so caches persist across
container runs and are owned by your host user. The `.cache/` folder is
git-ignored.

A VSCode devcontainer is provided at `.devcontainer/ibmdocs-dev01/` with the
Go extension, GitLens, and ShellCheck pre-configured.

---

## License

See [LICENSE](LICENSE).
