/**
 * admin.spec.ts — Admin SPA functional coverage against a live ct-server.
 *
 * Exercises the real admin UI end to end: login, dashboard, session creation,
 * translator account CRUD, and ABC audio controls (mocked route scenarios +
 * real-server provision/detail flow).
 */
import { test, expect, type Page, type Route } from "@playwright/test";
import { adminLoginUI, apiFetch, BASE_URL } from "../helpers";

const TEST_OUT_UID = "usb:0d8c:0014:path:platform-xhci-0_1";
const TEST_IN_UID = "usb:0d8c:0014:path:platform-xhci-0_1";

type AudioSettingsBody = {
  abc_id: string;
  connected: boolean;
  accepted_revision: number;
  overall_state: string;
  stale: boolean;
  desired: {
    revision: number;
    command_id?: string;
    output_device_uid?: string;
    output_volume_percent?: number;
    output_muted?: boolean;
    input_device_uid?: string;
    input_gain_percent?: number;
    updated_at?: string;
  };
  reported: {
    revision: number;
    output_device_uid?: string;
    input_device_uid?: string;
    observed_output_volume_percent?: number;
    observed_output_muted?: boolean;
    observed_input_gain_percent?: number;
    output_volume_state: string;
    output_mute_state: string;
    input_gain_state: string;
    error_code?: string;
    error_detail?: string;
    reported_at?: string;
    capabilities?: Array<{
      device_uid: string;
      direction?: string;
      backend?: string;
      supports_volume?: boolean;
      supports_mute?: boolean;
      supports_gain?: boolean;
    }>;
  };
};

function baseSettings(
  abcId: string,
  overrides: Partial<AudioSettingsBody> = {},
): AudioSettingsBody {
  const base: AudioSettingsBody = {
    abc_id: abcId,
    connected: true,
    accepted_revision: 0,
    overall_state: "unconfigured",
    stale: false,
    desired: { revision: 0 },
    reported: {
      revision: 0,
      output_volume_state: "unknown",
      output_mute_state: "unknown",
      input_gain_state: "unknown",
      capabilities: [],
    },
  };
  return {
    ...base,
    ...overrides,
    desired: { ...base.desired, ...(overrides.desired ?? {}) },
    reported: { ...base.reported, ...(overrides.reported ?? {}) },
  };
}

function withCaps(
  abcId: string,
  extra: Partial<AudioSettingsBody> = {},
): AudioSettingsBody {
  return baseSettings(abcId, {
    ...extra,
    reported: {
      revision: 0,
      output_volume_state: "unknown",
      output_mute_state: "unknown",
      input_gain_state: "unknown",
      output_device_uid: TEST_OUT_UID,
      input_device_uid: TEST_IN_UID,
      observed_output_volume_percent: 50,
      observed_output_muted: false,
      observed_input_gain_percent: 40,
      reported_at: new Date().toISOString(),
      capabilities: [
        {
          device_uid: TEST_OUT_UID,
          direction: "both",
          backend: "alsa",
          supports_volume: true,
          supports_mute: true,
          supports_gain: true,
        },
      ],
      ...(extra.reported ?? {}),
    },
  });
}

async function fulfillJson(
  route: Route,
  status: number,
  body: unknown,
): Promise<void> {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function openAbcDetail(
  page: Page,
  abcId: string,
  name = "Audio Booth",
): Promise<void> {
  // Stub ABC detail so we do not depend on a real row for mocked audio tests.
  await page.route(`**/api/abcs/${abcId}`, async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    await fulfillJson(route, 200, {
      id: abcId,
      name,
      connected: false,
      created_at: new Date().toISOString(),
    });
  });
  await page.goto(`/admin/abcs/${abcId}`);
  await expect(page.getByRole("heading", { name })).toBeVisible({
    timeout: 15_000,
  });
  await expect(page.getByTestId("abc-audio-controls")).toBeVisible();
}

test.describe("Admin SPA", () => {
  test("login lands on the dashboard", async ({ page }) => {
    await adminLoginUI(page);
    await expect(page.getByRole("heading", { name: /dashboard/i })).toBeVisible();
    await expect(page.getByText(/database healthy/i)).toBeVisible();
  });

  test("create a session via the UI", async ({ page, request }) => {
    const token = await adminLoginUI(page);
    const name = `UI Session ${Date.now()}`;

    await page.getByRole("link", { name: /sessions/i }).click();
    await expect(page).toHaveURL(/\/admin\/sessions/);
    await page.getByRole("button", { name: /new session/i }).click();
    await page.getByPlaceholder(/session name/i).fill(name);
    await page.getByRole("button", { name: /^create$/i }).click();

    await expect(page.getByText(name)).toBeVisible({ timeout: 15_000 });

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

    await page.getByRole("link", { name: /ABCs/i }).click();
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
    await expect(page.getByText(name)).toBeVisible({ timeout: 15_000 });
    await expect(tokenField).toHaveCount(0);
  });

  test("translator created in the UI persists across reload and can log in", async ({
    page,
  }) => {
    await adminLoginUI(page);
    const username = `xl8r_${Date.now()}`;
    const password = "translator-pass-123";

    await page.getByRole("link", { name: /translators/i }).click();
    await expect(page).toHaveURL(/\/admin\/translators/);
    await page.getByRole("button", { name: /new translator/i }).click();
    await page.getByPlaceholder(/translator_xx/i).fill(username);
    await page.getByPlaceholder(/•+/).fill(password);
    await page.getByRole("button", { name: /^create$/i }).click();

    // Appears in the list…
    await expect(page.getByText(username)).toBeVisible({ timeout: 15_000 });
    // …and is still there after a full reload (real persistence, not local state).
    await page.reload();
    await expect(page.getByText(username)).toBeVisible({ timeout: 15_000 });

    // …and the account actually authenticates.
    const resp = await page.request.post(`${BASE_URL}/api/auth/login`, {
      data: { username, password },
    });
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    expect(body.access_token).toBeTruthy();
  });
});

test.describe("Admin SPA — ABC audio controls", () => {
  test("loads audio controls and keeps save disabled without capability UIDs", async ({
    page,
  }) => {
    const abcId = "abc_audio_unconfigured";
    await adminLoginUI(page);

    let getCount = 0;
    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        getCount += 1;
        await fulfillJson(route, 200, baseSettings(abcId));
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Unconfigured Booth");

    await expect(page.getByRole("heading", { name: /audio controls/i })).toBeVisible();
    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "unconfigured",
    );
    await expect(page.getByTestId("abc-audio-save")).toBeDisabled();
    await expect(page.getByTestId("abc-audio-uid-hint")).toBeVisible();
    expect(getCount).toBeGreaterThan(0);
  });

  test("capability inventory initializes editable values and enables save", async ({
    page,
  }) => {
    const abcId = "abc_audio_caps";
    await adminLoginUI(page);

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        await fulfillJson(route, 200, withCaps(abcId));
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Caps Booth");

    await expect(page.getByTestId("abc-audio-output-volume")).toHaveValue("50");
    await expect(page.getByTestId("abc-audio-input-gain")).toHaveValue("40");
    await expect(page.getByTestId("abc-audio-output-mute")).not.toBeChecked();
    await expect(page.getByTestId("abc-audio-output-device")).toHaveAttribute(
      "title",
      TEST_OUT_UID,
    );
    await expect(page.getByTestId("abc-audio-save")).toBeEnabled();
  });

  test("save sends expected payload + stable request_id; 202 stays pending until applied readback", async ({
    page,
  }) => {
    const abcId = "abc_audio_save_202";
    await adminLoginUI(page);

    let phase: "caps" | "pending" | "applied" = "caps";
    const putBodies: Array<Record<string, unknown>> = [];
    let putCount = 0;

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      const method = route.request().method();
      if (method === "GET") {
        if (phase === "caps") {
          await fulfillJson(route, 200, withCaps(abcId));
        } else if (phase === "pending") {
          await fulfillJson(
            route,
            200,
            withCaps(abcId, {
              connected: true,
              accepted_revision: 1,
              overall_state: "pending",
              desired: {
                revision: 1,
                command_id: "cmd-1",
                output_device_uid: TEST_OUT_UID,
                output_volume_percent: 65,
                output_muted: false,
                input_device_uid: TEST_IN_UID,
                input_gain_percent: 40,
              },
              reported: {
                revision: 0,
                output_volume_state: "pending",
                output_mute_state: "pending",
                input_gain_state: "pending",
                output_device_uid: TEST_OUT_UID,
                input_device_uid: TEST_IN_UID,
                capabilities: [
                  {
                    device_uid: TEST_OUT_UID,
                    direction: "both",
                    supports_volume: true,
                    supports_mute: true,
                    supports_gain: true,
                  },
                ],
              },
            }),
          );
        } else {
          await fulfillJson(
            route,
            200,
            withCaps(abcId, {
              connected: true,
              accepted_revision: 1,
              overall_state: "applied",
              desired: {
                revision: 1,
                command_id: "cmd-1",
                output_device_uid: TEST_OUT_UID,
                output_volume_percent: 65,
                output_muted: false,
                input_device_uid: TEST_IN_UID,
                input_gain_percent: 40,
              },
              reported: {
                revision: 1,
                output_volume_state: "applied",
                output_mute_state: "applied",
                input_gain_state: "applied",
                observed_output_volume_percent: 65,
                observed_output_muted: false,
                observed_input_gain_percent: 40,
                output_device_uid: TEST_OUT_UID,
                input_device_uid: TEST_IN_UID,
                reported_at: new Date().toISOString(),
                capabilities: [
                  {
                    device_uid: TEST_OUT_UID,
                    direction: "both",
                    supports_volume: true,
                    supports_mute: true,
                    supports_gain: true,
                  },
                ],
              },
            }),
          );
        }
        return;
      }

      if (method === "PUT") {
        putCount += 1;
        const body = route.request().postDataJSON() as Record<string, unknown>;
        putBodies.push(body);
        phase = "pending";
        await fulfillJson(
          route,
          202,
          withCaps(abcId, {
            connected: true,
            accepted_revision: 1,
            overall_state: "pending",
            desired: {
              revision: 1,
              command_id: "cmd-1",
              output_device_uid: TEST_OUT_UID,
              output_volume_percent: 65,
              output_muted: false,
              input_device_uid: TEST_IN_UID,
              input_gain_percent: 40,
            },
            reported: {
              revision: 0,
              output_volume_state: "pending",
              output_mute_state: "pending",
              input_gain_state: "pending",
              output_device_uid: TEST_OUT_UID,
              input_device_uid: TEST_IN_UID,
              capabilities: [
                {
                  device_uid: TEST_OUT_UID,
                  direction: "both",
                  supports_volume: true,
                  supports_mute: true,
                  supports_gain: true,
                },
              ],
            },
          }),
        );
        // After a short delay, next poll sees applied.
        setTimeout(() => {
          phase = "applied";
        }, 500);
        return;
      }

      await route.continue();
    });

    await openAbcDetail(page, abcId, "Save Booth");

    await page.getByTestId("abc-audio-output-volume-number").fill("65");
    await page.getByTestId("abc-audio-save").click();

    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "pending",
      { timeout: 5_000 },
    );
    await expect(page.getByTestId("abc-audio-status-message")).toContainText(
      /queued|pending/i,
    );
    // Must NOT claim applied solely from 202.
    await expect(page.getByTestId("abc-audio-overall-badge")).not.toHaveAttribute(
      "data-state",
      "applied",
    );

    expect(putCount).toBe(1);
    expect(putBodies[0]).toMatchObject({
      expected_revision: 0,
      output: {
        device_uid: TEST_OUT_UID,
        volume_percent: 65,
        muted: false,
      },
      input: {
        device_uid: TEST_IN_UID,
        gain_percent: 40,
      },
    });
    expect(typeof putBodies[0].request_id).toBe("string");
    expect(String(putBodies[0].request_id).length).toBeGreaterThan(8);

    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "applied",
      { timeout: 8_000 },
    );
    await expect(page.getByTestId("abc-audio-desired-revision")).toHaveText("1");
    await expect(page.getByTestId("abc-audio-reported-revision")).toHaveText("1");
  });

  test("offline save shows reconnect message", async ({ page }) => {
    const abcId = "abc_audio_offline";
    await adminLoginUI(page);

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      const method = route.request().method();
      if (method === "GET") {
        await fulfillJson(
          route,
          200,
          withCaps(abcId, {
            connected: false,
            overall_state: "offline",
            desired: {
              revision: 1,
              output_device_uid: TEST_OUT_UID,
              output_volume_percent: 70,
              output_muted: true,
              input_device_uid: TEST_IN_UID,
              input_gain_percent: 30,
            },
            accepted_revision: 1,
          }),
        );
        return;
      }
      if (method === "PUT") {
        await fulfillJson(
          route,
          202,
          withCaps(abcId, {
            connected: false,
            overall_state: "offline",
            accepted_revision: 2,
            desired: {
              revision: 2,
              output_device_uid: TEST_OUT_UID,
              output_volume_percent: 70,
              output_muted: true,
              input_device_uid: TEST_IN_UID,
              input_gain_percent: 30,
            },
          }),
        );
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Offline Booth");
    // desired already present — editable from desired
    await expect(page.getByTestId("abc-audio-output-volume")).toHaveValue("70");
    await page.getByTestId("abc-audio-output-volume-number").fill("72");
    await page.getByTestId("abc-audio-save").click();

    await expect(page.getByTestId("abc-audio-status-message")).toContainText(
      /saved; will apply on reconnect/i,
      { timeout: 5_000 },
    );
  });

  test("unsupported controls are disabled", async ({ page }) => {
    const abcId = "abc_audio_unsupported";
    await adminLoginUI(page);

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        await fulfillJson(
          route,
          200,
          withCaps(abcId, {
            overall_state: "unsupported",
            reported: {
              revision: 0,
              output_volume_state: "unsupported",
              output_mute_state: "unsupported",
              input_gain_state: "unsupported",
              output_device_uid: TEST_OUT_UID,
              input_device_uid: TEST_IN_UID,
              capabilities: [
                {
                  device_uid: TEST_OUT_UID,
                  direction: "both",
                  supports_volume: false,
                  supports_mute: false,
                  supports_gain: false,
                },
              ],
            },
          }),
        );
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Unsupported Booth");
    await expect(page.getByTestId("abc-audio-output-volume")).toBeDisabled();
    await expect(page.getByTestId("abc-audio-output-mute")).toBeDisabled();
    await expect(page.getByTestId("abc-audio-input-gain")).toBeDisabled();
    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "unsupported",
    );
  });

  test("stale and error states surface explicit messages", async ({ page }) => {
    const abcId = "abc_audio_stale_error";
    await adminLoginUI(page);

    let mode: "stale" | "error" = "stale";
    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        if (mode === "stale") {
          await fulfillJson(
            route,
            200,
            withCaps(abcId, {
              overall_state: "stale",
              stale: true,
              desired: {
                revision: 1,
                output_device_uid: TEST_OUT_UID,
                output_volume_percent: 50,
                output_muted: false,
                input_device_uid: TEST_IN_UID,
                input_gain_percent: 40,
              },
              accepted_revision: 1,
            }),
          );
        } else {
          await fulfillJson(
            route,
            200,
            withCaps(abcId, {
              overall_state: "error",
              desired: {
                revision: 1,
                output_device_uid: TEST_OUT_UID,
                output_volume_percent: 50,
                output_muted: false,
                input_device_uid: TEST_IN_UID,
                input_gain_percent: 40,
              },
              accepted_revision: 1,
              reported: {
                revision: 0,
                output_volume_state: "error",
                output_mute_state: "error",
                input_gain_state: "error",
                error_code: "alsa_fail",
                error_detail: "mixer write failed",
                output_device_uid: TEST_OUT_UID,
                input_device_uid: TEST_IN_UID,
                capabilities: [
                  {
                    device_uid: TEST_OUT_UID,
                    direction: "both",
                    supports_volume: true,
                    supports_mute: true,
                    supports_gain: true,
                  },
                ],
              },
            }),
          );
        }
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Stale Booth");
    await expect(page.getByTestId("abc-audio-status-message")).toContainText(/stale/i);

    mode = "error";
    // Force a poll by waiting for the next GET (slow poll is 10s — trigger via navigation reload)
    await page.reload();
    await expect(page.getByTestId("abc-audio-status-message")).toContainText(
      /mixer write failed|error/i,
      { timeout: 15_000 },
    );
    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "error",
    );
  });

  test("409 conflict shows message and refetches without silent overwrite", async ({
    page,
  }) => {
    const abcId = "abc_audio_409";
    await adminLoginUI(page);

    let putSeen = false;
    let getAfterConflict = 0;

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      const method = route.request().method();
      if (method === "GET") {
        if (putSeen) getAfterConflict += 1;
        await fulfillJson(
          route,
          200,
          withCaps(abcId, {
            accepted_revision: putSeen ? 2 : 1,
            overall_state: putSeen ? "pending" : "applied",
            desired: {
              revision: putSeen ? 2 : 1,
              output_device_uid: TEST_OUT_UID,
              output_volume_percent: putSeen ? 90 : 50,
              output_muted: false,
              input_device_uid: TEST_IN_UID,
              input_gain_percent: 40,
            },
            reported: {
              revision: putSeen ? 0 : 1,
              output_volume_state: putSeen ? "pending" : "applied",
              output_mute_state: putSeen ? "pending" : "applied",
              input_gain_state: putSeen ? "pending" : "applied",
              observed_output_volume_percent: putSeen ? 50 : 50,
              observed_output_muted: false,
              observed_input_gain_percent: 40,
              output_device_uid: TEST_OUT_UID,
              input_device_uid: TEST_IN_UID,
              capabilities: [
                {
                  device_uid: TEST_OUT_UID,
                  direction: "both",
                  supports_volume: true,
                  supports_mute: true,
                  supports_gain: true,
                },
              ],
            },
          }),
        );
        return;
      }
      if (method === "PUT") {
        putSeen = true;
        await fulfillJson(route, 409, {
          title: "Conflict",
          status: 409,
          detail: "revision conflict: expected 1 have 2",
        });
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Conflict Booth");
    await page.getByTestId("abc-audio-output-volume-number").fill("88");
    await page.getByTestId("abc-audio-save").click();

    await expect(page.getByTestId("abc-audio-status-message")).toContainText(
      /conflict/i,
      { timeout: 5_000 },
    );
    // Refetch should land desired from server (90), not silent local 88.
    await expect(page.getByTestId("abc-audio-output-volume")).toHaveValue("90", {
      timeout: 5_000,
    });
    expect(getAfterConflict).toBeGreaterThan(0);
  });

  test("fetch failure surfaces explicit error (not only not-found)", async ({
    page,
  }) => {
    const abcId = "abc_audio_fetch_fail";
    await adminLoginUI(page);

    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        await fulfillJson(route, 500, {
          title: "Internal Server Error",
          status: 500,
          detail: "abc audio service not configured",
        });
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Fail Booth");
    await expect(page.getByTestId("abc-audio-fetch-error")).toContainText(
      /abc audio service not configured|Failed to load audio settings|500/i,
    );
  });

  test("stops polling after navigating away", async ({ page }) => {
    const abcId = "abc_audio_poll_stop";
    await adminLoginUI(page);

    let getCount = 0;
    await page.route(`**/api/abcs/${abcId}/audio-settings`, async (route) => {
      if (route.request().method() === "GET") {
        getCount += 1;
        await fulfillJson(
          route,
          200,
          withCaps(abcId, {
            overall_state: "pending",
            accepted_revision: 1,
            desired: {
              revision: 1,
              output_device_uid: TEST_OUT_UID,
              output_volume_percent: 50,
              output_muted: false,
              input_device_uid: TEST_IN_UID,
              input_gain_percent: 40,
            },
            reported: {
              revision: 0,
              output_volume_state: "pending",
              output_mute_state: "pending",
              input_gain_state: "pending",
              output_device_uid: TEST_OUT_UID,
              input_device_uid: TEST_IN_UID,
              capabilities: [
                {
                  device_uid: TEST_OUT_UID,
                  direction: "both",
                  supports_volume: true,
                  supports_mute: true,
                  supports_gain: true,
                },
              ],
            },
          }),
        );
        return;
      }
      await route.continue();
    });

    await openAbcDetail(page, abcId, "Poll Booth");
    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "pending",
    );

    // Allow at least one poll tick (2s) while still on the page.
    await page.waitForTimeout(2500);
    const countOnPage = getCount;
    expect(countOnPage).toBeGreaterThanOrEqual(2);

    await page.goto("/admin/dashboard");
    await expect(page.getByRole("heading", { name: /dashboard/i })).toBeVisible();
    const countAfterNav = getCount;
    await page.waitForTimeout(4500);
    // No further GETs after leave (allow at most one in-flight that started before nav).
    expect(getCount - countAfterNav).toBeLessThanOrEqual(1);
  });

  test("real server: provisioned ABC detail shows audio controls (unconfigured)", async ({
    page,
    request,
  }) => {
    const token = await adminLoginUI(page);
    const name = `Audio UI ${Date.now()}`;

    const created = await apiFetch(request, token, "post", "/api/abcs", { name });
    const abcId = created.id as string;
    expect(abcId).toBeTruthy();

    await page.goto(`/admin/abcs/${abcId}`);
    await expect(page.getByRole("heading", { name })).toBeVisible({
      timeout: 15_000,
    });
    await expect(page.getByTestId("abc-audio-controls")).toBeVisible();
    await expect(page.getByTestId("abc-audio-overall-badge")).toHaveAttribute(
      "data-state",
      "unconfigured",
    );
    await expect(page.getByTestId("abc-audio-save")).toBeDisabled();

    // Persistence: reload keeps the section.
    await page.reload();
    await expect(page.getByTestId("abc-audio-controls")).toBeVisible({
      timeout: 15_000,
    });
    await expect(page.getByTestId("abc-audio-desired-revision")).toHaveText("0");
  });

  test("translator token is denied direct audio-settings API access", async ({
    page,
    request,
  }) => {
    const adminToken = await adminLoginUI(page);
    const username = `xl8r_audio_${Date.now()}`;
    const password = "translator-pass-123";

    await apiFetch(request, adminToken, "post", "/api/translators", {
      username,
      password,
    });

    const abc = await apiFetch(request, adminToken, "post", "/api/abcs", {
      name: `Deny ${Date.now()}`,
    });
    const abcId = abc.id as string;

    const login = await page.request.post(`${BASE_URL}/api/auth/login`, {
      data: { username, password },
    });
    expect(login.ok()).toBeTruthy();
    const { access_token: translatorToken } = (await login.json()) as {
      access_token: string;
    };

    const denied = await page.request.get(
      `${BASE_URL}/api/abcs/${abcId}/audio-settings`,
      { headers: { Authorization: `Bearer ${translatorToken}` } },
    );
    expect(denied.status()).toBeGreaterThanOrEqual(400);
    expect([401, 403, 404]).toContain(denied.status());
  });
});
