/**
 * CrossTalk Playwright Integration Tests — Navigation & Layout
 *
 * Exercises the top nav bar, active link highlighting, logout flow,
 * and auth guard redirects.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI } from "../helpers";

test.describe("Navigation and layout", () => {
  test.beforeEach(async ({ request, page }) => {
    await resetServer(request);
    await loginViaUI(page);
  });

  test("should display nav bar with all links", async ({ page }) => {
    await page.goto("/dashboard");

    await expect(page.locator("header")).toBeVisible();
    await expect(page.locator("text=CrossTalk").first()).toBeVisible();
    await expect(page.locator('nav >> text=Dashboard')).toBeVisible();
    await expect(page.locator('nav >> text=Templates')).toBeVisible();
    await expect(page.locator('nav >> text=Sessions')).toBeVisible();
  });

  test("should navigate between all top-level views via nav links", async ({
    page,
  }) => {
    await page.goto("/dashboard");

    await page.click('nav >> text=Templates');
    await expect(page).toHaveURL(/\/templates/);
    await expect(page.locator("h1")).toContainText("Templates");

    await page.click('nav >> text=Sessions');
    await expect(page).toHaveURL(/\/sessions/);
    await expect(page.locator("h1")).toContainText("Sessions");

    await page.click('nav >> text=Dashboard');
    await expect(page).toHaveURL(/\/dashboard/);
    await expect(page.locator("h1")).toContainText("Dashboard");
  });

  test("should show logged-in username in header", async ({ page }) => {
    await page.goto("/dashboard");
    await expect(page.locator("header >> text=admin")).toBeVisible();
  });

  test("should logout and redirect to login", async ({ page }) => {
    await page.goto("/dashboard");

    await page.click('[data-testid="logout-button"]');
    await expect(page).toHaveURL(/\/login/, { timeout: 10000 });
  });

  test("should redirect unauthenticated users to login", async ({
    page,
    context,
  }) => {
    await context.clearCookies();
    await page.evaluate(() => sessionStorage.clear());

    await page.goto("/dashboard");
    await expect(page).toHaveURL(/\/login/, { timeout: 10000 });
  });

  test("should redirect unknown routes to dashboard", async ({ page }) => {
    await page.goto("/nonexistent-route");
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10000 });
  });

  test("should highlight active nav link", async ({ page }) => {
    await page.goto("/templates");
    await expect(page.locator("h1")).toContainText("Templates", { timeout: 10000 });

    const templatesLink = page.locator('nav >> text=Templates');
    await expect(templatesLink).toHaveClass(/bg-accent/, { timeout: 5000 });

    const dashboardLink = page.locator('nav >> text=Dashboard');
    await expect(dashboardLink).not.toHaveClass(/bg-accent/);
  });
});
