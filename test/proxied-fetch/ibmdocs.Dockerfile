# Builds the ibmdocs:test image from a pre-built Linux binary.
# Used by test/proxied-fetch/run.sh — not for production releases.
#
# Build context must be the repo root so BINARY (relative to repo root) is reachable.
#
#   docker build \
#     --build-arg BINARY=target/bin/ibmdocs-linux-arm64 \
#     -t ibmdocs:test \
#     -f test/proxied-fetch/ibmdocs.Dockerfile \
#     .

FROM scratch

ARG BINARY=target/bin/ibmdocs-linux-arm64

# System CA pool from Alpine (trusts standard public CAs)
COPY --from=alpine:3 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Trust the test harness target CA so scenario 1 (plain proxy, no TLS inspection)
# succeeds without requiring --tls-ca-bundle for the target's own cert.
# The proxy CA for scenario 2 is still passed via --tls-ca-bundle at runtime.
COPY --from=proxied-fetch-target /etc/nginx/ssl/ca.crt /etc/ssl/certs/target-ca.crt

COPY ${BINARY} /ibmdocs

ENTRYPOINT ["/ibmdocs"]
