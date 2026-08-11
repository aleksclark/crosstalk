/**
 * admin.spec.ts — Admin SPA functional coverage against a live ct-server.
 *
 * Exercises the real admin UI end to end: login, dashboard, session creation,
 * and translator account CRUD — including that a created translator PERSISTS
 * across a full page reload and can then authenticate (the bug that motivated
 * generating the API client from the server spec).
 */
import { test, expect } from "@playwright/test";
import { adminLoginUI, apiFetch, BASE_URL } from "../helpers";

test.describe("Admin SPA", () => {
  test("login lands on the dashboard", async ({ page }) => {
    await adminLoginUI(page);
    await expect(page.getByRole("heading", { name: /dashboard/i })).toBeVisible();
    // Dashboard is a truthful collection snapshot — no hard-coded health claims.
    await expect(
      page.getByText(/^Collection snapshot:/i),
    ).toBeVisible();
    await expect(page.getByText(/database healthy/i)).toHaveCount(0);
  });

  test("create a session via the UI", async ({ page, request }) => {
    const token = await adminLoginUI(page);
    const name = `UI Session ${Date.now()}`;

    await page.getByRole("navigation").getByRole("link", { name: "Sessions", exact: true }).click();
    await expect(page).toHaveURL(/\/admin\/sessions/);
    await page.getByRole("button", { name: /new session/i }).click();
    await page.getByRole("textbox", { name: /session name/i }).fill(name);
    await page.getByRole("button", { name: /^create$/i }).click();

    await expect(page.getByText(name).first()).toBeVisible({ timeout: 15_000 });

    // And it is really persisted server-side.
    const sessions = (await apiFetch(request, token, "get", "/api/sessions"))
      .data as Array<{ name: string }>;
    expect(sessions.some((s) => s.name === name)).toBeTruthy();
  });

  test("ABC provisioned in the UI persists and shows its token once", async ({
    page,
    request,
  }) => {
    const token = await adminLoginUI(page);
    const name = `UI ABC ${Date.now()}`;

    await page.getByRole("navigation").getByRole("link", { name: "ABCs", exact: true }).click();
    await expect(page).toHaveURL(/\/admin\/abcs/);
    await page.getByRole("button", { name: /new abc/i }).click();
    await page.getByPlaceholder(/booth a/i).fill(name);
    await page.getByRole("button", { name: /^create$/i }).click();

    const tokenField = page.getByRole("textbox", { name: /abc token/i });
    await expect(tokenField).toBeVisible({ timeout: 15_000 });
    await expect(tokenField).not.toHaveValue("");
    await expect(page.getByText(/cannot be retrieved/i)).toBeVisible();
    await expect(page.getByText(name).first()).toBeVisible();

    const abcs = (await apiFetch(request, token, "get", "/api/abcs")).data as Array<{
      name: string;
    }>;
    expect(abcs.some((abc) => abc.name === name)).toBeTruthy();

    await page.reload();
    await expect(page.getByText(name).first()).toBeVisible({ timeout: 15_000 });
    await expect(tokenField).toHaveCount(0);
  });

  test("translator created in the UI persists across reload and can log in", async ({
    page,
  }) => {
    await adminLoginUI(page);
    const username = `xl8r_${Date.now()}`;
    const password = "translator-pass-123";

    await page
      .getByRole("navigation")
      .getByRole("link", { name: "Translators", exact: true })
      .click();
    await expect(page).toHaveURL(/\/admin\/translators/);
    await page.getByRole("button", { name: /new translator/i }).click();
    await page.getByPlaceholder(/translator_xx/i).fill(username);
    await page.getByPlaceholder(/•+/).fill(password);
    await page.getByRole("button", { name: /^create$/i }).click();

    // Appears in the list…
    await expect(page.getByText(username).first()).toBeVisible({ timeout: 15_000 });
    // …and is still there after a full reload (real persistence, not local state).
    await page.reload();
    await expect(page.getByText(username).first()).toBeVisible({ timeout: 15_000 });

    // …and the account actually authenticates.
    const resp = await page.request.post(`${BASE_URL}/api/auth/login`, {
      data: { username, password },
    });
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.access_token).toBeTruthy();
  });
});
