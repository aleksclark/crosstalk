/**
 * CrossTalk Playwright Integration Tests — Session Detail Page
 *
 * Exercises the session detail view: status badge, connected clients table,
 * channel bindings card, end session button, connect button navigation.
 */
import { test, expect } from "@playwright/test";
import {
  resetServer,
  loginViaUI,
  createTemplateViaAPI,
  createSessionViaAPI,
} from "../helpers";

const BASE_URL = process.env.CT_SERVER_URL || "http://localhost:8080";

test.describe("Session detail page", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("should show session name and status", async ({ page, request }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Detail Test Session",
    );

    await page.goto(`/sessions/${session.id}`);

    await expect(page.locator("h1")).toContainText("Detail Test Session");
    await expect(page.locator("text=waiting")).toBeVisible();
    await expect(page.locator("text=0 / 2 clients connected")).toBeVisible();
  });

  test("should show template name and creation date", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken, {
      name: "DetailTemplate",
    });
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("text=DetailTemplate")).toBeVisible();
  });

  test("should show Connected Clients card with empty state", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("text=Connected Clients")).toBeVisible();
    await expect(page.locator("text=No clients connected")).toBeVisible();
  });

  test("should show Channel Bindings card with empty state", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("h1")).toContainText("Test Session", { timeout: 10_000 });
    await expect(page.locator("text=Channel Bindings")).toBeVisible({ timeout: 10_000 });
    await expect(page.locator("text=No channel bindings")).toBeVisible();
  });

  test("should show role selector populated from template roles", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);

    const roleSelect = page.locator('[data-testid="connect-role-select"]');
    await expect(roleSelect).toBeVisible({ timeout: 10000 });

    await expect(roleSelect.locator("option")).toHaveCount(2);
    await expect(roleSelect.locator("option", { hasText: "translator" })).toBeAttached();
    await expect(roleSelect.locator("option", { hasText: "studio" })).toBeAttached();
  });

  test("should navigate to connect view with selected role", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);

    const roleSelect = page.locator('[data-testid="connect-role-select"]');
    await expect(roleSelect).toBeVisible({ timeout: 10000 });
    await roleSelect.selectOption("studio");

    await page.click('[data-testid="connect-button"]');
    await expect(page).toHaveURL(
      new RegExp(`/sessions/${session.id}/connect\\?role=studio`),
      { timeout: 10000 },
    );
  });

  test("should default-select first role and navigate to connect view", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);

    const roleSelect = page.locator('[data-testid="connect-role-select"]');
    await expect(roleSelect).toBeVisible({ timeout: 10000 });

    await page.click('[data-testid="connect-button"]');
    await expect(page).toHaveURL(
      new RegExp(`/sessions/${session.id}/connect\\?role=`),
      { timeout: 10000 },
    );
  });

  test("should end session and update status", async ({ page, request }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);

    page.on("dialog", (dialog) => dialog.accept());
    await page.click('[data-testid="end-session-button"]');

    await expect(page.locator("text=ended")).toBeVisible({ timeout: 10000 });
  });

  test("should hide End/Connect buttons for ended session", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await request.delete(`${BASE_URL}/api/sessions/${session.id}`, {
      headers: { Authorization: `Bearer ${apiToken}` },
    });

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("text=ended")).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="end-session-button"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="connect-button"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="connect-role-select"]')).not.toBeVisible();
  });

  test("should show Assign Peers card", async ({ page, request }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    const session = await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
    );

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("text=Assign Peers")).toBeVisible();
  });
});
