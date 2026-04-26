#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Fix ALSA mixer for audiocodec ---"
$SSH '
amixer -c 1 cset numid=4 on   # Left Output Mixer DACL Switch
amixer -c 1 cset numid=6 on   # Right Output Mixer DACL Switch  
amixer -c 1 cset numid=5 on   # Left Output Mixer DACR Switch
amixer -c 1 cset numid=7 on   # Right Output Mixer DACR Switch
amixer -c 1 cset numid=2 31   # LINEOUT volume max
amixer -c 1 cset numid=3 on   # LINEOUT Switch
echo ""
echo "--- Verify ---"
amixer -c 1 contents | grep -A2 "Output Mixer\|LINEOUT"
'

echo ""
echo "--- Test: play a tone to audiocodec ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 3 speaker-test -D alsa_output.platform-5096000.codec.pro-output-0 -t sine -f 440 -l 1 2>&1 | head -5 || echo "speaker-test not available, trying pw-play..."'
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 3 pw-play --target=alsa_output.platform-5096000.codec.pro-output-0 /usr/share/sounds/alsa/Front_Left.wav 2>&1 || echo "pw-play test done"'
