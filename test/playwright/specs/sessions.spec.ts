/**
 * CrossTalk Playwright Integration Tests — Session Workflow
 *
 * Tests session creation, status display, and the session connect view.
 */
import { test, expect } from "@playwright/test";

test.describe("Session workflow", () => {
  test.skip("should create session from template and show waiting status", async ({
    page,
  }) => {
    // Navigate to sessions
    // Click create session
    // Select template
    // Enter session name
    // Submit
    // Verify session appears in list with "waiting" status
  });

  test.skip("should show session connect view with audio controls", async ({
    page,
  }) => {
    // Navigate to a session's connect view
    // Verify mic selector element exists
    // Verify VU meter element exists
    // Verify WebRTC debug panel is present (even if empty)
  });

  test.skip("should show session as ended after deletion", async ({ page }) => {
    // Create a session via API (or UI)
    // Delete/end the session
    // Verify status shows "ended" in the list
  });
});
