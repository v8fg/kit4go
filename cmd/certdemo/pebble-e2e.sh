#!/usr/bin/env bash
# Offline end-to-end issuance against a local Pebble ACME server (no Let's
# Encrypt, no public DNS, no port 80/root). Pebble validates http-01 on the
# httpPort configured below (5002), so the challenge server runs unprivileged.
#
# What it does:
#   1. installs Pebble + minica (a tiny CA generator);
#   2. generates a local root CA + a server cert for 127.0.0.1 (Pebble's ACME
#      API is TLS, self-signed by our CA — the driver trusts the root);
#   3. writes a Pebble config with httpPort=5002 and starts Pebble;
#   4. maps the test domain to 127.0.0.1 in /etc/hosts (needs sudo);
#   5. runs cmd/certdemo-pebble, which issues certdemo.test and writes
#      <domain>.crt/<domain>.key;
#   6. tears everything down on exit.
#
# Verified by construction against Pebble/minica's current conventions; the
# sandbox this was authored in cannot run Docker/pebble, so run it on a machine
# with network + Docker/Go toolchain to complete issuance.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
DOMAIN="certdemo.test"
HTTP_PORT=5002
ACME_ADDR="127.0.0.1:14000"

echo ">> work dir: $WORK"
cd "$WORK"

echo ">> installing pebble + minica"
GOBIN="$WORK/bin" go install github.com/letsencrypt/pebble/cmd/pebble@latest
GOBIN="$WORK/bin" go install github.com/jsha/minica@latest

echo ">> generating local CA + 127.0.0.1 server cert"
"$WORK/bin/minica" -ip-addresses 127.0.0.1
# -> minica.pem (root), minica-key.pem, 127.0.0.1/cert.pem, 127.0.0.1/key.pem

cat > pebble-config.json <<JSON
{
  "pebble": {
    "listenAddress": "$ACME_ADDR",
    "certificate": "$WORK/127.0.0.1/cert.pem",
    "privateKey": "$WORK/127.0.0.1/key.pem"
  },
  "httpPort": $HTTP_PORT,
  "tlsPort": 5003,
  "ocspResponderURL": "",
  "externalAccountBindingRequired": false
}
JSON

cleanup() {
  echo ">> cleanup"
  [[ -n "${PEBBLE_PID:-}" ]] && kill "$PEBBLE_PID" 2>/dev/null || true
  sudo grep -q " $DOMAIN" /etc/hosts && sudo sed -i '' "/ $DOMAIN/d" /etc/hosts 2>/dev/null \
    || sudo sed -i "/ $DOMAIN/d" /etc/hosts 2>/dev/null || true
}
trap cleanup EXIT

echo ">> mapping $DOMAIN -> 127.0.0.1 in /etc/hosts (sudo)"
if ! grep -q " $DOMAIN\$" /etc/hosts; then
  echo "127.0.0.1 $DOMAIN" | sudo tee -a /etc/hosts >/dev/null
fi

echo ">> starting pebble"
PEBBLE_VA_NOSLA=1 "$WORK/bin/pebble" -config "$WORK/pebble-config.json" &
PEBBLE_PID=$!
sleep 2

echo ">> running driver"
cd "$ROOT"
go run ./cmd/certdemo-pebble \
  -domain "$DOMAIN" \
  -dir "$WORK/certs-out" \
  -directory "https://$ACME_ADDR/dir" \
  -ca "$WORK/minica.pem" \
  -addr ":$HTTP_PORT"

echo ">> issued cert:"
openssl x509 -in "$WORK/certs-out/$DOMAIN.crt" -noout -subject -dates 2>/dev/null || true
