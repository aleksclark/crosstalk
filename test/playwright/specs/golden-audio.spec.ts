/**
 * golden-audio.spec.ts — E2E golden audio tests via Playwright + WebRTC.
 *
 * Browser→K2B:  Playwright connects as "translator" with a fake audio capture
 *               file (test-tone-1khz-5s.wav injected via --use-file-for-fake-audio-capture).
 *               The server SFU forwards the audio to ct-client on K2B, which
 *               outputs to PipeWire.  The orchestrator (run-e2e-tests.sh) captures
 *               the K2B output and runs compare-audio.sh.
 *
 * K2B→Browser:  ct-client on K2B sends audio from PipeWire loopback source.
 *               Playwright captures the received WebRTC audio track using the
 *               Web Audio API + MediaRecorder and saves to a file.  The
 *               orchestrator runs compare-audio.sh on the result.
 *
 * These tests are driven by run-e2e-tests.sh which sets environment variables:
 *   CT_SERVER_URL    — server base URL
 *   CT_SESSION_ID    — session to join
 *   CT_ROLE          — role to connect as (translator)
 *   CT_TEST_MODE     — "browser-to-k2b" or "k2b-to-browser"
 *   CT_CAPTURE_PATH  — where to save captured audio (k2b-to-browser only)
 *   CT_AUDIO_DURATION_MS — how long to wait for audio (default 7000)
 */
import { test, expect } from "@playwright/test";
import * as fs from "fs";
import * as path from "path";

const SERVER_URL = process.env.CT_SERVER_URL || "http://localhost:8080";
const SESSION_ID = process.env.CT_SESSION_ID || "";
const ROLE = process.env.CT_ROLE || "translator";
const TEST_MODE = process.env.CT_TEST_MODE || "browser-to-k2b";
const CAPTURE_PATH = process.env.CT_CAPTURE_PATH || "/tmp/browser-captured-audio.webm";
const AUDIO_DURATION = parseInt(process.env.CT_AUDIO_DURATION_MS || "7000", 10);

test.describe("Golden Audio Tests", () => {
  test.skip(!SESSION_ID, "CT_SESSION_ID not set — run via run-e2e-tests.sh");

  test("Browser→K2B: fake audio capture flows through WebRTC to K2B", async ({
    browser,
  }) => {
    test.skip(TEST_MODE !== "browser-to-k2b", "Not in browser-to-k2b mode");

    // Launch context with fake audio file — Chromium will use the test tone
    // as the fake microphone input (set via launchOptions in playwright.config.ts
    // plus --use-file-for-fake-audio-capture passed by the orchestrator).
    const context = await browser.newContext({
      permissions: ["microphone"],
    });
    const page = await context.newPage();

    // Navigate to the session connect page
    const connectURL = `${SERVER_URL}/session/${SESSION_ID}/connect?role=${ROLE}`;
    await page.goto(connectURL);

    // Wait for the WebRTC connection to establish.
    // The UI should show a "connected" indicator or similar.
    // We look for common connection-ready signals.
    await page.waitForFunction(
      () => {
        // Check for any RTCPeerConnection in connected/completed state
        // by looking at the page's connection status indicator or global state
        const el =
          document.querySelector("[data-testid='connection-status']") ||
          document.querySelector(".connection-status") ||
          document.querySelector("[data-connected]");
        if (el) {
          const text = el.textContent?.toLowerCase() || "";
          return (
            text.includes("connected") ||
            el.getAttribute("data-connected") === "true"
          );
        }
        // Fallback: check if there's a running peer connection via JS
        return (window as any).__rtcConnected === true;
      },
      { timeout: 15_000 },
    );

    // The fake audio capture is already playing the test tone into the
    // WebRTC peer connection. Wait for the audio to flow through.
    await page.waitForTimeout(AUDIO_DURATION);

    // The orchestrator captures audio on the K2B side — we just need to
    // keep the connection alive long enough for audio to flow.
    // Signal completion by setting a flag the orchestrator can check.
    await context.close();
  });

  test("K2B→Browser: capture received WebRTC audio", async ({ browser }) => {
    test.skip(TEST_MODE !== "k2b-to-browser", "Not in k2b-to-browser mode");

    const context = await browser.newContext({
      permissions: ["microphone"],
    });
    const page = await context.newPage();

    const connectURL = `${SERVER_URL}/session/${SESSION_ID}/connect?role=${ROLE}`;
    await page.goto(connectURL);

    // Wait for WebRTC connection
    await page.waitForFunction(
      () => {
        const el =
          document.querySelector("[data-testid='connection-status']") ||
          document.querySelector(".connection-status") ||
          document.querySelector("[data-connected]");
        if (el) {
          const text = el.textContent?.toLowerCase() || "";
          return (
            text.includes("connected") ||
            el.getAttribute("data-connected") === "true"
          );
        }
        return (window as any).__rtcConnected === true;
      },
      { timeout: 15_000 },
    );

    // Capture received audio using Web Audio API + MediaRecorder.
    // Inject a script that hooks into the received audio track and records it.
    const audioBase64 = await page.evaluate(
      async (durationMs: number) => {
        return new Promise<string>((resolve, reject) => {
          try {
            // Find the audio element or remote stream
            const audioElements = document.querySelectorAll("audio, video");
            let stream: MediaStream | null = null;

            // Try to get the stream from an audio/video element
            for (const el of audioElements) {
              const mediaEl = el as HTMLMediaElement;
              if (mediaEl.srcObject instanceof MediaStream) {
                stream = mediaEl.srcObject;
                break;
              }
            }

            // Fallback: look for exposed stream on window
            if (!stream && (window as any).__remoteStream) {
              stream = (window as any).__remoteStream;
            }

            if (!stream) {
              reject(new Error("No remote audio stream found"));
              return;
            }

            // Use MediaRecorder to capture the audio
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

            // Record for the specified duration
            setTimeout(() => {
              recorder.stop();
            }, durationMs);
          } catch (err) {
            reject(err);
          }
        });
      },
      AUDIO_DURATION,
    );

    // Write captured audio to file
    const audioBuffer = Buffer.from(audioBase64, "base64");
    const captureDir = path.dirname(CAPTURE_PATH);
    if (!fs.existsSync(captureDir)) {
      fs.mkdirSync(captureDir, { recursive: true });
    }
    fs.writeFileSync(CAPTURE_PATH, audioBuffer);

    expect(audioBuffer.length).toBeGreaterThan(1000);

    await context.close();
  });
});
