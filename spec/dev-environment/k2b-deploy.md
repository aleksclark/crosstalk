# K2B Deploy Loop

[← Back to Index](../index.md) · [Dev Environment Overview](overview.md)

---

## Purpose

Automated build → deploy → test cycle for the CLI client running on the KickPi K2B board. Enables rapid iteration without manual scp/ssh steps.

## Watcher Script

```bash
# dev/scripts/watch-deploy-k2b.sh
# Watches for Go source changes, builds via task, deploys to K2B

K2B_HOST="${K2B_HOST:-192.168.1.200}"

while true; do
    # Wait for .go file changes in cli/
    inotifywait -r -e modify,create,delete cli/

    echo "Change detected, building..."
    task build:cli-arm64

    echo "Deploying to ${K2B_HOST}..."
    k2b-board/scripts/deploy.sh "${K2B_HOST}" bin/ct-client-arm64

    echo "Deployed. Service restarting."
done
```

Or use the task shortcut:

```bash
task deploy:k2b:watch   # wraps the watcher script
task deploy:k2b         # one-shot build + deploy
task deploy:k2b:test    # run audio test harness
```

## Audio Test Harness

For testing without a human speaking into the mic, use PipeWire loopback devices:

```bash
# dev/scripts/k2b-test.sh
# Plays audio into loopback source, records from loopback sink

K2B_HOST="${K2B_HOST:-192.168.1.200}"
TEST_AUDIO="dev/fixtures/test-speech.mp3"

# Setup loopback on K2B (idempotent)
ssh "${K2B_HOST}" "bash -s" < k2b-board/scripts/setup-loopback.sh

# Play test audio into the loopback source (simulates mic input)
ssh "${K2B_HOST}" "ffmpeg -i /tmp/test-speech.mp3 -f s16le -ar 48000 -ac 1 - | \
    pw-play --target loopback-source -" &
PLAY_PID=$!

# Record from loopback sink (captures what the CLI client outputs)
ssh "${K2B_HOST}" "pw-record --target loopback-sink /tmp/captured-output.ogg" &
RECORD_PID=$!

# Wait for playback to finish + buffer
wait $PLAY_PID
sleep 2
ssh "${K2B_HOST}" "kill ${RECORD_PID}"

# Retrieve recorded audio
scp "${K2B_HOST}:/tmp/captured-output.ogg" dev/output/

echo "Captured audio saved to dev/output/captured-output.ogg"
```

## Test Flow

```
1. watch-deploy-k2b.sh detects code change
2. Cross-compile CLI binary for ARM64
3. Deploy to K2B, restart systemd service
4. k2b-test.sh plays test audio into loopback
5. CLI client captures from loopback source → sends to server via WebRTC
6. Server routes audio per session template
7. CLI client receives audio → outputs to loopback sink
8. k2b-test.sh records from loopback sink
9. Compare input/output audio for golden test validation
```

## Prerequisites

- K2B board provisioned and on the network (see `k2b-board/scripts/provision-k2b.sh`)
- SSH key-based auth to K2B (no password prompts)
- PipeWire running on K2B with loopback module available
- `inotifywait` installed on dev host (from `inotify-tools`)
- Test audio file at `dev/fixtures/test-speech.mp3`
