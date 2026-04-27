#!/bin/sh
# Generate config from env vars if not already present on the volume.
CONFIG="/data/config.json"
if [ ! -f "$CONFIG" ]; then
  # Discover public IP: CT_PUBLIC_IP (set via fly secrets), or empty
  PUBLIC_IP="${CT_PUBLIC_IP:-}"

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
