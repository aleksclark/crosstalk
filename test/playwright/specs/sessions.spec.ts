/**
 * CrossTalk Playwright Integration Tests — Session Workflow
 *
 * Tests session creation, status display, and the session connect view.
 * Each test resets the server and logs in fresh.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI, createTemplateViaAPI } from "../helpers";

test.describe("Session workflow", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("should create session from template and show waiting status", async ({
    page,
    request,
  }) => {
    // Create a template via API so we have one to select.
    await createTemplateViaAPI(request, apiToken, { name: "Session Test Template" });

    // Navigate to sessions page.
    await page.goto("/sessions");
    await expect(page.locator("h1")).toContainText("Sessions");

    // Click Create Session button.
    await page.click('[data-testid="create-session-button"]');

    // Fill in session name.
    await page.fill('[data-testid="session-name-input"]', "My Test Session");

    // Select the template.
    await page.selectOption('[data-testid="session-template-select"]', {
      label: "Session Test Template",
    });

    // Click create.
    await page.click('[data-testid="confirm-create-session"]');

    // Verify session appears in the list.
    await expect(page.locator('[data-testid="session-row"]')).toHaveCount(1, {
      timeout: 10000,
    });
    await expect(page.locator("text=My Test Session")).toBeVisible();

    // Verify "waiting" status badge.
    await expect(
      page.locator('[data-testid="session-row"]').locator("text=waiting"),
    ).toBeVisible();
  });

  test("should show session connect view with audio controls", async ({
    page,
    request,
  }) => {
    // Create template + session via API.
    const template = await createTemplateViaAPI(request, apiToken, {
      name: "Connect Test Template",
    });
    const templateId = template.id as string;

    const sessionResp = await request.post(
      `${process.env.CT_SERVER_URL || "http://localhost:8080"}/api/sessions`,
      {
        data: { template_id: templateId, name: "Connect Test Session" },
        headers: { Authorization: `Bearer ${apiToken}` },
      },
    );
    expect(sessionResp.ok()).toBeTruthy();
    const session = (await sessionResp.json()) as Record<string, unknown>;
    const sessionId = session.id as string;

    // Navigate to the connect view.
    await page.goto(`/sessions/${sessionId}/connect`);

    // Verify mic section exists.
    await expect(page.locator('[data-testid="mic-section"]')).toBeVisible({
      timeout: 10000,
    });

    // Verify mic device selector exists.
    await expect(page.locator('[data-testid="mic-device-select"]')).toBeVisible();

    // Verify VU meter element exists.
    await expect(page.locator('[data-testid="mic-vu-meter"]')).toBeVisible();

    // Verify WebRTC debug panel exists.
    await expect(page.locator('[data-testid="webrtc-debug"]')).toBeVisible();
  });
});
