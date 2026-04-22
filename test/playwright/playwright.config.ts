// @ts-check
import { defineConfig, devices } from "@playwright/test";

/**
 * CrossTalk integration tests — Playwright configuration.
 *
 * These tests run against a real ct-server serving the embedded web UI.
 * The server URL is passed via the CT_SERVER_URL environment variable.
 */
export default defineConfig({
  testDir: "./specs",
  timeout: 60_000,
  retries: 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: process.env.CT_SERVER_URL || "http://localhost:8080",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        // Grant mic/camera permissions for WebRTC tests.
        permissions: ["microphone", "camera"],
        launchOptions: {
          args: [
            "--use-fake-ui-for-media-stream",
            "--use-fake-device-for-media-stream",
            // When CT_FAKE_AUDIO_FILE is set, Chromium uses that file as
            // the fake microphone input (for golden audio tests).
            ...(process.env.CT_FAKE_AUDIO_FILE
              ? [
                  `--use-file-for-fake-audio-capture=${process.env.CT_FAKE_AUDIO_FILE}`,
                ]
              : []),
          ],
        },
      },
    },
  ],
});
