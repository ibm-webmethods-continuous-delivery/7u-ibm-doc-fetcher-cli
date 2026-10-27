#!/bin/sh
# Integration test: ibmdocs binary through a proxy (plain and TLS-inspect).
#
# Prerequisites: Docker with Compose v2 plugin.
# The ibmdocs Linux binary must already be built:
#   UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-amd64
#   UID=$(id -u) GID=$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-arm64
#
# Usage (from repo root):
#   ./test/proxied-fetch/run.sh
#
# Exit codes:
#   0  all scenarios passed
#   1  one or more scenarios failed
set -eu

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TEST_DIR="$REPO_ROOT/test/proxied-fetch"
COMPOSE="docker compose -f $TEST_DIR/docker-compose.yml"
PASS=0
FAIL=0

# ── Colour helpers ────────────────────────────────────────────────────────────
green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n'  "$*"; }

# ── Detect host architecture to pick the right pre-built binary ───────────────
ARCH="$(uname -m)"
case "$ARCH" in
  arm64|aarch64) BINARY="$REPO_ROOT/target/bin/ibmdocs-linux-arm64" ;;
  *)             BINARY="$REPO_ROOT/target/bin/ibmdocs-linux-amd64" ;;
esac

if [ ! -f "$BINARY" ]; then
  red "ERROR: binary not found: $BINARY"
  echo "Build it first:"
  case "$ARCH" in
    arm64|aarch64) echo "  UID=\$(id -u) GID=\$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-arm64" ;;
    *)             echo "  UID=\$(id -u) GID=\$(id -g) docker compose -f dev-compose.yml run --rm dev make build-linux-amd64" ;;
  esac
  exit 1
fi

# ── Build the target image first (ibmdocs:test copies the CA cert from it) ───
bold "==> Building harness images"
$COMPOSE build target proxy_plain proxy_tls

bold "==> Building ibmdocs:test from pre-built binary"
# Must happen after target is built so COPY --from=proxied-fetch-target resolves.
# BINARY must be relative to the build context (repo root).
BINARY_REL="target/bin/$(basename "$BINARY")"
docker build \
  --build-arg BINARY="$BINARY_REL" \
  -t ibmdocs:test \
  -f "$TEST_DIR/ibmdocs.Dockerfile" \
  "$REPO_ROOT"

# ── Cleanup helper ────────────────────────────────────────────────────────────
cleanup() {
  bold "==> Cleaning up"
  $COMPOSE down --volumes --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

# ── Start shared infrastructure ───────────────────────────────────────────────
bold "==> Starting target and proxy services"
$COMPOSE up -d target proxy_plain proxy_tls

bold "==> Waiting for services to be healthy"
# poll up to 60 s
i=0
while [ $i -lt 30 ]; do
  target_ok=false; plain_ok=false; tls_ok=false
  [ "$(docker inspect --format='{{.State.Health.Status}}' ibmdocs_test_target    2>/dev/null)" = "healthy" ] && target_ok=true
  [ "$(docker inspect --format='{{.State.Health.Status}}' ibmdocs_test_proxy_plain 2>/dev/null)" = "healthy" ] && plain_ok=true
  [ "$(docker inspect --format='{{.State.Health.Status}}' ibmdocs_test_proxy_tls   2>/dev/null)" = "healthy" ] && tls_ok=true
  if $target_ok && $plain_ok && $tls_ok; then break; fi
  sleep 2
  i=$((i+1))
done
if ! ($target_ok && $plain_ok && $tls_ok); then
  red "ERROR: services did not become healthy in time"
  $COMPOSE logs target proxy_plain proxy_tls
  exit 1
fi

# ── run_scenario <name> <service> ────────────────────────────────────────────
# Runs the named docker-compose service, checks its exit code, and verifies
# that at least one KB Markdown file was written into the data volume.
run_scenario() {
  name="$1"
  service="$2"
  bold ""
  bold "==> Scenario: $name"

  # Use a dedicated data volume per scenario so scenarios don't share cache
  volume="${service}_data"
  docker volume create "$volume" >/dev/null

  exit_code=0
  docker compose -f "$TEST_DIR/docker-compose.yml" \
    run --rm \
    -v "${volume}:/tmp/data" \
    "$service" \
    || exit_code=$?

  if [ "$exit_code" -ne 0 ]; then
    red "  FAIL [$name] ibmdocs exited with code $exit_code"
    FAIL=$((FAIL+1))
    docker volume rm "$volume" >/dev/null 2>&1 || true
    return
  fi

  # Verify that at least one KB file was written
  kb_count=$(docker run --rm -v "${volume}:/data" alpine:3 \
    sh -c 'find /data/kb -name "*.md" 2>/dev/null | wc -l' 2>/dev/null || echo 0)

  docker volume rm "$volume" >/dev/null 2>&1 || true

  if [ "${kb_count}" -lt 1 ]; then
    red "  FAIL [$name] no KB Markdown files written (kb_count=$kb_count)"
    FAIL=$((FAIL+1))
    return
  fi

  green "  PASS [$name] exit=0, kb_files=$kb_count"
  PASS=$((PASS+1))
}

# ── Run both scenarios ────────────────────────────────────────────────────────
run_scenario "plain proxy (no TLS inspection)"  run_plain
run_scenario "TLS-inspect proxy (MITM CA)"       run_tls

# ── Summary ───────────────────────────────────────────────────────────────────
bold ""
bold "==> Results"
echo "  Passed: $PASS"
echo "  Failed: $FAIL"
echo ""

if [ "$FAIL" -gt 0 ]; then
  red "FAILED ($FAIL scenario(s) failed)"
  exit 1
fi
green "ALL PASSED"
