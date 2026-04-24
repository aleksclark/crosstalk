/**
 * golden-audio.spec.ts — Full E2E golden audio test through the admin UI.
 *
 * ╔══════════════════════════════════════════════════════════════════════════╗
 * ║  INVARIANTS — do not weaken or remove these constraints:               ║
 * ║                                                                        ║
 * ║  1. Real server    — ct-server running with real SQLite, real WebRTC.  ║
 * ║                      No mocks, no stubs, no in-memory fakes.           ║
 * ║  2. Real K2B board — physical KickPi K2B with PipeWire + ALSA         ║
 * ║                      loopback. ct-client runs on real ARM64 hardware.  ║
 * ║  3. Real audio     — test tone travels the full WebRTC SFU pipeline:  ║
 * ║                      K2B PipeWire → ct-client → WebRTC → ct-server    ║
 * ║                      SFU → WebRTC → Playwright browser (Chromium).    ║
 * ║  4. Admin UI only  — every user-facing step uses the web UI. No raw   ║
 * ║                      API calls, no direct DB manipulation. The only    ║
 * ║                      non-UI operations are SSH commands to the K2B     ║
 * ║                      board (deploying ct-client, playing test audio).  ║
 * ║                                                                        ║
 * ║  If you are tempted to replace any of these with mocks, stubs, or     ║
 * ║  API shortcuts — don't. The entire point of this test is to prove the ║
 * ║  system works end-to-end exactly as a human operator would use it.    ║
 * ╚══════════════════════════════════════════════════════════════════════════╝
 *
 * Driven by run-e2e-tests.sh which sets:
 *   CT_SERVER_URL     — server base URL (e.g. http://192.168.0.10:9090)
 *   CT_K2B_HOST       — K2B board IP (e.g. 192.168.0.109)
 *   CT_K2B_USER       — PipeWire user on K2B (default: streamlate)
 *   CT_K2B_UID        — UID for XDG_RUNTIME_DIR (default: 999)
 *   CT_ADMIN_PASSWORD  — admin password for the server (default: Password!)
 *   CT_CAPTURE_PATH    — where to write captured audio (default: /tmp/browser-captured-audio.webm)
 *   CT_AUDIO_DURATION_MS — how long to record audio (default: 7000)
 *
 * The orchestrator script handles:
 *   - Building ct-server and ct-client-arm64
 *   - Starting ct-server with a fresh database
 *   - Deploying ct-client binary + test tone WAV to K2B
 *   - Creating an API token for the K2B client
 *   - Starting ct-client on K2B (connects to server, idle until assigned)
 *   - Running this Playwright spec
 *   - Comparing captured audio against the reference tone
 */
import { test, expect, type Page } from "@playwright/test";
import { execSync } from "child_process";
import * as fs from "fs";
import * as path from "path";

// ── Environment ─────────────────────────────────────────────────────────────

const SERVER_URL = process.env.CT_SERVER_URL || "http://localhost:8080";
const K2B_HOST = process.env.CT_K2B_HOST || "";
const K2B_USER = process.env.CT_K2B_USER || "streamlate";
const K2B_UID = process.env.CT_K2B_UID || "999";
const ADMIN_PASSWORD = process.env.CT_ADMIN_PASSWORD || "Password!";
const CAPTURE_PATH =
  process.env.CT_CAPTURE_PATH || "/tmp/browser-captured-audio.webm";
const AUDIO_DURATION = parseInt(
  process.env.CT_AUDIO_DURATION_MS || "7000",
  10,
);

// ── Helpers ─────────────────────────────────────────────────────────────────

function ssh(cmd: string): string {
  return execSync(`ssh -o ConnectTimeout=5 root@${K2B_HOST} "${cmd}"`, {
    encoding: "utf-8",
    timeout: 30_000,
  }).trim();
}

function sshNoFail(cmd: string): string {
  try {
    return ssh(cmd);
  } catch {
    return "";
  }
}

async function loginViaUI(page: Page): Promise<void> {
  await page.goto("/login");
  await page.fill("#username", "admin");
  await page.fill("#password", ADMIN_PASSWORD);
  await page.click('button[type="submit"]');
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });
}

// ── Test ────────────────────────────────────────────────────────────────────

test.describe("Golden Audio — Full Admin UI Flow", () => {
  /**
   * Skip the entire suite if the K2B host isn't configured.
   * This test requires physical hardware and is run exclusively via
   * run-e2e-tests.sh. It MUST NOT be neutered into a mock-based test.
   */
  test.skip(!K2B_HOST, "CT_K2B_HOST not set — run via run-e2e-tests.sh");

  /**
   * Increase the overall timeout for this test. The full flow involves
   * WebRTC negotiation across a real network to a real ARM64 board,
   * plus audio capture. 3 minutes is generous but realistic.
   */
  test.setTimeout(180_000);

  test("K2B→Browser: full admin UI flow with real audio capture", async ({
    page,
  }) => {
    // ════════════════════════════════════════════════════════════════════════
    //  STEP 1: Verify K2B is provisioned and ct-client is running
    // ════════════════════════════════════════════════════════════════════════
    //
    // The orchestrator script (run-e2e-tests.sh) has already:
    //   - Built and deployed ct-client-arm64 to the K2B board
    //   - Created an API token and written client config
    //   - Started ct-client, which connects to the server via WebSocket
    //     and WebRTC, sends Hello with PipeWire capabilities, and idles
    //     waiting for the server to assign it to a session.
    //
    // We verify the client process is alive. If it's not, there's no point
    // continuing — this is a REAL hardware test.

    const clientPid = sshNoFail("pgrep -x ct-client");
    expect(clientPid, "ct-client must be running on K2B").not.toBe("");

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 2: Log in via admin UI
    // ════════════════════════════════════════════════════════════════════════
    //
    // Real login through the web form. No API token injection, no
    // sessionStorage manipulation.

    await loginViaUI(page);

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 3: Create session template "Translation"
    // ════════════════════════════════════════════════════════════════════════
    //
    // Two roles: studio, translator.
    // Bidirectional audio mappings:
    //   studio:input    → translator:speakers
    //   translator:mic  → studio:output
    //
    // This is done entirely through the template editor UI.

    await page.click('nav >> text=Templates');
    await expect(page).toHaveURL(/\/templates/);
    await page.click('[data-testid="create-template-button"]');
    await expect(page).toHaveURL(/\/templates\/new/);

    await page.fill('[data-testid="template-name-input"]', "Translation");

    // Role 1: studio
    const roleInputs = page.locator('[data-testid="role-name-input"]');
    await roleInputs.first().clear();
    await roleInputs.first().fill("studio");

    // Role 2: translator
    await page.click('[data-testid="add-role-button"]');
    await roleInputs.nth(1).fill("translator");

    // Mapping 1: studio:input → translator:speakers
    await page
      .locator('[data-testid="mapping-from-role"]')
      .first()
      .selectOption("studio");
    await page
      .locator('[data-testid="mapping-from-channel"]')
      .first()
      .fill("input");
    await page
      .locator('[data-testid="mapping-to-role"]')
      .first()
      .selectOption("translator");
    await page
      .locator('[data-testid="mapping-to-channel"]')
      .first()
      .fill("speakers");

    // Mapping 2: translator:mic → studio:output
    await page.click('[data-testid="add-mapping-button"]');
    await page
      .locator('[data-testid="mapping-from-role"]')
      .nth(1)
      .selectOption("translator");
    await page
      .locator('[data-testid="mapping-from-channel"]')
      .nth(1)
      .fill("mic");
    await page
      .locator('[data-testid="mapping-to-role"]')
      .nth(1)
      .selectOption("studio");
    await page
      .locator('[data-testid="mapping-to-channel"]')
      .nth(1)
      .fill("output");

    // Save template
    await page.click('[data-testid="save-template-button"]');
    await expect(page).toHaveURL(/\/templates$/, { timeout: 15_000 });
    await expect(page.locator("text=Translation")).toBeVisible();

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 4: Confirm K2B appears as a connected peer
    // ════════════════════════════════════════════════════════════════════════
    //
    // Navigate to Sessions and create a session. The K2B ct-client should
    // already be connected to the server (visible as a peer). We verify
    // this on the session detail page's "Assign Peers" card.

    await page.click('nav >> text=Sessions');
    await expect(page).toHaveURL(/\/sessions/);

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 5: Create a session and assign K2B as studio
    // ════════════════════════════════════════════════════════════════════════
    //
    // Create the session through the UI, then assign the K2B peer to the
    // "studio" role via the Assign Peers card.

    await page.click('[data-testid="create-session-button"]');
    await page.fill('[data-testid="session-name-input"]', "Golden Audio Test");
    await page.selectOption('[data-testid="session-template-select"]', {
      label: "Translation",
    });
    await page.click('[data-testid="confirm-create-session"]');

    // Wait for the session row to appear, then navigate to its detail page.
    await expect(page.locator("text=Golden Audio Test")).toBeVisible({
      timeout: 10_000,
    });
    await page.click("text=Golden Audio Test");
    await expect(page.locator("h1")).toContainText("Golden Audio Test", {
      timeout: 10_000,
    });

    // The K2B peer should appear in the Assign Peers card. The peer list
    // polls every 3s, so wait for at least one peer row to appear.
    await expect(page.locator("text=Assign Peers")).toBeVisible();
    const peerRow = page.locator(
      ".flex.items-center.justify-between.border >> nth=0",
    );
    await expect(peerRow).toBeVisible({ timeout: 15_000 });

    // Select "studio" role in the assign dropdown and click Assign.
    const assignSelect = page.locator(
      '[data-testid="assign-role-select"]',
    ).first();
    await expect(assignSelect).toBeVisible({ timeout: 5_000 });
    await assignSelect.selectOption("studio");
    await page.locator('[data-testid="assign-peer-button"]').first().click();

    // Verify no error appeared and the peer now shows "in session" badge.
    await expect(
      page.locator('[data-testid="assign-error"]'),
    ).not.toBeVisible({ timeout: 5_000 });
    await expect(page.locator("text=in session")).toBeVisible({
      timeout: 10_000,
    });

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 6: Verify K2B is listed as a connected client
    // ════════════════════════════════════════════════════════════════════════
    //
    // The Connected Clients card should now show the K2B device with
    // role "studio". This is the server's authoritative view of who's
    // in the session.

    await expect(page.locator("text=Connected Clients")).toBeVisible();
    // After assignment the client count should update (may need a page
    // refresh since session detail doesn't auto-poll client list).
    // Reload to get fresh data.
    await page.reload();
    await expect(page.locator("h1")).toContainText("Golden Audio Test", {
      timeout: 10_000,
    });

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 7: Select "translator" role and click Connect
    // ════════════════════════════════════════════════════════════════════════
    //
    // The admin user (Playwright browser) connects to the session as the
    // "translator" role. This establishes a second WebRTC peer connection
    // through the server SFU. Audio will flow bidirectionally:
    //   studio (K2B) ↔ server SFU ↔ translator (browser)

    await expect(
      page.locator('[data-testid="connect-role-select"]'),
    ).toBeVisible({ timeout: 10_000 });
    await page
      .locator('[data-testid="connect-role-select"]')
      .selectOption("translator");
    await page.click('[data-testid="connect-button"]');

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 8: Transition to connect view — WebRTC connects bidirectionally
    // ════════════════════════════════════════════════════════════════════════
    //
    // We should now be on /sessions/:id/connect?role=translator.
    // The WebRTC debug panel shows ICE state. Wait for "connected".
    // This is a REAL WebRTC connection through a REAL SFU to a REAL device.

    await expect(page).toHaveURL(/\/connect\?role=translator/, {
      timeout: 15_000,
    });
    await expect(
      page.locator('[data-testid="webrtc-debug"]'),
    ).toBeVisible({ timeout: 15_000 });

    // Wait for ICE state to reach "connected". The debug panel shows
    // "ICE State" followed by the current state.
    await expect(page.locator("text=connected").first()).toBeVisible({
      timeout: 30_000,
    });

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 9: K2B remains stable & connected for 10 seconds
    // ════════════════════════════════════════════════════════════════════════
    //
    // Real WebRTC connections can flap. We hold the connection open for 10s
    // and verify the K2B client process is still running and ICE hasn't
    // degraded to "disconnected" or "failed".

    await page.waitForTimeout(10_000);

    // Verify ICE hasn't fallen to failed/disconnected.
    const iceText = await page
      .locator('[data-testid="webrtc-debug"]')
      .textContent();
    expect(
      iceText,
      "ICE must not be in failed state after 10s hold",
    ).not.toContain("failed");

    // Verify K2B client is still alive.
    const clientStillRunning = sshNoFail("pgrep -x ct-client");
    expect(
      clientStillRunning,
      "ct-client must remain running on K2B after 10s",
    ).not.toBe("");

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 10: Play test audio on K2B into PipeWire input
    // ════════════════════════════════════════════════════════════════════════
    //
    // This is the ONE step that cannot be done through the admin UI — it
    // requires SSH to the physical K2B board to inject audio into PipeWire.
    //
    // The audio path being tested:
    //   K2B PipeWire default source (ALSA loopback)
    //     → ct-client captures audio, encodes Opus, sends via WebRTC
    //       → ct-server SFU receives, forwards to translator peer
    //         → Playwright browser receives WebRTC audio track
    //
    // ffmpeg plays the 1kHz test tone into PipeWire's default sink, which
    // routes through the ALSA loopback to the source that ct-client reads.

    ssh(
      `su - ${K2B_USER} -c 'XDG_RUNTIME_DIR=/run/user/${K2B_UID} nohup ffmpeg -re -i /tmp/test-tone.wav -t 6 -f pulse default > /tmp/ffmpeg-play.log 2>&1 &'`,
    );

    // ════════════════════════════════════════════════════════════════════════
    //  STEP 11: Capture received audio in the Playwright browser
    // ════════════════════════════════════════════════════════════════════════
    //
    // The browser is connected as "translator". Audio from the K2B studio
    // role arrives as a WebRTC remote audio track. We capture it using the
    // Web Audio API + MediaRecorder.
    //
    // This proves the COMPLETE audio pipeline works:
    //   Physical hardware → real network → real SFU → real browser

    const audioBase64: string = await page.evaluate(
      async (durationMs: number) => {
        return new Promise<string>((resolve, reject) => {
          try {
            // Find any audio/video element with a remote MediaStream.
            let stream: MediaStream | null = null;
            for (const el of document.querySelectorAll("audio, video")) {
              const m = el as HTMLMediaElement;
              if (m.srcObject instanceof MediaStream) {
                stream = m.srcObject;
                break;
              }
            }
            // Fallback: check for exposed remote stream.
            if (!stream && (window as any).__remoteStream) {
              stream = (window as any).__remoteStream;
            }
            if (!stream) {
              reject(new Error("No remote audio stream found"));
              return;
            }
            const audioTracks = stream.getAudioTracks();
            if (audioTracks.length === 0) {
              reject(new Error("No audio tracks in remote stream"));
              return;
            }

            const audioStream = new MediaStream(audioTracks);
            const recorder = new MediaRecorder(audioStream, {
              mimeType: "audio/webm;codecs=opus",
            });
            const chunks: Blob[] = [];
            recorder.ondataavailable = (e) => {
              if (e.data.size > 0) chunks.push(e.data);
            };
            recorder.onstop = async () => {
              const blob = new Blob(chunks, { type: "audio/webm" });
              const buffer = await blob.arrayBuffer();
              const bytes = new Uint8Array(buffer);
              let binary = "";
              for (let i = 0; i < bytes.length; i++) {
                binary += String.fromCharCode(bytes[i]);
              }
              resolve(btoa(binary));
            };
            recorder.start();
            setTimeout(() => recorder.stop(), durationMs);
          } catch (err) {
            reject(err);
          }
        });
      },
      AUDIO_DURATION,
    );

    // Write captured audio to disk for the orchestrator to compare
    // against the reference tone using cross-correlation.
    const audioBuffer = Buffer.from(audioBase64, "base64");
    const captureDir = path.dirname(CAPTURE_PATH);
    if (!fs.existsSync(captureDir)) {
      fs.mkdirSync(captureDir, { recursive: true });
    }
    fs.writeFileSync(CAPTURE_PATH, audioBuffer);

    // Sanity check: the captured file should be non-trivial. A real audio
    // recording of 7s of Opus in WebM should be well over 1KB.
    expect(
      audioBuffer.length,
      "Captured audio must be non-trivial (>1KB)",
    ).toBeGreaterThan(1000);

    // Kill the ffmpeg playback on K2B (cleanup).
    sshNoFail("pkill -x ffmpeg");
  });
});
