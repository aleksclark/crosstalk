/**
 * accessibility-visual.spec.ts — axe + visual smoke for house-design SPAs.
 *
 * Runs against the real embedded stack (same harness as admin/translator/golden).
 * Screenshots are written under test/playwright/artifacts/ for the run report;
 * they are evidence, not approved goldens that mask functional failure.
 */
import { test, expect, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import * as fs from "fs";
import * as path from "path";
import { createHash } from "crypto";
import {
  adminLoginUI,
  translatorLoginUI,
  apiFetch,
  BASE_URL,
  ADMIN_PASSWORD,
} from "../helpers";

const ARTIFACT_DIR = path.join(__dirname, "..", "artifacts", "house-design");

function ensureDir(dir: string) {
  fs.mkdirSync(dir, { recursive: true });
}

async function axeScan(page: Page, name: string) {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  const serious = results.violations.filter(
    (v) => v.impact === "critical" || v.impact === "serious",
  );
  if (serious.length > 0) {
    const detail = serious
      .map(
        (v) =>
          `${v.id} (${v.impact}): ${v.help} — ${v.nodes
            .slice(0, 3)
            .map((n) => n.target.join(" "))
            .join("; ")}`,
      )
      .join("\n");
    throw new Error(`axe serious/critical on ${name}:\n${detail}`);
  }
  return results;
}

async function shot(page: Page, name: string, width: number, height: number) {
  ensureDir(ARTIFACT_DIR);
  await page.setViewportSize({ width, height });
  // Allow layout settle; freeze caret.
  await page.evaluate(() => {
    const style = document.createElement("style");
    style.textContent = `*, *::before, *::after { caret-color: transparent !important; }`;
    document.head.appendChild(style);
  });
  const file = path.join(ARTIFACT_DIR, `${name}-${width}x${height}.png`);
  await page.screenshot({ path: file, fullPage: true });
  const buf = fs.readFileSync(file);
  const sha = createHash("sha256").update(buf).digest("hex");
  fs.appendFileSync(
    path.join(ARTIFACT_DIR, "checksums.sha256"),
    `${sha}  ${path.basename(file)}\n`,
  );
  return { file, sha };
}

test.describe("House design accessibility + visual evidence", () => {
  test("admin login + dashboard axe and screenshots", async ({ page }) => {
    await page.goto("/admin/login");
    await axeScan(page, "admin-login");
    await shot(page, "admin-login", 1280, 900);
    await shot(page, "admin-login", 390, 844);

    await adminLoginUI(page);
    await expect(page.getByRole("heading", { name: /dashboard/i })).toBeVisible();
    await axeScan(page, "admin-dashboard");
    await shot(page, "admin-dashboard", 1280, 900);
    await shot(page, "admin-dashboard", 390, 844);

    // Font probe — product UI uses Archivo.
    const font = await page.evaluate(() => getComputedStyle(document.body).fontFamily);
    expect(font.toLowerCase()).toContain("archivo");
  });

  test("admin sessions list axe", async ({ page }) => {
    await adminLoginUI(page);
    await page.getByRole("navigation").getByRole("link", { name: "Sessions", exact: true }).click();
    await expect(page).toHaveURL(/\/admin\/sessions/);
    await axeScan(page, "admin-sessions");
    await shot(page, "admin-sessions", 1280, 900);
    await shot(page, "admin-sessions", 390, 844);
  });

  test("translator login + list axe and screenshots", async ({ page, request }) => {
    const adminToken = await adminLoginUI(page);
    const username = `a11y_xl8r_${Date.now()}`;
    const password = "translator-pass-123";
    await apiFetch(request, adminToken, "post", "/api/translators", {
      username,
      password,
    });

    const tpage = await page.context().newPage();
    await translatorLoginUI(tpage, username, password);
    await axeScan(tpage, "translator-sessions");
    await shot(tpage, "translator-sessions", 1280, 900);
    await shot(tpage, "translator-sessions", 390, 844);
    await tpage.close();
  });

  test("broadcast invalid link axe + screenshot", async ({ page }) => {
    await page.goto(
      "/broadcast/listen/01INVALIDSESSION0000000000?t=not-a-real-token",
    );
    await expect(page.getByTestId("broadcast-error-message")).toBeVisible({
      timeout: 15_000,
    });
    await axeScan(page, "broadcast-invalid");
    await shot(page, "broadcast-invalid", 390, 844);
    await shot(page, "broadcast-invalid", 1280, 900);
  });

  test("broadcast ready state screenshot when session exists", async ({
    page,
    request,
  }) => {
    // Login via API for speed.
    const login = await request.post(`${BASE_URL}/api/auth/login`, {
      data: { username: "admin", password: ADMIN_PASSWORD },
    });
    expect(login.ok()).toBeTruthy();
    const { access_token } = (await login.json()) as { access_token: string };
    const session = await apiFetch(request, access_token, "post", "/api/sessions", {
      name: `A11y Broadcast ${Date.now()}`,
    });
    const sessionId = session.id as string;
    const info = await apiFetch(
      request,
      access_token,
      "get",
      `/api/sessions/${sessionId}/broadcast-url`,
    );
    const token = info.broadcast_token as string;

    await page.goto(
      `/broadcast/listen/${sessionId}?t=${encodeURIComponent(token)}`,
    );
    await expect(page.getByTestId("listen-button")).toBeVisible({ timeout: 15_000 });
    await axeScan(page, "broadcast-ready");
    await shot(page, "broadcast-ready", 390, 844);
    await shot(page, "broadcast-ready", 1280, 900);
  });
});
