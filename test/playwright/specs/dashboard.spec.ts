/**
 * CrossTalk Playwright Integration Tests — Dashboard Page
 *
 * Exercises the dashboard view: stat cards, recent sessions table,
 * navigation links, empty states, and quick-test disabled state.
 */
import { test, expect } from "@playwright/test";
import {
  resetServer,
  loginViaUI,
  createTemplateViaAPI,
  createSessionViaAPI,
} from "../helpers";

test.describe("Dashboard page", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("should display stat cards on empty server", async ({ page }) => {
    await page.goto("/dashboard");
    await expect(page.locator("h1")).toContainText("Dashboard");

    await expect(page.locator('[data-testid="active-sessions-count"]')).toHaveText("0");
    await expect(page.locator('[data-testid="connected-clients-count"]')).toHaveText("0");
    await expect(page.locator('[data-testid="server-uptime"]')).toBeVisible();
    await expect(page.locator('[data-testid="server-version"]')).toBeVisible();
  });

  test("should show 'No sessions yet' when no sessions exist", async ({
    page,
  }) => {
    await page.goto("/dashboard");
    await expect(page.locator("text=No sessions yet")).toBeVisible();
  });

  test("should show recent sessions table when sessions exist", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Dashboard Session",
    );

    await page.goto("/dashboard");
    await expect(page.locator("text=Dashboard Session")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator('[data-testid="active-sessions-count"]')).toHaveText("1");
  });

  test("should disable quick-test button when no default template exists", async ({
    page,
  }) => {
    await page.goto("/dashboard");
    const btn = page.locator('[data-testid="quick-test-button"]');
    await expect(btn).toBeDisabled();
  });

  test("should navigate to session detail via View button", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken);
    await createSessionViaAPI(
      request,
      apiToken,
      template.id as string,
      "Viewable Session",
    );

    await page.goto("/dashboard");
    await expect(page.locator("text=Viewable Session")).toBeVisible({
      timeout: 10000,
    });

    await page.click("text=View");
    await expect(page).toHaveURL(/\/sessions\/[^/]+$/, { timeout: 10000 });
  });

  test("should show template count in stat cards", async ({
    page,
    request,
  }) => {
    await createTemplateViaAPI(request, apiToken, { name: "Tmpl A" });
    await createTemplateViaAPI(request, apiToken, { name: "Tmpl B" });

    await page.goto("/dashboard");
    await expect(page.locator("text=Templates").first()).toBeVisible();
    await expect(page.locator("text=2").first()).toBeVisible({ timeout: 10000 });
  });
});
