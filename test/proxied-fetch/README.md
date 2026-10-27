# proxied-fetch — integration test harness

Acceptance tests for the `--proxy` and `--tls-ca-bundle` flags.  
These tests run the **pre-built Linux binary** (no Go toolchain needed at test time)
against a fully self-contained Docker network — no real internet access required.

---

## What is tested

| Scenario | Proxy type | `--tls-ca-bundle` needed? | What it proves |
|---|---|---|---|
| **plain** | mitmproxy in CONNECT-passthrough mode (`--ignore-hosts .*`) | No | Binary reaches the target through a proxy without TLS interception |
| **tls** | mitmproxy in full MITM mode (default) | Yes (proxy CA) | Binary accepts re-signed certificates when the proxy CA is supplied |

Both scenarios use a **fake IBM Docs CDN** (nginx on HTTPS, self-signed cert)
that serves canned TOC, content, and search responses for product key `bob`.
All networks are Docker-internal — no traffic leaves the host.

---

## Architecture

```
                      ┌─────────────────────────────────────────┐
                      │            internet_sim network          │
                      │  (internal — no route to the host)      │
                      │                                         │
  plain_net  ┌──────────────┐   ┌─────────────────────────────┐  │
  ─────────► │ proxy_plain  │──►│                             │  │
             │ (passthrough)│   │  target  (nginx HTTPS :443) │  │
  tls_net    ├──────────────┤   │  hostname: 1.www.s81c.com   │  │
  ─────────► │  proxy_tls   │──►│                             │  │
             │ (MITM)       │   └─────────────────────────────┘  │
             └──────────────┘                                     │
                      └─────────────────────────────────────────┘

  run_plain ──(plain_net)──► proxy_plain ──(internet_sim)──► target
  run_tls   ──(tls_net)────► proxy_tls  ──(internet_sim)──► target
```

`run_plain` and `run_tls` are containers running the `ibmdocs:test` image
(the pre-built Linux binary in a `FROM scratch` image).  
`run_plain` only has access to `plain_net`; `run_tls` only to `tls_net`.  
Neither can reach `target` directly — they must go through their proxy.

---

## Prerequisites

- Docker with the Compose v2 plugin (`docker compose`)
- The pre-built Linux binary in `target/bin/`

Build it if needed (from the repo root):

```sh
# arm64 (Apple Silicon / AWS Graviton)
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-arm64

# amd64
UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-amd64
```

---

## Running the tests

```sh
# From the repo root:
./test/proxied-fetch/run.sh
```

`run.sh` will:

1. Detect your CPU architecture and select the matching Linux binary.
2. Build `ibmdocs:test` from the binary (no compilation — just wraps it in a `scratch` image).
3. Start `target`, `proxy_plain`, and `proxy_tls` and wait for them to be healthy.
4. Run **scenario 1** (`run_plain`) — asserts exit code 0 and that KB files were written.
5. Run **scenario 2** (`run_tls`) — asserts exit code 0 and that KB files were written.
6. Print a pass/fail summary and clean up all containers and volumes.

Exit code is `0` if both scenarios pass, `1` if either fails.

---

## Directory layout

```
test/proxied-fetch/
  run.sh                  ← test driver (run this)
  docker-compose.yml      ← service definitions
  ibmdocs.Dockerfile      ← builds ibmdocs:test from pre-built binary
  proxy/
    Dockerfile            ← mitmproxy image; MITM_MODE=plain|tls
  target/
    Dockerfile            ← nginx HTTPS fake CDN
    nginx.conf            ← routes /docs/api/v1/... to static files
    responses/
      toc-bob.json                              ← TOC API response
      search-bob.json                           ← Search API response
      content/bob/overview-getting-started.html ← Content API response
      content/bob/what-is-bob.html              ← Content API response
```

---

## Debugging

Run individual services interactively from the `test/proxied-fetch/` directory:

```sh
# Bring up infrastructure only
docker compose up target proxy_plain proxy_tls

# Run plain scenario manually (override command as needed)
docker compose run --rm -v my_vol:/tmp/data run_plain

# Inspect proxy traffic (mitmproxy flow log)
docker compose logs proxy_tls

# Interactive shell in the ibmdocs:test image for ad-hoc testing
docker run --rm --network proxied-fetch_plain_net --entrypoint sh ibmdocs:test
```
