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

// ── Broadcast Helpers ────────────────────────────────────────────────────

/**
 * Create a template with a broadcast mapping via the REST API.
 * By default the template maps "studio:mic → broadcast" and includes
 * a regular role-to-role mapping so the template has two roles.
 */
export async function createBroadcastTemplateViaAPI(
  request: APIRequestContext,
  token: string,
  overrides: Record<string, unknown> = {},
): Promise<Record<string, unknown>> {
  return createTemplateViaAPI(request, token, {
    name: "Broadcast Template",
    roles: [
      { name: "studio", multi_client: false },
      { name: "translator", multi_client: false },
    ],
    mappings: [
      { source: "studio:mic", sink: "broadcast" },
      { source: "translator:mic", sink: "studio:output" },
    ],
    ...overrides,
  });
}

/**
 * Generate a broadcast token for the given session via the REST API.
 * Returns the full response: { token, url, expires_at }.
 */
export async function createBroadcastTokenViaAPI(
  request: APIRequestContext,
  token: string,
  sessionId: string,
): Promise<{ token: string; url: string; expires_at: string }> {
  const resp = await request.post(
    `${BASE_URL}/api/sessions/${sessionId}/broadcast-token`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  expect(resp.ok()).toBeTruthy();
  return (await resp.json()) as { token: string; url: string; expires_at: string };
}

/**
 * Fetch the session detail via the REST API and return listener_count.
 */
export async function getSessionListenerCount(
  request: APIRequestContext,
  token: string,
  sessionId: string,
): Promise<number> {
  const resp = await request.get(`${BASE_URL}/api/sessions/${sessionId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(resp.ok()).toBeTruthy();
  const body = (await resp.json()) as Record<string, unknown>;
  return (body.listener_count as number) ?? 0;
}
