#!/usr/bin/env bash
# redact-evidence.sh — Scan an evidence directory for credential canaries.
# Fails closed if JWT-like strings, bearer headers, or known secret env values appear.
set -euo pipefail

DIR="${1:?usage: redact-evidence.sh <evidence-dir>}"
CANARIES_FILE="${2:-}"

echo "==> Secret canary scan on $DIR"

# Patterns that must never appear in uploaded artifacts.
PATTERNS=(
  'Authorization:[[:space:]]*Bearer'
  'eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}'
  'refresh_token'
  'access_token'
  'CROSSTALK_ADMIN_PASSWORD='
  'CROSSTALK_TRANSLATOR_PASSWORD='
  'CROSSTALK_ABC_TOKEN='
)

found=0
while IFS= read -r -d '' f; do
  # Skip binary-ish media; still scan small text sidecars.
  case "$f" in
    *.wav|*.png|*.jpg|*.jpeg|*.mp4|*.webm|*.apk) continue ;;
  esac
  for pat in "${PATTERNS[@]}"; do
    if grep -EIq "$pat" "$f" 2>/dev/null; then
      echo "CANARY HIT: $pat in $f" >&2
      found=1
    fi
  done
done < <(find "$DIR" -type f -print0)

if [[ -n "$CANARIES_FILE" && -f "$CANARIES_FILE" ]]; then
  while IFS= read -r c; do
    [[ -z "$c" || "$c" == \#* ]] && continue
    if grep -RIqF -- "$c" "$DIR" 2>/dev/null; then
      echo "CANARY HIT: explicit secret value present in evidence tree" >&2
      found=1
    fi
  done <"$CANARIES_FILE"
fi

if [[ "$found" -ne 0 ]]; then
  echo "FAIL: evidence contains secrets; refusing to archive/upload" >&2
  exit 2
fi
echo "OK: no credential canaries detected"
