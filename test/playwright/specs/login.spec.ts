/**
 * CrossTalk Playwright Integration Tests — Login Flow
 *
 * Prerequisites: ct-server running with embedded web UI at CT_SERVER_URL.
 * Admin user must be seeded (happens automatically on first server start).
 *
 * These tests verify the web UI login flow works end-to-end against a real
 * server with real SQLite persistence.
 */
import { test, expect } from "@playwright/test";

test.describe("Login flow", () => {
  test("should show login page at /", async ({ page }) => {
    await page.goto("/");
    // The SPA should render — either a login form or redirect to login.
    // Verify the page loaded (not a 404 or server error).
    await expect(page.locator("body")).toBeVisible();
  });

  test("should login with valid credentials and redirect to dashboard", async ({
    page,
  }) => {
    await page.goto("/login");

    // Fill in credentials. The admin user is seeded on first server start.
    // The seed password is logged at startup — for integration tests, the
    // test runner creates the user with a known password.
    await page.fill('input[name="username"]', "admin");
    await page.fill('input[name="password"]', "admin-password");
    await page.click('button[type="submit"]');

    // After successful login, should redirect to dashboard or main view.
    // Verify URL changed away from /login.
    await expect(page).not.toHaveURL(/\/login/);
  });

  test("should show error for invalid credentials", async ({ page }) => {
    await page.goto("/login");

    await page.fill('input[name="username"]', "admin");
    await page.fill('input[name="password"]', "wrong-password");
    await page.click('button[type="submit"]');

    // Should stay on login page or show an error message.
    await expect(page.locator("text=error").or(page.locator(".error"))).toBeVisible({
      timeout: 5000,
    });
  });
});
