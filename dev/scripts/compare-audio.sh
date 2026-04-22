#!/usr/bin/env bash
# compare-audio.sh — Compare two audio files via normalised cross-correlation.
#
# Usage:  compare-audio.sh <file_a> <file_b> [threshold]
#
# Normalises both inputs to 16 kHz mono s16le WAV, then computes the
# Pearson correlation coefficient using only Python 3 + struct (no numpy).
#
# Outputs the correlation score (0.0–1.0) on stdout.
# Exits 0 if score > threshold (default 0.9), exit 1 otherwise.
#
# Dependencies: ffmpeg, python3  (sox NOT required)
set -euo pipefail

die() { echo "ERROR: $*" >&2; exit 2; }

# ── args ────────────────────────────────────────────────────────────────
[[ $# -ge 2 ]] || die "usage: $0 <file_a> <file_b> [threshold]"
FILE_A="$1"
FILE_B="$2"
THRESHOLD="${3:-0.9}"

[[ -f "$FILE_A" ]] || die "file not found: $FILE_A"
[[ -f "$FILE_B" ]] || die "file not found: $FILE_B"

command -v ffmpeg  >/dev/null 2>&1 || die "ffmpeg not found"
command -v python3 >/dev/null 2>&1 || die "python3 not found"

# ── temp dir (cleaned up on exit) ──────────────────────────────────────
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# ── normalise to 16 kHz mono s16le raw PCM ─────────────────────────────
normalise() {
  local src="$1" dst="$2"
  ffmpeg -y -hide_banner -loglevel error \
    -i "$src" \
    -ar 16000 -ac 1 -f s16le -acodec pcm_s16le \
    "$dst"
}

RAW_A="$TMPDIR/a.raw"
RAW_B="$TMPDIR/b.raw"

normalise "$FILE_A" "$RAW_A"
normalise "$FILE_B" "$RAW_B"

# ── compute correlation via Python 3 (stdlib only) ─────────────────────
SCORE=$(python3 - "$RAW_A" "$RAW_B" <<'PYEOF'
import struct, sys, math, os

def read_pcm_s16le(path):
    """Read raw s16le PCM file into a list of float samples in [-1, 1]."""
    size = os.path.getsize(path)
    n = size // 2  # 2 bytes per sample
    with open(path, "rb") as f:
        data = f.read(n * 2)
    samples = struct.unpack(f"<{n}h", data)
    return [s / 32768.0 for s in samples]

def pearson(a, b):
    """Pearson correlation coefficient, clamped to [0, 1]."""
    n = min(len(a), len(b))
    if n == 0:
        return 0.0
    # Trim to equal length
    a = a[:n]
    b = b[:n]

    mean_a = sum(a) / n
    mean_b = sum(b) / n

    num = 0.0
    den_a = 0.0
    den_b = 0.0
    for i in range(n):
        da = a[i] - mean_a
        db = b[i] - mean_b
        num += da * db
        den_a += da * da
        den_b += db * db

    denom = math.sqrt(den_a * den_b)
    if denom < 1e-12:
        # Both signals are essentially silent / DC — check if both are near-zero
        if den_a < 1e-12 and den_b < 1e-12:
            return 1.0  # both silent ≡ identical
        return 0.0

    r = num / denom
    # Map from [-1,1] to [0,1]:  identical=1.0, uncorrelated=0.5, inverted=0.0
    # BUT for audio comparison we care about absolute correlation (phase-flip is OK).
    return abs(r)

a = read_pcm_s16le(sys.argv[1])
b = read_pcm_s16le(sys.argv[2])
score = pearson(a, b)
print(f"{score:.6f}")
PYEOF
)

echo "$SCORE"

# ── threshold check ────────────────────────────────────────────────────
if python3 -c "import sys; sys.exit(0 if float('$SCORE') > float('$THRESHOLD') else 1)"; then
  exit 0
else
  exit 1
fi
