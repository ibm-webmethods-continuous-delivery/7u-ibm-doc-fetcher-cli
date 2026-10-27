FROM mitmproxy/mitmproxy:latest

# Pre-create dirs — mitmproxy writes its CA here on first start.
# /tmp is needed by some mitmproxy internals.
RUN mkdir -p /home/mitmproxy/.mitmproxy /logs \
 && chown mitmproxy:mitmproxy /home/mitmproxy/.mitmproxy /logs

USER mitmproxy
EXPOSE 3128

# MITM_MODE is set by docker-compose:
#   plain — CONNECT passthrough (no TLS inspection)
#           mitmproxy runs with --ignore-hosts .* so it tunnels all CONNECT
#           requests without intercepting the TLS handshake.
#           ibmdocs does NOT need --tls-ca-bundle for this mode.
#
#   tls   — full MITM (TLS inspection)
#           mitmproxy intercepts and re-signs all HTTPS using its generated CA.
#           ibmdocs MUST supply --tls-ca-bundle /mitm/mitmproxy-ca-cert.pem.
ENV MITM_MODE=tls

ENTRYPOINT ["sh", "-c", "\
  if [ \"$MITM_MODE\" = 'plain' ]; then \
    exec mitmdump \
      --listen-host 0.0.0.0 --listen-port 3128 \
      --ignore-hosts '.*' \
      --set flow_detail=0; \
  else \
    exec mitmdump \
      --listen-host 0.0.0.0 --listen-port 3128 \
      --ssl-insecure \
      --set flow_detail=0; \
  fi"]
