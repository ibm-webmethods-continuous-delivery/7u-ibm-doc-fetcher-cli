---
name: ibmdocs
description: Use when the user asks a question about an IBM product, or before implementing requirements that involve IBM product deployment or operation — fetches authoritative IBM documentation into the local KB using the ibmdocs CLI, then reads the resulting Markdown files into context before answering or coding.
---

# ibmdocs — Fetch IBM Documentation Before Answering

Run this skill whenever a question or implementation task involves specific IBM product behaviour,
APIs, configuration, or deployment that cannot be answered reliably from training data alone.
The goal is to pull the relevant IBM Docs pages into the local `kb/` folder first, then ground
every answer or code change in that fetched content.

---

## Step 1 — Confirm the `ibmdocs` binary is available

Run:

```sh
ibmdocs version
```

If the command is not found, check whether the binary is available under a path-local alias
(`./ibmdocs`, `target/bin/ibmdocs`, etc.) or whether the container alias has been configured.
Refer the user to the [README installation section](../../../README.md#installation) if the
binary is missing before continuing.

Use `execute_command` to run all CLI calls in this skill. Set `cwd` to the project's
`ibmdocs-data` folder (or wherever `--data` points) when the command writes persistent state.

---

## Step 2 — Identify the IBM product and topic

From the user's request, determine:

1. **Which IBM product(s)** are involved (e.g. `webMethods Integration iPaaS`, `Kubecost`,
   `IBM MQ`, `Db2 on Cloud`).
2. **Which topic area** is needed (e.g. public APIs, deployment, configuration, monitoring).

If you already have a direct IBM Docs browser URL for the topic, skip to Step 4.

---

## Step 3 — Discover the right URL with `search`

If no URL is known, search IBM Docs to find the relevant product page:

```sh
ibmdocs search "<keyword or product name>" --latest-only
```

- Review the output. Each result includes a `fullurl` field — that is the URL to pass to `fetch`.
- If the results are too broad, refine the search term and repeat.
- Prefer `--latest-only` to avoid fetching deprecated product versions.
- For URL-only output suitable for piping:

```sh
ibmdocs search "<keyword>" --url-only --latest-only
```

> `search` makes no writes to disk — safe to run without a `--data` mount.

---

## Step 4 — Fetch the documentation

### Single topic

```sh
ibmdocs fetch "<ibm-docs-url>"
```

### Topic with child pages (recommended for implementation tasks)

```sh
ibmdocs fetch "<ibm-docs-url>" --recursive --depth 2
```

- `--depth 2` walks 2 levels of child topics in the TOC (up to 10 children per node).
- Increase depth only if the first fetch clearly misses required sub-topics.
- The fetch prints Markdown to stdout and writes cache + KB files under the data folder.

### Multiple topics from a manifest file

Create a plain-text file with one IBM Docs URL per line (`#` lines and blank lines are ignored),
then run:

```sh
ibmdocs fetch-file <manifest.txt> --depth 2
```

---

## Step 5 — Build the KB (if not already done by `fetch`)

`fetch` and `fetch-file` automatically populate `kb/`. If you ran `fetch` with `--recursive` you
can skip this step. Otherwise, or after a `refresh`, regenerate the Markdown files explicitly:

```sh
ibmdocs build-kb
```

This converts every cached JSON response under `cache/content/` into a `kb/<product>/<topic>/en.md`
file with YAML frontmatter. Idempotent — safe to re-run at any time.

---

## Step 6 — Locate and read the fetched Markdown

The KB files live at:

```
<data-dir>/kb/<product-key>/<topic-slug>/en.md
```

A top-level index is at `<data-dir>/kb/INDEX.md` and per-product indexes at
`<data-dir>/kb/<product-key>/INDEX.md`.

Steps to load content into context:

1. Read `<data-dir>/kb/INDEX.md` with `read_file` to see which products and topics are available.
2. Navigate to the relevant `en.md` files and read them with `read_file`.
3. Filter by YAML frontmatter fields if you only need specific sub-types (e.g. `stub: false` for
   full pages, `http_methods` for API reference topics).

Do not summarise the fetched content — read it in full so that answers and code are grounded in
the exact wording and structure IBM publishes.

---

## Step 7 — Answer or implement, citing the KB

- Base every factual claim, configuration value, API path, or code snippet on the KB content just
  read — not on training-time assumptions.
- If a topic is missing from the KB after fetching, tell the user and suggest a more specific URL
  or search term before proceeding.
- If the KB content is stale (older than 7 days for fast-moving products), refresh first:

```sh
ibmdocs refresh --older-than 7
ibmdocs build-kb
```

---

## Quick-reference: key CLI commands

| Command | Purpose |
|---|---|
| `ibmdocs search "<term>" --latest-only` | Discover IBM Docs URLs for a product |
| `ibmdocs fetch "<url>"` | Fetch a single topic |
| `ibmdocs fetch "<url>" --recursive --depth 2` | Fetch topic + 2 levels of children |
| `ibmdocs fetch-file <file> --depth 2` | Batch fetch from a URL manifest |
| `ibmdocs build-kb` | Rebuild all KB Markdown from cache |
| `ibmdocs list` | Show what is cached |
| `ibmdocs refresh --older-than 7` | Re-fetch entries older than N days |
| `ibmdocs trim --stubs-only` | Remove thin navigation stubs from the KB |

Global flags available on every command:

| Flag | Default | Description |
|---|---|---|
| `--data`, `-d` | `./ibmdocs-data` | Path to the data folder |
| `--lang` | `en` | Language code (`fr`, `de`, `ja`, `zh-cn`, …) |
| `--verbose`, `-v` | false | Debug-level logging to stderr |
| `--delay` | `200ms` | Inter-request delay (polite crawling) |
