/**
 * CrossTalk Playwright Integration Tests — Peer Assignment
 *
 * Tests the connections API and session peer assignment flow on
 * both the SessionDetailPage and SessionConnectPage.
 */
import { test, expect } from "@playwright/test";
import { resetServer, loginViaUI, createTemplateViaAPI } from "../helpers";

const BASE_URL = process.env.CT_SERVER_URL || "http://localhost:8080";

test.describe("Peer assignment", () => {
  let apiToken: string;

  test.beforeEach(async ({ request, page }) => {
    apiToken = await resetServer(request);
    await loginViaUI(page);
  });

  test("GET /api/connections returns list of peers", async ({ request }) => {
    const resp = await request.get(`${BASE_URL}/api/connections`, {
      headers: { Authorization: `Bearer ${apiToken}` },
    });
    expect(resp.ok()).toBeTruthy();
    const peers = await resp.json();
    expect(Array.isArray(peers)).toBeTruthy();
  });

  test("POST /api/sessions/:id/assign rejects missing fields", async ({
    request,
  }) => {
    const tmpl = await createTemplateViaAPI(request, apiToken);
    const sessionResp = await request.post(`${BASE_URL}/api/sessions`, {
      headers: {
        Authorization: `Bearer ${apiToken}`,
        "Content-Type": "application/json",
      },
      data: { template_id: tmpl.id, name: "assign-test" },
    });
    const session = await sessionResp.json();

    const resp = await request.post(
      `${BASE_URL}/api/sessions/${session.id}/assign`,
      {
        headers: {
          Authorization: `Bearer ${apiToken}`,
          "Content-Type": "application/json",
        },
        data: { peer_id: "", role: "" },
      },
    );
    expect(resp.status()).toBe(400);
  });

  test("POST /api/sessions/:id/assign rejects unknown peer", async ({
    request,
  }) => {
    const tmpl = await createTemplateViaAPI(request, apiToken);
    const sessionResp = await request.post(`${BASE_URL}/api/sessions`, {
      headers: {
        Authorization: `Bearer ${apiToken}`,
        "Content-Type": "application/json",
      },
      data: { template_id: tmpl.id, name: "assign-test-2" },
    });
    const session = await sessionResp.json();

    const resp = await request.post(
      `${BASE_URL}/api/sessions/${session.id}/assign`,
      {
        headers: {
          Authorization: `Bearer ${apiToken}`,
          "Content-Type": "application/json",
        },
        data: { peer_id: "nonexistent", role: "studio" },
      },
    );
    expect(resp.status()).toBe(400);
    const body = await resp.json();
    expect(body.error).toContain("peer not found");
  });

  test("session detail page shows assign peers card", async ({ page, request }) => {
    const tmpl = await createTemplateViaAPI(request, apiToken);
    const sessionResp = await request.post(`${BASE_URL}/api/sessions`, {
      headers: {
        Authorization: `Bearer ${apiToken}`,
        "Content-Type": "application/json",
      },
      data: { template_id: tmpl.id, name: "UI assign test" },
    });
    const session = await sessionResp.json();

    await page.goto(`/sessions/${session.id}`);
    await expect(page.locator("text=Assign Peers")).toBeVisible();
    await expect(page.locator('[data-testid="assign-role-select"]').first()).toBeVisible();
  });

  test("session connect page shows session peers card", async ({ page, request }) => {
    const tmpl = await createTemplateViaAPI(request, apiToken);
    const sessionResp = await request.post(`${BASE_URL}/api/sessions`, {
      headers: {
        Authorization: `Bearer ${apiToken}`,
        "Content-Type": "application/json",
      },
      data: { template_id: tmpl.id, name: "connect assign test" },
    });
    const session = await sessionResp.json();

    await page.goto(`/sessions/${session.id}/connect?role=translator`);
    await expect(page.locator("text=Session Peers")).toBeVisible({ timeout: 10_000 });
  });
});
