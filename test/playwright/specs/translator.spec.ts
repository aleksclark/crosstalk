/**
 * translator.spec.ts — Translator SPA functional coverage.
 *
 * Verifies a translator can log in through the translate interface, that their
 * session persists across reload (deep-linkable), and that an assigned session
 * appears in their list.
 */
import { test, expect } from "@playwright/test";
import { adminLoginUI, apiFetch } from "../helpers";

test.describe("Translator SPA", () => {
  test("login, session assignment is visible, auth survives reload", async ({
    page,
    request,
  }) => {
    // Admin sets up a session + a translator and assigns them (via API).
    const adminToken = await adminLoginUI(page);
    const sessionName = `Xlate ${Date.now()}`;
    const session = await apiFetch(request, adminToken, "post", "/api/sessions", {
      name: sessionName,
    });
    const username = `t_${Date.now()}`;
    const password = "translate-pass-123";
    const translator = await apiFetch(request, adminToken, "post", "/api/translators", {
      username,
      password,
    });
    await apiFetch(
      request,
      adminToken,
      "put",
      `/api/translators/${translator.id as string}/sessions`,
      { session_ids: [session.id as string] },
    );

    // Translator logs in through the translate interface.
    await page.goto("/translator/login");
    await page.fill("#username", username);
    await page.fill("#password", password);
    await page.getByRole("button", { name: /sign in|log in/i }).click();

    await expect(page.getByText(new RegExp(`logged in as ${username}`, "i"))).toBeVisible({
      timeout: 15_000,
    });
    // The assigned session is listed.
    await expect(page.getByText(sessionName)).toBeVisible();

    // Auth persists across a full reload (in-memory-only auth would bounce to /login).
    await page.reload();
    await expect(page.getByText(sessionName)).toBeVisible({ timeout: 15_000 });
    await expect(page).not.toHaveURL(/\/login/);
  });
});
