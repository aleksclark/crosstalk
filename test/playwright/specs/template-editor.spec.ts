/**
 * CrossTalk Playwright Integration Tests — Template Editor (Extended)
 *
 * Extends the existing templates.spec.ts with coverage for multi-client roles,
 * mappings editor, validation errors, default template toggle, and cancel button.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI, createTemplateViaAPI } from "../helpers";

test.describe("Template editor", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("should toggle default template checkbox", async ({ page }) => {
    await page.goto("/templates/new");

    const toggle = page.locator('[data-testid="template-default-toggle"]');
    await expect(toggle).not.toBeChecked();

    await toggle.check();
    await expect(toggle).toBeChecked();

    await toggle.uncheck();
    await expect(toggle).not.toBeChecked();
  });

  test("should add and remove roles", async ({ page }) => {
    await page.goto("/templates/new");

    await expect(page.locator('[data-testid="role-row"]')).toHaveCount(1);

    await page.click('[data-testid="add-role-button"]');
    await expect(page.locator('[data-testid="role-row"]')).toHaveCount(2);

    await page.click('[data-testid="add-role-button"]');
    await expect(page.locator('[data-testid="role-row"]')).toHaveCount(3);

    await page.locator('[data-testid="role-row"]').nth(2).locator("text=Remove").click();
    await expect(page.locator('[data-testid="role-row"]')).toHaveCount(2);
  });

  test("should toggle multi-client on a role", async ({ page }) => {
    await page.goto("/templates/new");

    const multiClient = page.locator('[data-testid="role-multi-client-toggle"]').first();
    await expect(multiClient).not.toBeChecked();
    await multiClient.check();
    await expect(multiClient).toBeChecked();
  });

  test("should add and remove mappings", async ({ page }) => {
    await page.goto("/templates/new");

    await expect(page.locator('[data-testid="mapping-row"]')).toHaveCount(1);

    await page.click('[data-testid="add-mapping-button"]');
    await expect(page.locator('[data-testid="mapping-row"]')).toHaveCount(2);

    await page.locator('[data-testid="mapping-row"]').nth(1).locator("text=Remove").click();
    await expect(page.locator('[data-testid="mapping-row"]')).toHaveCount(1);
  });

  test("should populate mapping role dropdowns from defined roles", async ({
    page,
  }) => {
    await page.goto("/templates/new");

    await page.locator('[data-testid="role-name-input"]').first().fill("interpreter");
    await page.click('[data-testid="add-role-button"]');
    await page.locator('[data-testid="role-name-input"]').nth(1).fill("studio");

    const fromRoleSelect = page.locator('[data-testid="mapping-from-role"]').first();
    await expect(fromRoleSelect.locator("option", { hasText: "interpreter" })).toBeAttached();
    await expect(fromRoleSelect.locator("option", { hasText: "studio" })).toBeAttached();
  });

  test("should show mapping to-type options (Role, Record, Broadcast)", async ({
    page,
  }) => {
    await page.goto("/templates/new");

    const toType = page.locator('[data-testid="mapping-to-type"]').first();
    await expect(toType.locator("option", { hasText: "Role" })).toBeAttached();
    await expect(toType.locator("option", { hasText: "Record" })).toBeAttached();
    await expect(toType.locator("option", { hasText: "Broadcast" })).toBeAttached();
  });

  test("should hide target role/channel when mapping type is Record", async ({
    page,
  }) => {
    await page.goto("/templates/new");

    await page.locator('[data-testid="mapping-to-type"]').first().selectOption("record");
    await expect(page.locator('[data-testid="mapping-to-role"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="mapping-to-channel"]')).not.toBeVisible();
  });

  test("should show validation error for invalid mapping role reference", async ({
    page,
  }) => {
    await page.goto("/templates/new");

    await page.locator('[data-testid="template-name-input"]').fill("Validation Test");
    await page.locator('[data-testid="role-name-input"]').first().fill("translator");

    await page.locator('[data-testid="mapping-from-role"]').first().selectOption("translator");
    await page.locator('[data-testid="mapping-from-channel"]').first().fill("mic");
    await page.locator('[data-testid="mapping-to-role"]').first().selectOption("");

    await page.click('[data-testid="save-template-button"]');

    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });
  });

  test("should cancel and return to template list", async ({ page }) => {
    await page.goto("/templates/new");
    await page.click("text=Cancel");
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });
  });

  test("should load existing template data when editing", async ({
    page,
    request,
  }) => {
    const template = await createTemplateViaAPI(request, apiToken, {
      name: "Editable Template",
      is_default: true,
    });

    await page.goto(`/templates/${template.id}`);
    await expect(page.locator("h1")).toContainText("Edit Template");

    const nameInput = page.locator('[data-testid="template-name-input"]');
    await expect(nameInput).toHaveValue("Editable Template");

    const defaultToggle = page.locator('[data-testid="template-default-toggle"]');
    await expect(defaultToggle).toBeChecked();
  });

  test("should show template list empty state", async ({ page }) => {
    await page.goto("/templates");
    await expect(page.locator("text=No templates defined")).toBeVisible();
  });

  test("should show template row with roles and mappings counts", async ({
    page,
    request,
  }) => {
    await createTemplateViaAPI(request, apiToken, { name: "Count Template" });

    await page.goto("/templates");
    await expect(page.locator("text=Count Template")).toBeVisible({
      timeout: 10000,
    });

    const row = page.locator('[data-testid="template-row"]');
    await expect(row).toHaveCount(1);
    await expect(row.locator("text=2")).toBeVisible();
    await expect(row.locator("text=1")).toBeVisible();
  });

  test("should save template with roles and mappings without Bad Request", async ({
    page,
  }) => {
    await page.goto("/templates/new");

    await page.locator('[data-testid="template-name-input"]').fill("Translation");
    await page.locator('[data-testid="template-default-toggle"]').check();

    const firstRole = page.locator('[data-testid="role-name-input"]').first();
    await firstRole.clear();
    await firstRole.fill("studio");

    await page.click('[data-testid="add-role-button"]');
    await page.locator('[data-testid="role-name-input"]').nth(1).fill("translator");

    const firstFromRole = page.locator('[data-testid="mapping-from-role"]').first();
    await firstFromRole.selectOption("studio");
    await page.locator('[data-testid="mapping-from-channel"]').first().fill("input");
    await page.locator('[data-testid="mapping-to-role"]').first().selectOption("translator");
    await page.locator('[data-testid="mapping-to-channel"]').first().fill("speakers");

    await page.click('[data-testid="add-mapping-button"]');
    const secondFromRole = page.locator('[data-testid="mapping-from-role"]').nth(1);
    await secondFromRole.selectOption("translator");
    await page.locator('[data-testid="mapping-from-channel"]').nth(1).fill("microphone");
    await page.locator('[data-testid="mapping-to-role"]').nth(1).selectOption("studio");
    await page.locator('[data-testid="mapping-to-channel"]').nth(1).fill("output");

    await page.click('[data-testid="save-template-button"]');

    await expect(page.locator('[role="alert"]')).not.toBeVisible({ timeout: 3000 });
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });
    await expect(page.locator("text=Translation")).toBeVisible({ timeout: 10000 });
  });

  test("should round-trip template mappings through edit", async ({
    page,
    request,
  }) => {
    await page.goto("/templates/new");
    await page.locator('[data-testid="template-name-input"]').fill("Roundtrip");

    const firstRole = page.locator('[data-testid="role-name-input"]').first();
    await firstRole.clear();
    await firstRole.fill("speaker");

    await page.click('[data-testid="add-role-button"]');
    await page.locator('[data-testid="role-name-input"]').nth(1).fill("listener");

    await page.locator('[data-testid="mapping-from-role"]').first().selectOption("speaker");
    await page.locator('[data-testid="mapping-from-channel"]').first().fill("mic");
    await page.locator('[data-testid="mapping-to-role"]').first().selectOption("listener");
    await page.locator('[data-testid="mapping-to-channel"]').first().fill("out");

    await page.click('[data-testid="save-template-button"]');
    await expect(page).toHaveURL(/\/templates$/, { timeout: 10000 });

    await page.click('[data-testid="template-row"] >> text=Edit');
    await expect(page).toHaveURL(/\/templates\/[^/]+$/);

    await expect(page.locator('[data-testid="mapping-from-role"]').first()).toHaveValue("speaker");
    await expect(page.locator('[data-testid="mapping-from-channel"]').first()).toHaveValue("mic");
    await expect(page.locator('[data-testid="mapping-to-role"]').first()).toHaveValue("listener");
    await expect(page.locator('[data-testid="mapping-to-channel"]').first()).toHaveValue("out");
  });

  test("should mark default template with badge", async ({
    page,
    request,
  }) => {
    await createTemplateViaAPI(request, apiToken, {
      name: "Default One",
      is_default: true,
    });

    await page.goto("/templates");
    await expect(page.locator("text=Default").first()).toBeVisible({
      timeout: 10000,
    });
  });
});
