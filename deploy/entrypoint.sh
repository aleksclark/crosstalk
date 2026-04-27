#!/bin/sh
# Generate config from env vars if not already present on the volume.
CONFIG="/data/config.json"
if [ ! -f "$CONFIG" ]; then
  # Discover public IP: try FLY_PUBLIC_IP env, then curl Fly metadata, then empty
  PUBLIC_IP="${FLY_PUBLIC_IP:-}"
  if [ -z "$PUBLIC_IP" ] && command -v curl >/dev/null 2>&1; then
    PUBLIC_IP=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || true)
  fi

  cat > "$CONFIG" <<EOF
{
  "listen": ":8080",
  "db_path": "/data/crosstalk.db",
  "recording_path": "/data/recordings",
  "log_level": "${CT_LOG_LEVEL:-info}",
  "webrtc": {
    "stun_servers": ["stun:stun.l.google.com:19302"],
    "udp_mux_port": 5000,
    "public_ip": "${PUBLIC_IP}"
  },
  "auth": {
    "session_secret": "${CT_SESSION_SECRET:-change-me-in-production}"
  },
  "web": {
    "dev_mode": false
  }
}
EOF
  echo "Generated config at $CONFIG (public_ip=${PUBLIC_IP})"
fi

exec ct-server --config "$CONFIG"
