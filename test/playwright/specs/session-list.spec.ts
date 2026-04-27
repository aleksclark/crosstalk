/**
 * CrossTalk Playwright Integration Tests — Session List Page (Extended)
 *
 * Extends the existing sessions.spec.ts with coverage for end session,
 * session link navigation, empty states, and multi-session display.
 */
import { test, expect } from "@playwright/test";
import {
  resetServer,
  loginViaUI,
  createTemplateViaAPI,
  createSessionViaAPI,
} from "../helpers";

test.describe("Session list page", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("should show empty state when no sessions exist", async ({ page }) => {
    await page.goto("/sessions");
    await expect(page.locator("h1")).toContainText("Sessions");
    await expect(page.locator("text=No sessions")).toBeVisible();
  });

  test("should list multiple sessions with correct columns", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken, {
      name: "Multi Template",
    });
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Session Alpha",
    );
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Session Beta",
    );

    await page.goto("/sessions");
    await expect(page.locator('[data-testid="session-row"]')).toHaveCount(2, {
      timeout: 10000,
    });
    await expect(page.locator("text=Session Alpha")).toBeVisible();
    await expect(page.locator("text=Session Beta")).toBeVisible();
    await expect(page.locator("text=Multi Template").first()).toBeVisible();
  });

  test("should navigate to session detail via session name link", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Linked Session",
    );

    await page.goto("/sessions");
    await expect(page.locator("text=Linked Session")).toBeVisible({
      timeout: 10000,
    });

    await page.click("text=Linked Session");
    await expect(page).toHaveURL(
      new RegExp(`/sessions/${session.id}$`),
      { timeout: 10000 },
    );
  });

  test("should end a session from the session list", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Endable Session",
    );

    await page.goto("/sessions");
    await expect(page.locator("text=Endable Session")).toBeVisible({
      timeout: 10000,
    });

    page.on("dialog", (dialog) => dialog.accept());
    await page.click('[data-testid="end-session-button"]');

    await expect(page.locator("text=ended")).toBeVisible({ timeout: 10000 });
  });

  test("should navigate to connect view via Connect button", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Connect Session",
    );

    await page.goto("/sessions");
    await expect(page.locator("text=Connect Session")).toBeVisible({
      timeout: 10000,
    });

    await page.locator('[data-testid="session-row"] button', { hasText: 'Connect' }).click();
    await expect(page).toHaveURL(
      new RegExp(`/sessions/${session.id}/connect`),
      { timeout: 10000 },
    );
  });

  test("should show client count and total roles in session row", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Count Session",
    );

    await page.goto("/sessions");
    await expect(page.locator("text=0 / 2")).toBeVisible({ timeout: 10000 });
  });

  test("should toggle create session form open and closed", async ({
    page,
    request,
  }) => {
    await createTemplateViaAPI(request, apiToken, { name: "Toggle Template" });

    await page.goto("/sessions");

    const createBtn = page.locator('[data-testid="create-session-button"]');
    await createBtn.click();
    await expect(
      page.locator('[data-testid="session-name-input"]'),
    ).toBeVisible();

    await createBtn.click();
    await expect(
      page.locator('[data-testid="session-name-input"]'),
    ).not.toBeVisible();
  });

  test("should show status badges with correct variants", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Badge Session",
    );

    await page.goto("/sessions");
    const badge = page
      .locator('[data-testid="session-row"]')
      .locator("text=waiting");
    await expect(badge).toBeVisible({ timeout: 10000 });
  });
});
