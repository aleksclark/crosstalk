/**
 * CrossTalk Playwright Integration Tests — Login Flow
 *
 * Prerequisites: ct-server running with embedded web UI at CT_SERVER_URL.
 * Test mode must be enabled (CROSSTALK_TEST_MODE=1) for the reset endpoint.
 *
 * These tests verify the web UI login flow works end-to-end against a real
 * server with real SQLite persistence.
 */
import { test, expect } from "@playwright/test";
import { resetServer } from "../helpers";

test.describe("Login flow", () => {
  test.beforeEach(async ({ request }) => {
    // Reset server DB and re-seed admin with known credentials.
    await resetServer(request);
  });

  test("should show login page at /", async ({ page }) => {
    await page.goto("/");
    // Unauthenticated users are redirected to /login.
    await expect(page).toHaveURL(/\/login/);
    // The login form should be visible.
    await expect(page.locator("#username")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test("should login with valid credentials and redirect to dashboard", async ({
    page,
  }) => {
    await page.goto("/login");

    // Fill in credentials — admin / admin-password (seeded by test reset).
    await page.fill("#username", "admin");
    await page.fill("#password", "admin-password");
    await page.click('button[type="submit"]');

    // After successful login, should redirect to dashboard.
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 10000 });
  });

  test("should show error for invalid credentials", async ({ page }) => {
    await page.goto("/login");

    await page.fill("#username", "admin");
    await page.fill("#password", "wrong-password");
    await page.click('button[type="submit"]');

    // Should show an error message (role="alert" div).
    await expect(page.locator('[role="alert"]')).toBeVisible({
      timeout: 5000,
    });
    // Should stay on login page.
    await expect(page).toHaveURL(/\/login/);
  });
});
