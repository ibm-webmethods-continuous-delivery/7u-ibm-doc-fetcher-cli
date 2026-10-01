# Stage 1: build static binary
FROM alpine:3 AS builder

RUN apk add --no-cache go musl-dev git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/     ./cmd/
COPY internal/ ./internal/

ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG GIT_COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/cmd.version=${VERSION} \
      -X github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/cmd.buildDate=${BUILD_DATE} \
      -X github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/cmd.gitCommit=${GIT_COMMIT}" \
    -o ibmdocs \
    ./cmd/ibmdocs

# Verify static linkage
RUN ldd ibmdocs 2>&1 | grep -qE "(statically linked|Not a valid dynamic program|not a dynamic executable)" \
    || (echo "ERROR: binary is not statically linked" && exit 1)

# Stage 2: minimal runtime image
FROM scratch

# CA certificates required for HTTPS to 1.www.s81c.com
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /build/ibmdocs /ibmdocs

ENTRYPOINT ["/ibmdocs"]
