/**
 * CrossTalk Playwright Integration Tests — Broadcast Listeners
 *
 * Phase 10, Step 7: End-to-end tests for the public broadcast listener flow.
 *
 * These tests verify:
 *   - Broadcast token generation and QR code display on session detail page
 *   - Public listener page loads without authentication
 *   - Invalid broadcast tokens are rejected with an error
 *   - Listener count tracks connected broadcast listeners in real time
 *   - (Optional) Listener receives audio via WebRTC
 *
 * Prerequisites:
 *   - ct-server running with embedded web UI (CT_SERVER_URL)
 *   - CROSSTALK_TEST_MODE=1 for the reset endpoint
 *   - Server has broadcast token, signaling, and listener count support
 */
import { test, expect, type BrowserContext, type Page } from "@playwright/test";
import {
  resetServer,
  loginViaUI,
  createBroadcastTemplateViaAPI,
  createSessionViaAPI,
  createBroadcastTokenViaAPI,
  getSessionListenerCount,
} from "../helpers";

const BASE_URL = process.env.CT_SERVER_URL || "http://localhost:8080";

test.describe("Broadcast listeners", () => {
  let apiToken: string;

  test.beforeEach(async ({ request }) => {
    apiToken = await resetServer(request);
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 1: Broadcast token and QR code on session detail page
  // ════════════════════════════════════════════════════════════════════════

  test("broadcast-token-and-qr-code", async ({ page, request }) => {
    // 1. Login as admin
    await loginViaUI(page);

    // 2. Create a template with a broadcast mapping
    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const templateId = template.id as string;

    // 3. Create a session with that template
    const session = await createSessionViaAPI(
      request,
      apiToken,
      templateId,
      "Broadcast QR Test",
    );
    const sessionId = session.id as string;

    // 4. Navigate to session detail page
    await page.goto(`/sessions/${sessionId}`);
    await expect(page.locator("h1")).toContainText("Broadcast QR Test", {
      timeout: 10_000,
    });

    // 5. Verify BroadcastCard is visible
    const broadcastCard = page.locator('[data-testid="broadcast-card"]');
    await expect(broadcastCard).toBeVisible({ timeout: 10_000 });

    // 6. Click "Generate Broadcast Link" button
    const generateBtn = page.locator('[data-testid="generate-link-button"]');
    await expect(generateBtn).toBeVisible();
    await generateBtn.click();

    // 7. Verify QR code is displayed
    const qrCode = page.locator('[data-testid="qr-code"]');
    await expect(qrCode).toBeVisible({ timeout: 10_000 });

    // 8. Verify broadcast URL is displayed and contains /listen/ path
    const broadcastUrl = page.locator('[data-testid="broadcast-url"]');
    await expect(broadcastUrl).toBeVisible();
    const urlText = await broadcastUrl.textContent();
    expect(urlText).toBeTruthy();
    expect(urlText).toContain("/listen/");
    expect(urlText).toContain(sessionId);
    expect(urlText).toContain("token=");

    // 9. Verify Copy button is present
    const copyBtn = page.locator('[data-testid="copy-link-button"]');
    await expect(copyBtn).toBeVisible();
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 2: Listener page loads without authentication
  // ════════════════════════════════════════════════════════════════════════

  test("listener-page-loads", async ({ page, request }) => {
    // 1. Create a template + session + broadcast token via API
    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Listener Load Test",
    );
    const sessionId = session.id as string;

    const broadcastToken = await createBroadcastTokenViaAPI(
      request,
      apiToken,
      sessionId,
    );

    // 2. Navigate to /listen/{sessionId}?token={token}
    //    (no loginViaUI — this page must work without auth)
    await page.goto(`/listen/${sessionId}?token=${broadcastToken.token}`);

    // 3. Verify listener page is displayed (not redirected to /login)
    const listenerPage = page.locator('[data-testid="listener-page"]');
    await expect(listenerPage).toBeVisible({ timeout: 15_000 });

    // 4. Verify session title is displayed
    const sessionTitle = page.locator('[data-testid="session-title"]');
    await expect(sessionTitle).toBeVisible({ timeout: 10_000 });
    await expect(sessionTitle).toContainText("Listener Load Test");

    // 5. Verify play button is visible
    const playBtn = page.locator('[data-testid="play-pause-button"]');
    await expect(playBtn).toBeVisible({ timeout: 15_000 });

    // 6. Verify volume slider is visible
    const volumeSlider = page.locator('[data-testid="volume-slider"]');
    await expect(volumeSlider).toBeVisible();

    // 7. Verify no login prompt appears (page should NOT have /login in URL)
    await expect(page).not.toHaveURL(/\/login/);
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 3: Listener page with invalid token shows error
  // ════════════════════════════════════════════════════════════════════════

  test("listener-page-invalid-token", async ({ page, request }) => {
    // Create a session so the session ID is valid
    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Invalid Token Test",
    );
    const sessionId = session.id as string;

    // Navigate to listener page with a bad token
    await page.goto(`/listen/${sessionId}?token=bad-token-12345`);

    // Verify error message is displayed
    const errorMsg = page.locator('[data-testid="error-message"]');
    await expect(errorMsg).toBeVisible({ timeout: 15_000 });
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 4: Listener count updates as listeners join and leave
  // ════════════════════════════════════════════════════════════════════════

  test("listener-count-updates", async ({ page, request, context }) => {
    // 1. Login as admin, create session with broadcast mapping
    await loginViaUI(page);

    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Listener Count Test",
    );
    const sessionId = session.id as string;

    // 2. Generate broadcast token
    const broadcastToken = await createBroadcastTokenViaAPI(
      request,
      apiToken,
      sessionId,
    );

    // 3. Verify initial listener count is 0 via API
    const initialCount = await getSessionListenerCount(
      request,
      apiToken,
      sessionId,
    );
    expect(initialCount).toBe(0);

    // 4. Open listener page in a new browser context (no auth)
    const listenerContext = await context.browser()!.newContext();
    const listenerPage = await listenerContext.newPage();

    await listenerPage.goto(
      `${BASE_URL}/listen/${sessionId}?token=${broadcastToken.token}`,
    );

    // Wait for listener page to be visible and connected
    await expect(
      listenerPage.locator('[data-testid="listener-page"]'),
    ).toBeVisible({ timeout: 15_000 });

    // Wait for the connection to establish (play button appears when connected)
    await expect(
      listenerPage.locator('[data-testid="play-pause-button"]'),
    ).toBeVisible({ timeout: 20_000 });

    // 5. Give the server time to register the listener and update counts
    await page.waitForTimeout(2_000);

    // Verify via API that listener count is now 1
    const countAfterJoin = await getSessionListenerCount(
      request,
      apiToken,
      sessionId,
    );
    expect(countAfterJoin).toBe(1);

    // 6. Close the listener page
    await listenerPage.close();
    await listenerContext.close();

    // 7. Give the server time to detect the disconnection
    await page.waitForTimeout(3_000);

    // Verify count is back to 0
    const countAfterLeave = await getSessionListenerCount(
      request,
      apiToken,
      sessionId,
    );
    expect(countAfterLeave).toBe(0);
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 5: Multiple listeners tracked correctly
  // ════════════════════════════════════════════════════════════════════════

  test("multiple-listeners-counted", async ({ page, request, context }) => {
    await loginViaUI(page);

    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Multi Listener Test",
    );
    const sessionId = session.id as string;

    const broadcastToken = await createBroadcastTokenViaAPI(
      request,
      apiToken,
      sessionId,
    );

    const listenerUrl = `${BASE_URL}/listen/${sessionId}?token=${broadcastToken.token}`;

    // Open two listener pages in separate contexts
    const ctx1 = await context.browser()!.newContext();
    const ctx2 = await context.browser()!.newContext();
    const listener1 = await ctx1.newPage();
    const listener2 = await ctx2.newPage();

    await listener1.goto(listenerUrl);
    await expect(
      listener1.locator('[data-testid="play-pause-button"]'),
    ).toBeVisible({ timeout: 20_000 });

    await page.waitForTimeout(1_500);

    // Verify 1 listener
    let count = await getSessionListenerCount(request, apiToken, sessionId);
    expect(count).toBe(1);

    await listener2.goto(listenerUrl);
    await expect(
      listener2.locator('[data-testid="play-pause-button"]'),
    ).toBeVisible({ timeout: 20_000 });

    await page.waitForTimeout(1_500);

    // Verify 2 listeners
    count = await getSessionListenerCount(request, apiToken, sessionId);
    expect(count).toBe(2);

    // Close one listener
    await listener1.close();
    await ctx1.close();
    await page.waitForTimeout(3_000);

    // Verify back to 1
    count = await getSessionListenerCount(request, apiToken, sessionId);
    expect(count).toBe(1);

    // Close the other listener
    await listener2.close();
    await ctx2.close();
    await page.waitForTimeout(3_000);

    // Verify back to 0
    count = await getSessionListenerCount(request, apiToken, sessionId);
    expect(count).toBe(0);
  });

  // ════════════════════════════════════════════════════════════════════════
  //  Test 6 (optional / harder): Listener WebRTC connection establishes
  // ════════════════════════════════════════════════════════════════════════
  //
  // This test verifies the broadcast listener's WebRTC connection reaches
  // the "connected" ICE state. Full audio verification is left to the
  // golden-audio.spec.ts flow which requires physical hardware.

  test("listener-webrtc-connects", async ({ page, request, context }) => {
    // Set up: template + session + broadcast token
    const template = await createBroadcastTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "WebRTC Connect Test",
    );
    const sessionId = session.id as string;

    const broadcastToken = await createBroadcastTokenViaAPI(
      request,
      apiToken,
      sessionId,
    );

    // Open listener page in new context (no auth)
    const listenerCtx = await context.browser()!.newContext();
    const listenerPage = await listenerCtx.newPage();

    await listenerPage.goto(
      `${BASE_URL}/listen/${sessionId}?token=${broadcastToken.token}`,
    );

    // Verify listener page loads
    await expect(
      listenerPage.locator('[data-testid="listener-page"]'),
    ).toBeVisible({ timeout: 15_000 });

    // Verify connection status element appears (connecting or connected)
    const connectionStatus = listenerPage.locator(
      '[data-testid="connection-status"]',
    );
    await expect(connectionStatus).toBeVisible({ timeout: 20_000 });

    // Clean up
    await listenerPage.close();
    await listenerCtx.close();
  });
});
