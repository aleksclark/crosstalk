/**
 * CrossTalk Playwright Integration Tests — Session Connect Page (Extended)
 *
 * Exercises the connect view beyond audio controls: session logs panel,
 * log filter, WebRTC debug panel, volume controls section, mute button,
 * end session from connect view, and session info bar.
 */
import { test, expect } from "@playwright/test";
import {
  resetServer,
  loginViaUI,
  createTemplateViaAPI,
  createSessionViaAPI,
} from "../helpers";

test.describe("Session connect page", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  async function createSessionAndNavigate(
    page: import("@playwright/test").Page,
    request: import("@playwright/test").APIRequestContext,
    token: string,
    role: string = "translator",
  ) {
    const template = await createTemplateViaAPI(request, token);
    const session = await createSessionViaAPI(
      request,
      token,
      template.id as string,
      "Connect Test",
    );
    await page.goto(`/sessions/${session.id}/connect?role=${role}`);
    return session;
  }

  test("should display session info bar with name and role", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator("text=Session: Connect Test")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator("text=Role: translator")).toBeVisible();
  });

  test("should show ICE state badge", async ({ page, request }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator("text=ICE:")).toBeVisible({ timeout: 10000 });
  });

  test("should display WebRTC debug panel with all stats", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    const debug = page.locator('[data-testid="webrtc-debug"]');
    await expect(debug).toBeVisible({ timeout: 10000 });

    await expect(debug.locator("text=ICE State")).toBeVisible();
    await expect(debug.locator("text=ICE Candidates")).toBeVisible();
    await expect(debug.locator("text=Bytes Sent")).toBeVisible();
    await expect(debug.locator("text=Bytes Received")).toBeVisible();
    await expect(debug.locator("text=Packet Loss")).toBeVisible();
    await expect(debug.locator("text=Jitter")).toBeVisible();
    await expect(debug.locator("text=RTT")).toBeVisible();
  });

  test("should display microphone section with device selector", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator('[data-testid="mic-section"]')).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator('[data-testid="mic-device-select"]')).toBeVisible();
    await expect(page.locator('[data-testid="mic-vu-meter"]')).toBeVisible();
  });

  test("should have mute button that toggles state", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    const muteBtn = page.locator('[data-testid="mic-mute-button"]');
    await expect(muteBtn).toBeVisible({ timeout: 10000 });
    await expect(muteBtn).toHaveText("Mute");

    await muteBtn.click();
    await expect(muteBtn).toHaveText("Unmute");

    await muteBtn.click();
    await expect(muteBtn).toHaveText("Mute");
  });

  test("should display Session Peers card", async ({ page, request }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator("text=Session Peers").first()).toBeVisible({
      timeout: 10000,
    });
  });

  test("should display incoming channels section", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(
      page.locator('[data-testid="incoming-channels"]'),
    ).toBeVisible({ timeout: 10000 });
    await expect(page.locator("text=Incoming Channels")).toBeVisible();
  });

  test("should display volume controls section", async ({ page, request }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(
      page.locator('[data-testid="volume-controls"]'),
    ).toBeVisible({ timeout: 10000 });
    await expect(page.locator("text=Volume Controls")).toBeVisible();
  });

  test("should display session logs panel with filter", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator("text=Session Logs")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator('[data-testid="session-logs"]')).toBeVisible();
    await expect(page.locator('[data-testid="log-filter"]')).toBeVisible();
  });

  test("should allow changing log filter", async ({ page, request }) => {
    await createSessionAndNavigate(page, request, apiToken);

    const filter = page.locator('[data-testid="log-filter"]');
    await expect(filter).toBeVisible({ timeout: 10000 });

    await filter.selectOption("debug");
    await expect(filter).toHaveValue("debug");

    await filter.selectOption("error");
    await expect(filter).toHaveValue("error");

    await filter.selectOption("all");
    await expect(filter).toHaveValue("all");
  });

  test("should end session from connect view and redirect", async ({
    page,
    request,
  }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator('[data-testid="end-session-button"]')).toBeVisible({
      timeout: 10000,
    });

    page.on("dialog", (dialog) => dialog.accept());
    await page.click('[data-testid="end-session-button"]');

    await expect(page).toHaveURL(/\/sessions$/, { timeout: 15000 });
  });

  test("should display Audio Channels card", async ({ page, request }) => {
    await createSessionAndNavigate(page, request, apiToken);

    await expect(page.locator("text=Audio Channels")).toBeVisible({
      timeout: 10000,
    });
  });
});
