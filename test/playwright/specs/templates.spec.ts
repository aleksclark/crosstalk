/**
 * CrossTalk Playwright Integration Tests — Template CRUD
 *
 * Tests template management through the web UI.
 * Each test resets the server and logs in fresh.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI } from "../helpers";

test.describe("Template CRUD", () => {
  test.beforeEach(async ({ request, page }) => {
    await resetServer(request);
    await loginViaUI(page);
  });

  test("should create a new template via the UI", async ({ page }) => {
    // Navigate to templates page.
    await page.goto("/templates");
    await expect(page.locator("h1")).toContainText("Templates");

    // Click create button.
    await page.click('[data-testid="create-template-button"]');
    await expect(page).toHaveURL(/\/templates\/new/);

    // Fill in template name.
    await page.fill('[data-testid="template-name-input"]', "My Test Template");

    // Fill in a role name.
    await page.fill(
      '[data-testid="role-name-input"] >> nth=0',
      "translator",
    );

    // Save the template.
    await page.click('[data-testid="save-template-button"]');

    // Should redirect to template list.
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });

    // Verify the template appears in the list.
    await expect(page.locator('[data-testid="template-row"]')).toHaveCount(1);
    await expect(page.locator("text=My Test Template")).toBeVisible();
  });

  test("should edit an existing template", async ({ page }) => {
    // First create a template via UI.
    await page.goto("/templates/new");
    await page.fill('[data-testid="template-name-input"]', "Original Name");
    await page.fill(
      '[data-testid="role-name-input"] >> nth=0',
      "speaker",
    );
    await page.click('[data-testid="save-template-button"]');
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });

    // Click Edit on the template.
    await page.click('[data-testid="template-row"] >> text=Edit');

    // Should navigate to the editor.
    await expect(page).toHaveURL(/\/templates\/[^/]+$/);

    // Change the template name.
    const nameInput = page.locator('[data-testid="template-name-input"]');
    await nameInput.clear();
    await nameInput.fill("Updated Name");

    // Save.
    await page.click('[data-testid="save-template-button"]');
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });

    // Verify the updated name appears.
    await expect(page.locator("text=Updated Name")).toBeVisible();
    await expect(page.locator("text=Original Name")).not.toBeVisible();
  });

  test("should delete a template", async ({ page }) => {
    // Create a template first.
    await page.goto("/templates/new");
    await page.fill('[data-testid="template-name-input"]', "To Be Deleted");
    await page.fill(
      '[data-testid="role-name-input"] >> nth=0',
      "speaker",
    );
    await page.click('[data-testid="save-template-button"]');
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });
    await expect(page.locator("text=To Be Deleted")).toBeVisible();

    // Click Delete on the template. Accept the confirm dialog.
    page.on("dialog", (dialog) => dialog.accept());
    await page.click('[data-testid="template-row"] >> text=Delete');

    // Verify the template is removed from the list.
    await expect(page.locator("text=To Be Deleted")).not.toBeVisible({ timeout: 10000 });
    await expect(page.locator("text=No templates defined")).toBeVisible({ timeout: 10000 });
  });
});
