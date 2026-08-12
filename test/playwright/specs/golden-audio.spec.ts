/**
 * golden-audio.spec.ts — End-to-end audio through the real SPAs.
 *
 * ╔══════════════════════════════════════════════════════════════════════════╗
 * ║  This is the golden test: it proves real audio flows through the whole    ║
 * ║  system exactly as human operators would drive it.                        ║
 * ║                                                                            ║
 * ║   • Admin SPA   — logs in and creates the session (real UI).              ║
 * ║   • ABC booth   — floor mic produces into "feed" via /ws/signaling        ║
 * ║       (translators cannot expand produce into feed; ABC is the board).    ║
 * ║   • Translator SPA — browser connects with real mic over WebRTC,          ║
 * ║       listens to feed and produces into broadcast.                        ║
 * ║   • Broadcast SPA — a listener browser plays the live stream.             ║
 * ║   • ct-server   — real Pion SFU + decoded mixer forwards Opus.            ║
 * ║                                                                            ║
 * ║  Topology under test:                                                     ║
 * ║      Floor(ABC) ──produce→ "feed"      ─▶ Translator (hears floor)        ║
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
  createAssignedABC,
  connectABCFloorProducer,
  getBroadcastToken,
  makeToneWav,
  installInboundAudioCapture,
  dominantFrequency,
  dominantFrequencies,
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
    await page.getByRole("navigation").getByRole("link", { name: "Sessions", exact: true }).click();
    await expect(page).toHaveURL(/\/admin\/sessions/);
    await page.getByRole("button", { name: /new session/i }).click();
    await page.getByRole("textbox", { name: /session name/i }).fill(sessionName);
    await page.getByRole("button", { name: /^create$/i }).click();
    await expect(page.getByRole("link", { name: sessionName, exact: true }).first()).toBeVisible({
      timeout: 15_000,
    });

    // Resolve the session id + create its channels (no admin UI for channels).
    const sessionsBody = await apiFetch(request, adminToken, "get", "/api/sessions");
    const sessions = (sessionsBody.data ?? sessionsBody) as Array<{
      id: string;
      name: string;
    }>;
    const session = sessions.find((s) => s.name === sessionName);
    expect(session, "created session present via API").toBeTruthy();
    const sessionId = session!.id;

    const feed = await createChannel(request, adminToken, sessionId, "Floor Feed", "feed");
    await createChannel(request, adminToken, sessionId, "English Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    // Floor producer = ABC (only role that may produce into feed by default).
    const floorAbc = await createAssignedABC(
      request,
      adminToken,
      sessionId,
      `Floor Booth ${Date.now()}`,
    );

    // Translator account assigned to the session (media tickets fail closed otherwise).
    const pw = "audio-pass-123";
    const mariaUser = `maria_${Date.now()}`;
    const mariaAcct = await apiFetch(request, adminToken, "post", "/api/translators", {
      username: mariaUser,
      password: pw,
    });
    await apiFetch(
      request,
      adminToken,
      "put",
      `/api/translators/${mariaAcct.id as string}/sessions`,
      { session_ids: [sessionId] },
    );

    // ══ 2. Floor browser: ABC mic (440Hz) producing into feed ══════════════
    const floorBrowser = await launchWithMic(floorWav);
    const floorCtx = await floorBrowser.newContext({
      baseURL: BASE_URL,
      permissions: ["microphone"],
    });
    // Still install capture hooks so any inbound tracks are decoded if present.
    await installInboundAudioCapture(floorCtx);
    const floorPage = await floorCtx.newPage();
    await connectABCFloorProducer(floorPage, floorAbc.token);

    // ══ 3. Translator browser: mic (880Hz) → broadcast, listening to feed ══
    const transBrowser = await launchWithMic(translatorWav);
    const transCtx = await transBrowser.newContext({
      baseURL: BASE_URL,
      permissions: ["microphone"],
    });
    await installInboundAudioCapture(transCtx);
    const transPage = await transCtx.newPage();
    await loginTranslator(transPage, mariaUser, pw);
    // Explicit deep link: produce default (broadcast) + listen to the floor feed
    // on the same PC so feed audio is captured without depending solely on the
    // separate SessionAudioManager monitor connections.
    await transPage.goto(
      `/translator/sessions/${sessionId}/connect?listen=${encodeURIComponent(feed.name)}`,
    );
    await expect(transPage.getByRole("button", { name: /^connect$/i })).toBeVisible({
      timeout: 15_000,
    });
    await transPage.getByRole("button", { name: /^connect$/i }).click();
    await expect(
      transPage.getByRole("button", { name: /disconnect/i }),
    ).toBeVisible({ timeout: 60_000 });

    // Wait until both producers register as sources (ABC floor + translator).
    // Response body is { data: SourceOut[] } — never treat the envelope as the list.
    await expect
      .poll(
        async () => {
          const body = await apiFetch(
            request,
            adminToken,
            "get",
            `/api/sessions/${sessionId}/sources`,
          );
          const srcs = (body.data ?? body) as Array<{ id: string; name: string; connected?: boolean }>;
          if (!Array.isArray(srcs)) return 0;
          return srcs.filter((s) => s.connected !== false).length;
        },
        { timeout: 30_000 },
      )
      .toBeGreaterThanOrEqual(2);

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

    // Give the SFU a moment to establish both hops and pipe audio, then poll.
    await transPage.waitForTimeout(3000);

    // ══ 5. Verify the correct tone reached each destination ════════════════
    // Poll until the floor tone is audible on the translator (feed hop) and the
    // broadcast listener hears the translator tone.
    let translatorTones: number[] = [];
    let heardByListener = { hz: 0, energy: 0, streams: 0, tracks: "" };
    const deadline = Date.now() + 35_000;
    while (Date.now() < deadline) {
      translatorTones = await dominantFrequencies(transPage, 1500);
      heardByListener = await dominantFrequency(listenPage, 1500);
      const floorOk = translatorTones.some((hz) => Math.abs(hz - FLOOR_HZ) <= 25);
      const bcastOk = Math.abs(heardByListener.hz - TRANSLATOR_HZ) <= 25;
      if (floorOk && bcastOk) break;
      await transPage.waitForTimeout(500);
    }

    // eslint-disable-next-line no-console
    console.log(
      `translator streams heard ${translatorTones.join(", ")}Hz; broadcast heard ${heardByListener.hz}Hz`,
    );

    const translatorHeardFloor = translatorTones.some(
      (hz) => Math.abs(hz - FLOOR_HZ) <= 25,
    );
    expect(
      translatorHeardFloor,
      `translator (feed): expected a ~${FLOOR_HZ}Hz stream among [${translatorTones.join(", ")}]`,
    ).toBe(true);

    expectTone(heardByListener.hz, TRANSLATOR_HZ, "broadcast");

    // Correct-destination: the floor tone must not leak onto the broadcast.
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
