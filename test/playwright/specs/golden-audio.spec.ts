/**
 * golden-audio.spec.ts — End-to-end audio through the real SPAs.
 *
 * ╔══════════════════════════════════════════════════════════════════════════╗
 * ║  This is the golden test: it proves real audio flows through the whole    ║
 * ║  system exactly as human operators would drive it.                        ║
 * ║                                                                            ║
 * ║   • Admin SPA   — logs in and creates the session (real UI).              ║
 * ║   • Translator SPA — two browsers connect with real microphones           ║
 * ║       (Chromium fake mic fed a WAV tone) over real WebRTC.                ║
 * ║   • Broadcast SPA — a listener browser plays the live stream.             ║
 * ║   • ct-server   — real Pion SFU forwards the Opus RTP between them.       ║
 * ║                                                                            ║
 * ║  Topology under test:                                                     ║
 * ║      Floor  ──produce→ "feed"      ─▶ Translator (hears floor)            ║
 * ║      Translator ──produce→ "broadcast" ─▶ Broadcast listener (hears xl8r) ║
 * ║                                                                            ║
 * ║  Each producer emits a distinct tone. The test decodes the audio actually ║
 * ║  received in each listener browser (AnalyserNode FFT) and asserts the     ║
 * ║  correct tone arrives at the correct destination — no mocks, no stubs.    ║
 * ╚══════════════════════════════════════════════════════════════════════════╝
 */
import { test, expect, chromium, type Browser } from "@playwright/test";
import * as os from "os";
import * as path from "path";
import {
  BASE_URL,
  adminLoginUI,
  apiFetch,
  createChannel,
  getBroadcastToken,
  makeToneWav,
  installInboundAudioCapture,
  dominantFrequency,
  expectTone,
} from "../helpers";

// Distinct, well-separated tones for the two producers.
const FLOOR_HZ = 440;
const TRANSLATOR_HZ = 880;

const floorWav = path.join(os.tmpdir(), "ct-e2e-floor.wav");
const translatorWav = path.join(os.tmpdir(), "ct-e2e-translator.wav");

// A Chromium instance whose fake microphone plays the given WAV tone.
function launchWithMic(toneFile: string): Promise<Browser> {
  return chromium.launch({
    args: [
      "--use-fake-ui-for-media-stream",
      "--use-fake-device-for-media-stream",
      `--use-file-for-fake-audio-capture=${toneFile}`,
      "--autoplay-policy=no-user-gesture-required",
      // Emit real host ICE candidates instead of mDNS ".local" names, which
      // the Pion SFU cannot resolve on localhost — without this DTLS/SRTP never
      // establishes and no audio flows.
      "--disable-features=WebRtcHideLocalIpsWithMdns",
    ],
  });
}

test.describe("Golden Audio — SPA-driven end-to-end", () => {
  test.setTimeout(180_000);

  test.beforeAll(() => {
    // Long tones so Chromium's fake mic keeps producing for the whole test.
    makeToneWav(floorWav, FLOOR_HZ, 60);
    makeToneWav(translatorWav, TRANSLATOR_HZ, 60);
  });

  test("floor→feed→translator and translator→broadcast→listener", async ({
    page,
    request,
  }) => {
    // ══ 1. Admin SPA: log in and create the session ═══════════════════════
    const adminToken = await adminLoginUI(page);

    const sessionName = `E2E Service ${Date.now()}`;
    await page.getByRole("link", { name: /sessions/i }).click();
    await expect(page).toHaveURL(/\/admin\/sessions/);
    await page.getByRole("button", { name: /new session/i }).click();
    await page.getByPlaceholder(/session name/i).fill(sessionName);
    await page.getByRole("button", { name: /^create$/i }).click();
    await expect(page.getByText(sessionName)).toBeVisible({ timeout: 15_000 });

    // Resolve the session id + create its channels (no admin UI for channels).
    const sessions = (await apiFetch(request, adminToken, "get", "/api/sessions"))
      .data as Array<{ id: string; name: string }>;
    const session = sessions.find((s) => s.name === sessionName);
    expect(session, "created session present via API").toBeTruthy();
    const sessionId = session!.id;

    const feed = await createChannel(request, adminToken, sessionId, "Floor Feed", "feed");
    await createChannel(request, adminToken, sessionId, "English Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    // Two translator accounts: one drives the "floor" source, one translates.
    // Unique per run so Playwright retries don't collide on the username.
    const pw = "audio-pass-123";
    const floorUser = `floor_${Date.now()}`;
    const mariaUser = `maria_${Date.now()}`;
    await apiFetch(request, adminToken, "post", "/api/translators", {
      username: floorUser,
      password: pw,
    });
    await apiFetch(request, adminToken, "post", "/api/translators", {
      username: mariaUser,
      password: pw,
    });

    // ══ 2. Floor browser: connect a mic (440Hz) producing into "feed" ══════
    const floorBrowser = await launchWithMic(floorWav);
    const floorCtx = await floorBrowser.newContext({
      baseURL: BASE_URL,
      permissions: ["microphone"],
    });
    await installInboundAudioCapture(floorCtx);
    const floorPage = await floorCtx.newPage();
    await loginTranslator(floorPage, floorUser, pw);
    // Deep link: produce into the feed channel, listen to nothing.
    await floorPage.goto(
      `/translator/sessions/${sessionId}/connect?produce=${encodeURIComponent(feed.name)}&listen=`,
    );
    await floorPage.getByRole("button", { name: /^connect$/i }).click();
    await expect(
      floorPage.getByRole("button", { name: /disconnect/i }),
    ).toBeVisible({ timeout: 30_000 });

    // ══ 3. Translator browser: mic (880Hz) → broadcast, listening to feed ══
    const transBrowser = await launchWithMic(translatorWav);
    const transCtx = await transBrowser.newContext({
      baseURL: BASE_URL,
      permissions: ["microphone"],
    });
    await installInboundAudioCapture(transCtx);
    const transPage = await transCtx.newPage();
    await loginTranslator(transPage, mariaUser, pw);
    // Default routing for a translator: produce → broadcast, listen → feed.
    await transPage.goto(`/translator/sessions/${sessionId}/connect`);
    await transPage.getByRole("button", { name: /^connect$/i }).click();
    await expect(
      transPage.getByRole("button", { name: /disconnect/i }),
    ).toBeVisible({ timeout: 30_000 });

    // ══ 4. Broadcast browser: listen to the session ═══════════════════════
    const listenBrowser = await chromium.launch({
      args: [
        "--use-fake-ui-for-media-stream",
        "--use-fake-device-for-media-stream",
        "--autoplay-policy=no-user-gesture-required",
        "--disable-features=WebRtcHideLocalIpsWithMdns",
      ],
    });
    const listenCtx = await listenBrowser.newContext({ baseURL: BASE_URL });
    await installInboundAudioCapture(listenCtx);
    const listenPage = await listenCtx.newPage();
    await listenPage.goto(
      `/broadcast/listen/${sessionId}?t=${encodeURIComponent(broadcastToken)}`,
    );
    await listenPage.getByRole("button", { name: /listen/i }).click();

    // Give the SFU a moment to establish both hops and pipe audio.
    await transPage.waitForTimeout(4000);

    // ══ 5. Verify the correct tone reached each destination ════════════════
    const heardByTranslator = await dominantFrequency(transPage, 2500);
    const heardByListener = await dominantFrequency(listenPage, 2500);

    // eslint-disable-next-line no-console
    console.log(
      `translator heard ${heardByTranslator.hz}Hz; broadcast heard ${heardByListener.hz}Hz`,
    );

    expectTone(heardByTranslator.hz, FLOOR_HZ, "translator (feed)");
    expectTone(heardByListener.hz, TRANSLATOR_HZ, "broadcast");

    // Correct-destination: tones must not be swapped across channels.
    expect(Math.abs(heardByTranslator.hz - TRANSLATOR_HZ)).toBeGreaterThan(25);
    expect(Math.abs(heardByListener.hz - FLOOR_HZ)).toBeGreaterThan(25);

    await Promise.all([
      floorBrowser.close(),
      transBrowser.close(),
      listenBrowser.close(),
    ]);
  });
});

// loginTranslator logs into the translator SPA (persists auth for deep links).
async function loginTranslator(
  p: import("@playwright/test").Page,
  username: string,
  password: string,
): Promise<void> {
  await p.goto("/translator/login");
  await p.fill("#username", username);
  await p.fill("#password", password);
  await p.getByRole("button", { name: /sign in|log in/i }).click();
  await expect(p.getByText(/logged in as/i)).toBeVisible({ timeout: 15_000 });
}
