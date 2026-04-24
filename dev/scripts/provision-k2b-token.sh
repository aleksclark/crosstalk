#!/usr/bin/env bash
# provision-k2b-token.sh — Upsert a Client on the dev server, create an API
# token scoped to it, and write a ct-client config to the K2B board.
#
# Usage: ./dev/scripts/provision-k2b-token.sh <board-ip>
#
# Requires: the dev server running (task dev:server), jq, curl, ssh.
# Idempotent: reuses existing "K2B Booth" client, revokes old token.
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SERVER_URL="http://localhost:8080"
CLIENT_NAME="K2B Booth"
TOKEN_NAME="k2b"
SSH="ssh -o ConnectTimeout=5 -i ~/.ssh/id_rsa root@${BOARD_IP}"
K2B_CFG="/etc/crosstalk/client.json"

for cmd in jq curl ssh; do
    command -v "$cmd" >/dev/null 2>&1 || { echo "ERROR: $cmd not found"; exit 1; }
done

# ── Obtain an API token for provisioning ──────────────────────────────────────
echo "Obtaining API token..."

# Try 1: extract seed token from docker logs (available on first-ever boot).
SEED_TOKEN="$(docker compose -f dev/docker-compose.yml logs server 2>&1 \
    | grep '"seed API token created"' \
    | tail -1 \
    | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"

if [[ -z "$SEED_TOKEN" ]]; then
    # Try 2: extract seed password from docker logs and login to get a token.
    SEED_PASS="$(docker compose -f dev/docker-compose.yml logs server 2>&1 \
        | grep '"admin user seeded"' \
        | tail -1 \
        | sed -n 's/.*"password":"\([^"]*\)".*/\1/p')"
    if [[ -n "$SEED_PASS" ]]; then
        echo "  Seed token not in logs, logging in with seed password..."
        LOGIN_RESP=$(curl -sf -X POST "${SERVER_URL}/api/auth/login" \
            -H "Content-Type: application/json" \
            -d "{\"username\": \"admin\", \"password\": \"${SEED_PASS}\"}" 2>/dev/null || true)
        SEED_TOKEN=$(echo "$LOGIN_RESP" | jq -r '.token // empty' 2>/dev/null || true)
    fi
fi

if [[ -z "$SEED_TOKEN" ]]; then
    echo "ERROR: Could not obtain API token from dev server."
    echo "       Try: task dev:reset && task dev:server  (wipes DB and recreates seed)"
    exit 1
fi
echo "  API token: ${SEED_TOKEN:0:12}..."

# ── Verify dev server is reachable ───────────────────────────────────────────
if ! curl -sf "${SERVER_URL}/api/templates" -H "Authorization: Bearer ${SEED_TOKEN}" >/dev/null 2>&1; then
    echo "ERROR: Dev server at ${SERVER_URL} not responding."
    exit 1
fi
echo "  Dev server OK"

# ── Upsert client ────────────────────────────────────────────────────────────
echo "Looking up client '${CLIENT_NAME}'..."
CLIENT_ID=$(curl -sf "${SERVER_URL}/api/clients" \
    -H "Authorization: Bearer ${SEED_TOKEN}" \
    | jq -r ".[] | select(.name == \"${CLIENT_NAME}\") | .id" 2>/dev/null | head -1 || true)

if [[ -n "$CLIENT_ID" && "$CLIENT_ID" != "null" ]]; then
    echo "  Found existing client: ${CLIENT_ID:0:12}..."
else
    echo "  Creating client '${CLIENT_NAME}'..."
    CLIENT_RESP=$(curl -sf -X POST "${SERVER_URL}/api/clients" \
        -H "Authorization: Bearer ${SEED_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "{\"name\": \"${CLIENT_NAME}\"}")
    CLIENT_ID=$(echo "$CLIENT_RESP" | jq -r '.id')
    if [[ -z "$CLIENT_ID" || "$CLIENT_ID" == "null" ]]; then
        echo "ERROR: Client creation failed: $CLIENT_RESP"
        exit 1
    fi
    echo "  Created client: ${CLIENT_ID:0:12}..."
fi

# ── Revoke any existing k2b token for this client ────────────────────────────
EXISTING=$(curl -sf "${SERVER_URL}/api/tokens" \
    -H "Authorization: Bearer ${SEED_TOKEN}" \
    | jq -r ".[] | select(.name == \"${TOKEN_NAME}\" and .client_id == \"${CLIENT_ID}\") | .id" 2>/dev/null || true)

for tok_id in $EXISTING; do
    echo "  Revoking old ${TOKEN_NAME} token ${tok_id:0:12}..."
    curl -sf -X DELETE "${SERVER_URL}/api/tokens/${tok_id}" \
        -H "Authorization: Bearer ${SEED_TOKEN}" >/dev/null
done

# ── Create new token scoped to the client ─────────────────────────────────────
echo "Creating API token '${TOKEN_NAME}' for client ${CLIENT_ID:0:12}..."
TOKEN_RESP=$(curl -sf -X POST "${SERVER_URL}/api/tokens" \
    -H "Authorization: Bearer ${SEED_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${TOKEN_NAME}\", \"client_id\": \"${CLIENT_ID}\"}")

K2B_TOKEN=$(echo "$TOKEN_RESP" | jq -r '.token')
if [[ -z "$K2B_TOKEN" || "$K2B_TOKEN" == "null" ]]; then
    echo "ERROR: Token creation failed: $TOKEN_RESP"
    exit 1
fi
echo "  Token: ${K2B_TOKEN:0:12}..."

# ── Detect host IP reachable from K2B ────────────────────────────────────────
HOST_IP="${HOST_IP:-$(ip route get "$BOARD_IP" 2>/dev/null | sed -n 's/.*src \([0-9.]*\).*/\1/p' | head -1 || true)}"
if [[ -z "$HOST_IP" ]]; then
    echo "ERROR: Could not detect host IP. Set HOST_IP env var."
    exit 1
fi
echo "  Host IP: ${HOST_IP}"

# ── Write config to K2B ──────────────────────────────────────────────────────
echo "Writing client config to ${BOARD_IP}:${K2B_CFG}..."
$SSH "mkdir -p $(dirname "$K2B_CFG")"
$SSH "cat > ${K2B_CFG}" <<EOCFG
{
  "server_url": "http://${HOST_IP}:8080",
  "token": "${K2B_TOKEN}",
  "source_name": "alsa_input.platform-snd_aloop.0.analog-stereo",
  "sink_name": "alsa_output.platform-snd_aloop.0.analog-stereo",
  "log_level": "debug"
}
EOCFG

# ── Configure systemd to use the config ──────────────────────────────────────
echo "Configuring systemd drop-in for app.service..."
$SSH "mkdir -p /etc/systemd/system/app.service.d"
$SSH "cat > /etc/systemd/system/app.service.d/crosstalk.conf" <<EOUNIT
[Service]
User=streamlate
Environment=CROSSTALK_CONFIG=${K2B_CFG}
Environment=XDG_RUNTIME_DIR=/run/user/999
EOUNIT
$SSH "systemctl daemon-reload"

echo "Done. Client '${CLIENT_NAME}' (${CLIENT_ID:0:12}...) provisioned with token."
