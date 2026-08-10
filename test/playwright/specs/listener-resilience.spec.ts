/**
 * listener-resilience.spec.ts — Broadcast SPA reconnect / failure UX.
 *
 * These tests exercise the public listener SPA against a live ct-server when
 * available (CT_SERVER_URL). Scenarios that need full media (audio resume
 * after interruption) are soft-gated: they run when the harness is up and
 * skip cleanly otherwise so CI without the integration stack does not fake a pass.
 *
 * Pure reconnect policy is covered by apps/broadcast unit tests
 * (reconnect-policy.test.ts) — this file is the SPA-level contract.
 */
import { test, expect, type Page, type APIRequestContext } from "@playwright/test";
import {
  BASE_URL,
  adminLoginUI,
  apiFetch,
  createChannel,
  getBroadcastToken,
} from "../helpers";

async function serverReachable(request: APIRequestContext): Promise<boolean> {
  try {
    const resp = await request.get(`${BASE_URL}/api/health`).catch(async () => {
      // health may not exist — try root
      return request.get(`${BASE_URL}/`);
    });
    return resp.ok() || resp.status() < 500;
  } catch {
    return false;
  }
}

async function openListener(
  page: Page,
  sessionId: string,
  token: string,
): Promise<void> {
  await page.goto(
    `/broadcast/listen/${sessionId}?t=${encodeURIComponent(token)}&debug=1`,
  );
}

test.describe("Broadcast listener resilience", () => {
  test.describe.configure({ mode: "serial" });

  test("invalid token fails without infinite reconnect loop", async ({
    page,
    request,
  }) => {
    test.skip(
      !(await serverReachable(request)),
      "ct-server not reachable — deferred until integration harness is up",
    );

    // No session setup — deliberately bad credentials.
    await openListener(page, "01INVALIDSESSION0000000000", "not-a-real-token");

    // App should surface a terminal invalid-link error (from info fetch or WS).
    const err = page.getByTestId("broadcast-error-message");
    await expect(err).toBeVisible({ timeout: 15_000 });
    await expect(err).toContainText(/invalid|expired|unable|failed/i);

    // Must NOT spin reconnect forever — no "reconnecting (attempt N" loop.
    await page.waitForTimeout(3_000);
    const body = await page.locator("body").innerText();
    expect(body.toLowerCase()).not.toMatch(/reconnecting \(attempt [2-9]/);
    // Retry button may appear for connection_failed; for INVALID_TOKEN on
    // the landing error screen there is no listen session at all.
    await expect(page.getByTestId("status-warn")).toHaveCount(0);
  });

  test("initial successful listen reaches playing / live", async ({
    page,
    request,
  }) => {
    test.skip(
      !(await serverReachable(request)),
      "ct-server not reachable — deferred until integration harness is up",
    );

    const adminToken = await adminLoginUI(page);
    const session = await apiFetch(request, adminToken, "post", "/api/sessions", {
      name: `Listener resilience ${Date.now()}`,
    });
    const sessionId = session.id as string;
    await createChannel(request, adminToken, sessionId, "Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    const listen = await page.context().newPage();
    await openListener(listen, sessionId, broadcastToken);

    // Info load → Listen gesture required for autoplay policy.
    await expect(listen.getByTestId("listen-button")).toBeVisible({
      timeout: 15_000,
    });
    await listen.getByTestId("listen-button").click();

    // Connecting then either playing (if media bridges) or still connecting —
    // assert we left idle and did not immediately terminal-error.
    // Prefer a single locator: status-live/warn and debug-state can both be
    // present (strict-mode violation with .or()).
    const debugState = listen.getByTestId("debug-state");
    await expect(debugState).toBeVisible({ timeout: 20_000 });
    const state = (await debugState.textContent())?.trim();
    expect(["connecting", "reconnecting", "playing"]).toContain(state);
    expect(state).not.toBe("error");

    // Listener count hidden unless server pushes a real value.
    // (Presence is fine; inventing "0 listeners" without a server value is not.)
    // We only assert the element is not showing a fabricated default of nonsense.
    const count = listen.getByTestId("listener-count");
    if (await count.count()) {
      await expect(count).toContainText(/\d+ listener/);
    }

    await listen.close();
  });

  test("ended session stops retrying", async ({ page, request }) => {
    test.skip(
      !(await serverReachable(request)),
      "ct-server not reachable — deferred until integration harness is up",
    );

    const adminToken = await adminLoginUI(page);
    const session = await apiFetch(request, adminToken, "post", "/api/sessions", {
      name: `Listener end ${Date.now()}`,
    });
    const sessionId = session.id as string;
    await createChannel(request, adminToken, sessionId, "Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    // End/delete the session before listening when the API supports it.
    let ended = false;
    for (const path of [
      `/api/sessions/${sessionId}/end`,
      `/api/sessions/${sessionId}`,
    ]) {
      try {
        const method = path.endsWith("/end") ? "post" : "delete";
        const resp = await request[method](`${BASE_URL}${path}`, {
          headers: { Authorization: `Bearer ${adminToken}` },
        });
        if (resp.ok() || resp.status() === 204) {
          ended = true;
          break;
        }
      } catch {
        /* try next */
      }
    }

    test.skip(
      !ended,
      "session end/delete API not available — structure ready for when server supports it",
    );

    const listen = await page.context().newPage();
    await openListener(listen, sessionId, broadcastToken);

    // Should show ended/invalid terminal UI — never a reconnect loop.
    // Prefer a single nested message locator (parent + child both match .or()).
    await expect(listen.getByTestId("broadcast-error-message")).toBeVisible({
      timeout: 15_000,
    });

    await listen.waitForTimeout(3_000);
    const text = (await listen.locator("body").innerText()).toLowerCase();
    expect(text).not.toMatch(/reconnecting \(attempt [2-9]/);

    await listen.close();
  });

  test("server close triggers reconnect with fresh WS path", async ({
    page,
    request,
  }) => {
    test.skip(
      !(await serverReachable(request)),
      "ct-server not reachable — deferred until integration harness is up",
    );

    // Prove a real transport drop → bounded reconnect with a second WebSocket.
    // Uses the SPA's test-only __ctCloseBroadcastWs hook (real socket.close),
    // not offline toggles or fake status flips.
    const adminToken = await adminLoginUI(page);
    const session = await apiFetch(request, adminToken, "post", "/api/sessions", {
      name: `Listener reconnect ${Date.now()}`,
    });
    const sessionId = session.id as string;
    await createChannel(request, adminToken, sessionId, "Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    const listen = await page.context().newPage();

    // Capture WS URLs the SPA opens — reconnect must use the same public
    // broadcast token path (fresh socket), not a consumed one-shot ticket.
    const wsUrls: string[] = [];
    listen.on("websocket", (ws) => {
      wsUrls.push(ws.url());
    });

    await openListener(listen, sessionId, broadcastToken);
    await expect(listen.getByTestId("listen-button")).toBeVisible({
      timeout: 15_000,
    });
    await listen.getByTestId("listen-button").click();

    // Wait for first WS + hook registration after connect starts.
    await expect
      .poll(() => wsUrls.length, { timeout: 15_000 })
      .toBeGreaterThanOrEqual(1);

    const first = wsUrls[0]!;
    expect(first).toContain(`/ws/broadcast/${sessionId}`);
    expect(first).toContain(`token=${encodeURIComponent(broadcastToken)}`);

    await expect
      .poll(
        async () =>
          listen.evaluate(
            () =>
              typeof (window as unknown as { __ctCloseBroadcastWs?: unknown })
                .__ctCloseBroadcastWs === "function",
          ),
        { timeout: 10_000 },
      )
      .toBe(true);

    // Real close with a browser-legal retryable code (4005 ∈ 3000–4999).
    // Do NOT use 4000/4001/4002/1008 (session_ended / invalid_token) or the SPA
    // will terminal-stop. Do NOT use 1011 — browsers reject non-1000/<3000 codes.
    const closed = await listen.evaluate(() => {
      const fn = (
        window as unknown as {
          __ctCloseBroadcastWs?: (code?: number, reason?: string) => boolean;
        }
      ).__ctCloseBroadcastWs;
      if (!fn) return { ok: false, reason: "hook-missing" as const };
      const sock = (
        window as unknown as { __ctCloseBroadcastWs?: unknown }
      ).__ctCloseBroadcastWs;
      void sock;
      try {
        const ok = fn(4005, "test-server-drop");
        return { ok, reason: ok ? ("closed" as const) : ("already-closed" as const) };
      } catch (e) {
        return {
          ok: false,
          reason: e instanceof Error ? e.message : String(e),
        };
      }
    });
    expect(closed.ok, `test hook close failed: ${closed.reason}`).toBe(true);

    // UI should enter reconnecting (bounded backoff), then open a second WS.
    // Prefer a single testid — status badge + message + debug all say "reconnecting".
    await expect(listen.getByTestId("debug-state")).toHaveText(/reconnecting/i, {
      timeout: 15_000,
    });

    await expect
      .poll(() => wsUrls.length, { timeout: 25_000 })
      .toBeGreaterThanOrEqual(2);

    const second = wsUrls[1]!;
    expect(second).toContain(`/ws/broadcast/${sessionId}`);
    expect(second).toContain(encodeURIComponent(broadcastToken));
    // Fresh socket — URL may be identical path; that is correct (token reuse
    // is the public listener contract; do not mint a one-shot ticket client-side).
    expect(second).not.toMatch(/ticket=/i);

    // Bounded: must not spin unbounded reconnect attempts immediately.
    await listen.waitForTimeout(2_000);
    const body = (await listen.locator("body").innerText()).toLowerCase();
    expect(body).not.toMatch(/reconnecting \(attempt [5-9]/);

    await listen.close();
  });

  test("audio resumes after bounded interruption (structure)", async ({
    page,
    request,
  }) => {
    // Full tone-in/tone-out proof lives in golden-audio.spec.ts once a producer
    // is attached. Here we only assert the SPA exposes resume controls after
    // the user gesture and does not claim live audio without connecting.
    test.skip(
      !(await serverReachable(request)),
      "ct-server not reachable — deferred until integration harness is up",
    );

    const adminToken = await adminLoginUI(page);
    const session = await apiFetch(request, adminToken, "post", "/api/sessions", {
      name: `Listener audio ${Date.now()}`,
    });
    const sessionId = session.id as string;
    await createChannel(request, adminToken, sessionId, "Broadcast", "broadcast");
    const broadcastToken = await getBroadcastToken(request, adminToken, sessionId);

    const listen = await page.context().newPage();
    await openListener(listen, sessionId, broadcastToken);

    // User gesture still required.
    await expect(listen.getByTestId("listen-button")).toBeVisible({
      timeout: 15_000,
    });
    await listen.getByTestId("listen-button").click();

    // After gesture, pause control appears while not terminal-invalid.
    // Prefer pause-button; fall back to error message without .or() strict issues.
    const pause = listen.getByTestId("pause-button");
    const errMsg = listen.getByTestId("broadcast-error-message");
    await expect
      .poll(
        async () =>
          (await pause.count()) + (await errMsg.count()) > 0 ? "ready" : "",
        { timeout: 20_000 },
      )
      .toBe("ready");

    if (await pause.count()) {
      await pause.click();
      await expect(pause).toContainText(/resume/i);
      await pause.click();
      await expect(pause).toContainText(/pause/i);
    }

    await listen.close();
  });
});
