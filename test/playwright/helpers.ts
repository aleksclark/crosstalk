/**
 * CrossTalk v3 Playwright helpers.
 *
 * These utilities drive the real SPAs (admin, translator, broadcast) against a
 * live ct-server, and provide the primitives for the end-to-end audio test:
 * generating tone WAVs for Chromium's fake microphone, capturing the inbound
 * WebRTC audio in a browser page, and detecting its dominant frequency.
 */
import {
  type Page,
  type BrowserContext,
  type APIRequestContext,
  expect,
} from "@playwright/test";
import * as fs from "fs";

export const BASE_URL = process.env.CT_SERVER_URL || "http://localhost:8080";
export const ADMIN_USERNAME = "admin";
export const ADMIN_PASSWORD = process.env.CT_ADMIN_PASSWORD || "admin";

// ── Auth / UI ────────────────────────────────────────────────────────────

/** Read the admin JWT the SPA stored in localStorage after login. */
async function readToken(page: Page): Promise<string> {
  const raw = await page.evaluate(() =>
    window.localStorage.getItem("crosstalk_auth"),
  );
  expect(raw, "admin auth not stored").toBeTruthy();
  const token = JSON.parse(raw!).token as string;
  expect(token, "admin token missing").toBeTruthy();
  return token;
}

/**
 * Log in through the admin SPA UI and return the access token.
 * Navigates to /admin/login, submits credentials, waits for the dashboard.
 */
export async function adminLoginUI(page: Page): Promise<string> {
  await page.goto("/admin/login");
  await page.locator('input[type="text"]').first().fill(ADMIN_USERNAME);
  await page.locator('input[type="password"]').first().fill(ADMIN_PASSWORD);
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page).toHaveURL(/\/admin\/dashboard/, { timeout: 15_000 });
  return readToken(page);
}

/** Log in through the translator SPA UI, landing on the sessions list. */
export async function translatorLoginUI(
  page: Page,
  username: string,
  password: string,
): Promise<void> {
  await page.goto("/translator/login");
  await page.fill("#username", username);
  await page.fill("#password", password);
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await expect(page.getByText(/logged in as/i)).toBeVisible({ timeout: 15_000 });
}

// ── REST helpers (for setup the SPAs don't expose, e.g. channels) ─────────

export async function apiFetch(
  request: APIRequestContext,
  token: string,
  method: "get" | "post" | "put" | "delete",
  path: string,
  body?: unknown,
): Promise<Record<string, unknown>> {
  const resp = await request[method](`${BASE_URL}${path}`, {
    headers: { Authorization: `Bearer ${token}` },
    data: body as never,
  });
  expect(resp.ok(), `${method} ${path} -> ${resp.status()}`).toBeTruthy();
  const text = await resp.text();
  return text ? (JSON.parse(text) as Record<string, unknown>) : {};
}

export async function createChannel(
  request: APIRequestContext,
  token: string,
  sessionId: string,
  name: string,
  type: "feed" | "broadcast",
): Promise<{ id: string; name: string }> {
  const ch = await apiFetch(
    request,
    token,
    "post",
    `/api/sessions/${sessionId}/channels`,
    { name, type },
  );
  return { id: ch.id as string, name: ch.name as string };
}

export async function getBroadcastToken(
  request: APIRequestContext,
  token: string,
  sessionId: string,
): Promise<string> {
  const info = await apiFetch(
    request,
    token,
    "get",
    `/api/sessions/${sessionId}/broadcast-url`,
  );
  return info.broadcast_token as string;
}

/**
 * Register an ABC (booth) assigned to a session and return the one-time API
 * token. Translators cannot produce into feed channels — floor audio must
 * come from an ABC on /ws/signaling (production board path).
 */
export async function createAssignedABC(
  request: APIRequestContext,
  adminToken: string,
  sessionId: string,
  name: string,
): Promise<{ id: string; token: string; name: string }> {
  const created = await apiFetch(request, adminToken, "post", "/api/abcs", {
    name,
  });
  const id = created.id as string;
  const abcToken = created.token as string;
  expect(id, "ABC id").toBeTruthy();
  expect(abcToken, "ABC token shown once").toBeTruthy();
  await apiFetch(request, adminToken, "put", `/api/abcs/${id}`, {
    name,
    session_id: sessionId,
  });
  return { id, token: abcToken, name };
}

/**
 * Drive a browser as an ABC floor producer: mic → /ws/signaling with the ABC
 * API token (client-offer). Keeps PC/WS on window so the connection lives for
 * the test duration. Requires a context launched with fake-mic + permissions.
 */
export async function connectABCFloorProducer(
  page: Page,
  abcToken: string,
): Promise<void> {
  // Land on same-origin so relative WS host matches the SPA under test.
  await page.goto("/admin/login");
  await page.evaluate(async (token) => {
    const w = window as unknown as {
      __ctAbcPc?: RTCPeerConnection;
      __ctAbcWs?: WebSocket;
      __ctAbcStream?: MediaStream;
      __ctAbcReady?: Promise<void>;
      __ctAbcError?: string;
    };

    const stream = await navigator.mediaDevices.getUserMedia({
      audio: true,
      video: false,
    });
    w.__ctAbcStream = stream;

    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });
    w.__ctAbcPc = pc;
    // Control channel — server adopts it (same as translator SPA / ABC client).
    pc.createDataChannel("control");
    stream.getTracks().forEach((t) => pc.addTrack(t, stream));

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(
      `${proto}//${window.location.host}/ws/signaling?token=${encodeURIComponent(token)}`,
    );
    w.__ctAbcWs = ws;

    const pending: RTCIceCandidateInit[] = [];
    let wsOpen = false;

    const send = (obj: unknown) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(obj));
      }
    };

    pc.onicecandidate = (ev) => {
      if (!ev.candidate) return;
      const init = ev.candidate.toJSON();
      if (wsOpen) {
        send({ type: "candidate", candidate: init });
      } else {
        pending.push(init);
      }
    };

    w.__ctAbcReady = new Promise<void>((resolve, reject) => {
      const deadline = window.setTimeout(() => {
        w.__ctAbcError = `abc ICE timeout ice=${pc.iceConnectionState} conn=${pc.connectionState}`;
        reject(new Error(w.__ctAbcError));
      }, 45_000);

      const maybeDone = () => {
        if (
          pc.iceConnectionState === "connected" ||
          pc.iceConnectionState === "completed" ||
          pc.connectionState === "connected"
        ) {
          window.clearTimeout(deadline);
          resolve();
        }
      };
      pc.oniceconnectionstatechange = maybeDone;
      pc.onconnectionstatechange = maybeDone;

      ws.onerror = () => {
        w.__ctAbcError = "abc websocket error";
      };
      ws.onclose = (ev) => {
        if (pc.connectionState !== "connected") {
          w.__ctAbcError = `abc ws closed code=${ev.code} ${ev.reason}`;
        }
      };

      ws.onopen = () => {
        wsOpen = true;
        for (const c of pending.splice(0)) {
          send({ type: "candidate", candidate: c });
        }
      };

      ws.onmessage = async (ev) => {
        try {
          const msg = JSON.parse(String(ev.data)) as {
            type: string;
            sdp?: string;
            candidate?: RTCIceCandidateInit;
          };
          if (msg.type === "answer" && msg.sdp) {
            await pc.setRemoteDescription({ type: "answer", sdp: msg.sdp });
            maybeDone();
          } else if (msg.type === "offer" && msg.sdp) {
            await pc.setRemoteDescription({ type: "offer", sdp: msg.sdp });
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            send({ type: "answer", sdp: answer.sdp });
          } else if (
            (msg.type === "candidate" || msg.type === "ice") &&
            msg.candidate
          ) {
            await pc.addIceCandidate(msg.candidate);
          }
        } catch (err) {
          w.__ctAbcError = err instanceof Error ? err.message : String(err);
        }
      };

      // Create + send offer once WS is open (or immediately if already open).
      void (async () => {
        try {
          const offer = await pc.createOffer();
          await pc.setLocalDescription(offer);
          const pushOffer = () => send({ type: "offer", sdp: offer.sdp });
          if (ws.readyState === WebSocket.OPEN) {
            pushOffer();
          } else {
            ws.addEventListener("open", pushOffer, { once: true });
          }
        } catch (err) {
          window.clearTimeout(deadline);
          w.__ctAbcError = err instanceof Error ? err.message : String(err);
          reject(err instanceof Error ? err : new Error(String(err)));
        }
      })();
    });

    await w.__ctAbcReady;
  }, abcToken);

  // Surface browser-side failure with a clear Playwright error.
  const err = await page.evaluate(
    () => (window as unknown as { __ctAbcError?: string }).__ctAbcError ?? "",
  );
  if (err) {
    throw new Error(`ABC floor producer failed: ${err}`);
  }
  await expect
    .poll(
      async () =>
        page.evaluate(() => {
          const pc = (window as unknown as { __ctAbcPc?: RTCPeerConnection })
            .__ctAbcPc;
          return pc?.connectionState ?? pc?.iceConnectionState ?? "missing";
        }),
      { timeout: 45_000 },
    )
    .toMatch(/connected|completed/);
}

// ── Tone generation (Chromium fake mic input) ────────────────────────────

/** Write a mono 16-bit PCM WAV of a pure sine at `hz` for `seconds` @ 48kHz. */
export function makeToneWav(path: string, hz: number, seconds = 5): void {
  const rate = 48_000;
  const n = rate * seconds;
  const bytesPerSample = 2;
  const dataSize = n * bytesPerSample;
  const buf = Buffer.alloc(44 + dataSize);
  // RIFF header
  buf.write("RIFF", 0);
  buf.writeUInt32LE(36 + dataSize, 4);
  buf.write("WAVE", 8);
  buf.write("fmt ", 12);
  buf.writeUInt32LE(16, 16); // PCM chunk size
  buf.writeUInt16LE(1, 20); // PCM
  buf.writeUInt16LE(1, 22); // mono
  buf.writeUInt32LE(rate, 24);
  buf.writeUInt32LE(rate * bytesPerSample, 28); // byte rate
  buf.writeUInt16LE(bytesPerSample, 32); // block align
  buf.writeUInt16LE(16, 34); // bits/sample
  buf.write("data", 36);
  buf.writeUInt32LE(dataSize, 40);
  for (let i = 0; i < n; i++) {
    const v = Math.sin((2 * Math.PI * hz * i) / rate) * 0.6 * 0x7fff;
    buf.writeInt16LE(Math.round(v), 44 + i * bytesPerSample);
  }
  fs.writeFileSync(path, buf);
}

// ── In-browser WebRTC audio capture + frequency analysis ──────────────────

/**
 * Install a hook (before any page script runs) that records the audio tracks
 * arriving on every RTCPeerConnection into window.__ctInboundStreams. This lets
 * the test analyze what a listener actually receives, independent of whether the
 * SPA attaches the stream to an <audio> element.
 */
export async function installInboundAudioCapture(
  ctx: BrowserContext,
): Promise<void> {
  await ctx.addInitScript(() => {
    // @ts-expect-error test-only global
    window.__ctInboundStreams = [];
    // @ts-expect-error test-only global
    window.__ctPCs = [];
    const Orig = window.RTCPeerConnection;
    // @ts-expect-error wrap constructor
    window.RTCPeerConnection = function (...args: unknown[]) {
      const pc = new Orig(...(args as []));
      // @ts-expect-error test-only global
      window.__ctPCs.push(pc);
      pc.addEventListener("track", (ev: RTCTrackEvent) => {
        const stream = ev.streams[0] ?? new MediaStream([ev.track]);
        // @ts-expect-error test-only global
        window.__ctInboundStreams.push(stream);
        // Attach to an autoplaying <audio> element. Chromium only pulls/decodes
        // a remote audio track when it is consumed by a media element; without
        // this the WebAudio AnalyserNode sees silence.
        try {
          const el = document.createElement("audio");
          el.autoplay = true;
          el.srcObject = stream;
          (document.body ?? document.documentElement).appendChild(el);
          void el.play().catch(() => {});
        } catch {
          /* ignore */
        }
      });
      return pc;
    };
    // @ts-expect-error preserve prototype/statics
    window.RTCPeerConnection.prototype = Orig.prototype;
  });
}

/**
 * Analyze the most-recently-received inbound audio stream and return its
 * dominant frequency in Hz (0 if no audio/energy). Runs an AnalyserNode FFT for
 * ~`windowMs`, taking the modal peak bin across snapshots for robustness.
 */
export async function dominantFrequency(
  page: Page,
  windowMs = 2000,
): Promise<{ hz: number; energy: number; streams: number; tracks: string }> {
  return page.evaluate(async (windowMsInner) => {
    // @ts-expect-error test-only global
    const streams: MediaStream[] = window.__ctInboundStreams || [];
    const stream = streams[streams.length - 1];
    const tracks = streams
      .map((s) =>
        s
          .getAudioTracks()
          .map((t) => `${t.readyState}/${t.muted ? "muted" : "live"}`)
          .join(","),
      )
      .join(" | ");
    if (!stream) return { hz: 0, energy: 0, streams: streams.length, tracks };

    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext })
        .webkitAudioContext;
    const ctx = new AudioCtx();
    if (ctx.state === "suspended") await ctx.resume();
    const src = ctx.createMediaStreamSource(stream);
    const analyser = ctx.createAnalyser();
    analyser.fftSize = 8192;
    src.connect(analyser);
    // Keep the graph "pulling" by routing through a silent gain to output.
    const silent = ctx.createGain();
    silent.gain.value = 0;
    analyser.connect(silent);
    silent.connect(ctx.destination);

    const bins = analyser.frequencyBinCount;
    const data = new Float32Array(bins);
    const binHz = ctx.sampleRate / analyser.fftSize;

    const counts = new Map<number, number>();
    let peakEnergy = 0;
    const start = performance.now();
    while (performance.now() - start < windowMsInner) {
      analyser.getFloatFrequencyData(data);
      let maxDb = -Infinity;
      let maxBin = 0;
      // Ignore the lowest bins (DC/hum) below ~150Hz.
      const minBin = Math.ceil(150 / binHz);
      for (let i = minBin; i < bins; i++) {
        if (data[i] > maxDb) {
          maxDb = data[i];
          maxBin = i;
        }
      }
      if (maxDb > -90) {
        counts.set(maxBin, (counts.get(maxBin) ?? 0) + 1);
        peakEnergy = Math.max(peakEnergy, maxDb);
      }
      await new Promise((r) => setTimeout(r, 50));
    }
    ctx.close();

    let modalBin = 0;
    let best = 0;
    for (const [bin, c] of counts) {
      if (c > best) {
        best = c;
        modalBin = bin;
      }
    }
    return { hz: Math.round(modalBin * binHz), energy: peakEnergy };
  }, windowMs);
}

/** Assert a detected frequency is within tolerance of the expected tone. */
export function expectTone(
  detectedHz: number,
  expectedHz: number,
  label: string,
  tolHz = 25,
): void {
  expect(
    Math.abs(detectedHz - expectedHz),
    `${label}: expected ~${expectedHz}Hz, detected ${detectedHz}Hz`,
  ).toBeLessThanOrEqual(tolHz);
}

/**
 * Analyze EVERY inbound audio stream and return each one's dominant frequency
 * in Hz. Used when a page monitors multiple channels at once (e.g. the
 * translator, which monitors every session channel) so a test can assert a
 * given tone is present among the received streams rather than assuming a
 * single stream.
 */
export async function dominantFrequencies(
  page: Page,
  windowMs = 2000,
): Promise<number[]> {
  return page.evaluate(async (windowMsInner) => {
    // @ts-expect-error test-only global
    const streams: MediaStream[] = window.__ctInboundStreams || [];
    if (streams.length === 0) return [];

    const AudioCtx =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext })
        .webkitAudioContext;
    const ctx = new AudioCtx();
    if (ctx.state === "suspended") await ctx.resume();

    const analysers = streams.map((stream) => {
      const src = ctx.createMediaStreamSource(stream);
      const analyser = ctx.createAnalyser();
      analyser.fftSize = 8192;
      src.connect(analyser);
      const silent = ctx.createGain();
      silent.gain.value = 0;
      analyser.connect(silent);
      silent.connect(ctx.destination);
      return analyser;
    });

    const bins = analysers[0]!.frequencyBinCount;
    const binHz = ctx.sampleRate / analysers[0]!.fftSize;
    const minBin = Math.ceil(150 / binHz);
    const data = new Float32Array(bins);
    const counts = analysers.map(() => new Map<number, number>());

    const start = performance.now();
    while (performance.now() - start < windowMsInner) {
      analysers.forEach((analyser, idx) => {
        analyser.getFloatFrequencyData(data);
        let maxDb = -Infinity;
        let maxBin = 0;
        for (let i = minBin; i < bins; i++) {
          if (data[i] > maxDb) {
            maxDb = data[i];
            maxBin = i;
          }
        }
        if (maxDb > -90) {
          counts[idx]!.set(maxBin, (counts[idx]!.get(maxBin) ?? 0) + 1);
        }
      });
      await new Promise((r) => setTimeout(r, 50));
    }
    ctx.close();

    return counts.map((c) => {
      let modalBin = 0;
      let best = 0;
      for (const [bin, n] of c) {
        if (n > best) {
          best = n;
          modalBin = bin;
        }
      }
      return Math.round(modalBin * binHz);
    });
  }, windowMs);
}

/** Diagnostic: report the last peer connection's state + RTP byte counters. */
export async function rtcReport(page: Page): Promise<string> {
  return page.evaluate(async () => {
    // @ts-expect-error test-only global
    const pcs: RTCPeerConnection[] = window.__ctPCs || [];
    const pc = pcs[pcs.length - 1];
    if (!pc) return "no pc";
    let inBytes = 0;
    let outBytes = 0;
    let pair = "";
    const stats = await pc.getStats();
    stats.forEach((s: Record<string, unknown>) => {
      if (s.type === "inbound-rtp") inBytes += (s.bytesReceived as number) ?? 0;
      if (s.type === "outbound-rtp") outBytes += (s.bytesSent as number) ?? 0;
      if (s.type === "candidate-pair" && s.nominated) {
        pair = `${s.state as string}`;
      }
    });
    const tx = pc
      .getTransceivers()
      .map(
        (t) =>
          `${t.currentDirection ?? t.direction}:${t.sender.track ? t.sender.track.readyState : "notrack"}`,
      )
      .join(",");
    return `conn=${pc.connectionState} ice=${pc.iceConnectionState} pair=${pair} in=${inBytes} out=${outBytes} tx=[${tx}]`;
  });
}

// ── Broadcast Helpers ────────────────────────────────────────────────────

/**
 * Create a template with a broadcast mapping via the REST API.
 * By default the template maps "studio:mic → broadcast" and includes
 * a regular role-to-role mapping so the template has two roles.
 */
export async function createBroadcastTemplateViaAPI(
  request: APIRequestContext,
  token: string,
  overrides: Record<string, unknown> = {},
): Promise<Record<string, unknown>> {
  return createTemplateViaAPI(request, token, {
    name: "Broadcast Template",
    roles: [
      { name: "studio", multi_client: false },
      { name: "translator", multi_client: false },
    ],
    mappings: [
      { source: "studio:mic", sink: "broadcast" },
      { source: "translator:mic", sink: "studio:output" },
    ],
    ...overrides,
  });
}

/**
 * Generate a broadcast token for the given session via the REST API.
 * Returns the full response: { token, url, expires_at }.
 */
export async function createBroadcastTokenViaAPI(
  request: APIRequestContext,
  token: string,
  sessionId: string,
): Promise<{ token: string; url: string; expires_at: string }> {
  const resp = await request.post(
    `${BASE_URL}/api/sessions/${sessionId}/broadcast-token`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  expect(resp.ok()).toBeTruthy();
  return (await resp.json()) as { token: string; url: string; expires_at: string };
}

/**
 * Fetch the session detail via the REST API and return listener_count.
 */
export async function getSessionListenerCount(
  request: APIRequestContext,
  token: string,
  sessionId: string,
): Promise<number> {
  const resp = await request.get(`${BASE_URL}/api/sessions/${sessionId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(resp.ok()).toBeTruthy();
  const body = (await resp.json()) as Record<string, unknown>;
  return (body.listener_count as number) ?? 0;
}
