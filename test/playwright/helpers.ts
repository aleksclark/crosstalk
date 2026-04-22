/**
 * CrossTalk Playwright test helpers.
 *
 * Provides login, API reset, and common setup utilities for integration tests.
 */
import { type Page, type APIRequestContext, expect } from "@playwright/test";

const BASE_URL = process.env.CT_SERVER_URL || "http://localhost:8080";
const ADMIN_USERNAME = "admin";
const ADMIN_PASSWORD = "admin-password";

/**
 * Reset the server DB and re-seed admin user.
 * Returns the seed API token for direct API calls.
 */
export async function resetServer(
  request: APIRequestContext,
): Promise<string> {
  const resp = await request.post(`${BASE_URL}/api/test/reset`);
  expect(resp.ok()).toBeTruthy();
  const body = await resp.json();
  return body.token as string;
}

/**
 * Log in through the web UI with admin credentials.
 * Navigates to /login, fills form, submits, and waits for redirect to dashboard.
 */
export async function loginViaUI(page: Page): Promise<void> {
  await page.goto("/login");
  await page.fill("#username", ADMIN_USERNAME);
  await page.fill("#password", ADMIN_PASSWORD);
  await page.click('button[type="submit"]');
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 10000 });
}

/**
 * Create a template via the REST API.
 * Returns the template object with id.
 */
export async function createTemplateViaAPI(
  request: APIRequestContext,
  token: string,
  overrides: Record<string, unknown> = {},
): Promise<Record<string, unknown>> {
  const body = {
    name: "Test Template",
    is_default: false,
    roles: [
      { name: "translator", multi_client: false },
      { name: "studio", multi_client: false },
    ],
    mappings: [
      { source: "translator:mic", sink: "studio:output" },
    ],
    ...overrides,
  };
  const resp = await request.post(`${BASE_URL}/api/templates`, {
    data: body,
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(resp.ok()).toBeTruthy();
  return (await resp.json()) as Record<string, unknown>;
}

/**
 * Create a session via the REST API.
 */
export async function createSessionViaAPI(
  request: APIRequestContext,
  token: string,
  templateId: string,
  name: string = "Test Session",
): Promise<Record<string, unknown>> {
  const resp = await request.post(`${BASE_URL}/api/sessions`, {
    data: { template_id: templateId, name },
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(resp.ok()).toBeTruthy();
  return (await resp.json()) as Record<string, unknown>;
}
