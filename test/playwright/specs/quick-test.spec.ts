/**
 * CrossTalk Playwright Integration Tests — Quick Test Flow
 *
 * Tests the dashboard quick-test button that creates a session
 * from the default template and redirects to the connect view.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI, createTemplateViaAPI } from "../helpers";

test.describe("Quick test flow", () => {
  test("should create session from default template and redirect to connect view", async ({
    page,
    request,
  }) => {
    const apiToken = await resetServer(request);
    await loginViaUI(page);

    // Create a default template via API.
    await createTemplateViaAPI(request, apiToken, {
      name: "Default Template",
      is_default: true,
    });

    // Navigate to dashboard.
    await page.goto("/dashboard");
    await expect(page.locator("h1")).toContainText("Dashboard");

    // The quick-test button should be enabled (we have a default template).
    const quickTestBtn = page.locator('[data-testid="quick-test-button"]');
    await expect(quickTestBtn).toBeEnabled({ timeout: 10000 });

    // Click the quick-test button.
    await quickTestBtn.click();

    // Should redirect to a connect view (URL pattern: /sessions/:id/connect).
    await expect(page).toHaveURL(/\/sessions\/[^/]+\/connect/, {
      timeout: 15000,
    });

    // Verify we're on the connect page with session info.
    await expect(page.locator('[data-testid="mic-section"]')).toBeVisible({
      timeout: 10000,
    });
  });
});
