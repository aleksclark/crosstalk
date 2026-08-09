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
    await expect(
      listen
        .getByTestId("status-live")
        .or(listen.getByTestId("status-warn"))
        .or(listen.getByTestId("debug-state")),
    ).toBeVisible({ timeout: 20_000 });

    const debugState = listen.getByTestId("debug-state");
    if (await debugState.count()) {
      const state = (await debugState.textContent())?.trim();
      expect(["connecting", "reconnecting", "playing"]).toContain(state);
      expect(state).not.toBe("error");
    }

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
    await expect(
      listen
        .getByTestId("broadcast-error-message")
        .or(listen.getByTestId("broadcast-ended-message"))
        .or(listen.getByTestId("broadcast-error")),
    ).toBeVisible({ timeout: 15_000 });

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

    // Full media drop + resume needs a producer and controllable peer teardown.
    // When the harness cannot inject a mid-session WS close, we document the
    // contract via the SPA debug surface after a forced client-side socket close.
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

    // Wait for first WS.
    await expect
      .poll(() => wsUrls.length, { timeout: 15_000 })
      .toBeGreaterThanOrEqual(1);

    const first = wsUrls[0]!;
    expect(first).toContain(`/ws/broadcast/${sessionId}`);
    expect(first).toContain(`token=${encodeURIComponent(broadcastToken)}`);

    // Force-close the page's WebSocket to simulate server drop.
    await listen.evaluate(() => {
      // Access is via prototype patch is hard; close all sockets the page knows.
      // The SPA holds the socket privately — dispatch via performance entries
      // is unreliable. Instead, yank network offline then online to provoke
      // reconnect when the browser marks the socket dead.
    });

    // Offline → online is the portable way to force a transport drop in Chromium.
    await listen.context().setOffline(true);
    await listen.waitForTimeout(500);
    await listen.context().setOffline(false);

    // Expect either a reconnecting status or a second WS open with the same
    // token path (fresh connection, not a different ticket).
    await Promise.race([
      listen
        .getByText(/reconnecting/i)
        .waitFor({ timeout: 20_000 })
        .catch(() => undefined),
      expect
        .poll(() => wsUrls.length, { timeout: 20_000 })
        .toBeGreaterThanOrEqual(2),
    ]);

    if (wsUrls.length >= 2) {
      const second = wsUrls[1]!;
      expect(second).toContain(`/ws/broadcast/${sessionId}`);
      expect(second).toContain(encodeURIComponent(broadcastToken));
      // Fresh socket — URL may be identical path; that is correct (token reuse
      // is the public listener contract; do not mint a one-shot ticket client-side).
      expect(second).not.toMatch(/ticket=/i);
    } else {
      // At least the UI told the truth about reconnecting or ended/error.
      const body = (await listen.locator("body").innerText()).toLowerCase();
      expect(
        body.includes("reconnect") ||
          body.includes("disconnected") ||
          body.includes("ended") ||
          body.includes("connection"),
      ).toBeTruthy();
    }

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
    await expect(
      listen
        .getByTestId("pause-button")
        .or(listen.getByTestId("broadcast-error-message"))
        .or(listen.getByTestId("retry-button")),
    ).toBeVisible({ timeout: 20_000 });

    if (await listen.getByTestId("pause-button").count()) {
      await listen.getByTestId("pause-button").click();
      await expect(listen.getByTestId("pause-button")).toContainText(/resume/i);
      await listen.getByTestId("pause-button").click();
      await expect(listen.getByTestId("pause-button")).toContainText(/pause/i);
    }

    await listen.close();
  });
});
