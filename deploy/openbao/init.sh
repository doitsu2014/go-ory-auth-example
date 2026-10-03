#!/bin/sh
# One-shot OpenBao bootstrap for local development (docker compose).
#
#   init.sh          initialise (first run), unseal, configure transit keys and
#                    policies, and issue the identity-service token. Idempotent;
#                    runs on every `make up` so a restarted server is unsealed.
#   init.sh rotate   rotate the KEK with a short-lived operator token.
#
# LOCAL ONLY: the unseal key and root token are kept on the `openbao-keys`
# volume so the stack can restart unattended. Production uses auto-unseal and
# never stores either.
set -eu

export BAO_ADDR="${BAO_ADDR:-http://openbao:8200}"
KEYS_FILE=/bao-keys/init.json
TOKEN_DIR=/bao-app
TOKEN_FILE="$TOKEN_DIR/identity-service.token"
ACCESSOR_FILE=/bao-keys/identity-service.accessor
APP_UID="${APP_UID:-65532}" # distroless nonroot

json_field() { # json_field <name> <file>: first string value of a key
  tr -d " \n" < "$2" | sed -n "s/.*\"$1\":\[\{0,1\}\"\([^\"]*\)\".*/\1/p" | head -n 1
}

for _ in $(seq 1 60); do
  set +e; bao status >/dev/null 2>&1; rc=$?; set -e
  [ "$rc" -ne 1 ] && break
  sleep 1
done
[ "$rc" -eq 1 ] && { echo "openbao not reachable at $BAO_ADDR" >&2; exit 1; }

if ! bao status -format=json | grep -q '"initialized": *true'; then
  echo "initialising openbao (1 key share, local only)"
  umask 077
  bao operator init -key-shares=1 -key-threshold=1 -format=json > "$KEYS_FILE"
fi

if bao status -format=json | grep -q '"sealed": *true'; then
  echo "unsealing openbao"
  bao operator unseal "$(json_field unseal_keys_b64 "$KEYS_FILE")" >/dev/null
fi

BAO_TOKEN="$(json_field root_token "$KEYS_FILE")"
export BAO_TOKEN

if [ "${1:-}" = "rotate" ]; then
  OP_TOKEN="$(bao token create -policy=identity-pii-operator -no-default-policy -ttl=5m -field=token)"
  BAO_TOKEN="$OP_TOKEN" bao write -f transit/keys/identity-pii-kek/rotate >/dev/null
  BAO_TOKEN="$OP_TOKEN" bao read -field=latest_version transit/keys/identity-pii-kek | sed 's/^/kek latest_version: /'
  bao token revoke "$OP_TOKEN" >/dev/null
  exit 0
fi

bao secrets list -format=json | grep -q '"transit/"' || bao secrets enable transit >/dev/null

bao read transit/keys/identity-pii-kek >/dev/null 2>&1 ||
  bao write transit/keys/identity-pii-kek type=aes256-gcm96 exportable=false allow_plaintext_backup=false >/dev/null
# The index key is never rotated automatically: lookups pin key_version=1.
bao read transit/keys/identity-pii-bidx >/dev/null 2>&1 ||
  bao write transit/keys/identity-pii-bidx type=hmac key_size=32 exportable=false allow_plaintext_backup=false auto_rotate_period=0 >/dev/null

bao policy write identity-service /policies/identity-service.hcl >/dev/null
bao policy write identity-pii-operator /policies/identity-pii-operator.hcl >/dev/null

# Reuse a still-valid app token; otherwise issue a new periodic one. The
# service renews it (renew-self) well within the period.
if [ -s "$TOKEN_FILE" ] && bao token lookup "$(cat "$TOKEN_FILE")" >/dev/null 2>&1; then
  echo "identity-service token still valid"
else
  umask 077
  # Revoke the previous token (if any) so a leaked copy dies with rotation.
  if [ -s "$ACCESSOR_FILE" ]; then
    bao token revoke -accessor "$(cat "$ACCESSOR_FILE")" >/dev/null 2>&1 || true
  fi
  bao token create -policy=identity-service -no-default-policy -orphan -period=24h \
    -display-name=identity-service -format=json > /tmp/app-token.json
  json_field client_token /tmp/app-token.json > "$TOKEN_FILE.tmp"
  json_field accessor /tmp/app-token.json > "$ACCESSOR_FILE"
  rm -f /tmp/app-token.json
  mv "$TOKEN_FILE.tmp" "$TOKEN_FILE"
  echo "issued identity-service token"
fi
chown "$APP_UID" "$TOKEN_FILE"
chmod 0400 "$TOKEN_FILE"
echo "openbao ready"
