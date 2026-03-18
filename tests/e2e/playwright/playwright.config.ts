import { defineConfig, devices } from 'playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: ['*.spec.ts'],
  fullyParallel: false,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  outputDir: 'test-results',
  use: {
    // The Playwright suite exercises the backend-served web app on :9880.
    // This keeps local runs aligned with the CI workflow that builds web/dist
    // before compiling the Go binary.
    baseURL: process.env.MAXX_E2E_BASE_URL || 'http://localhost:9880',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'e2e-chromium',
      testIgnore: ['stats-page.spec.ts'],
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      name: 'desktop-chromium',
      testMatch: ['stats-page.spec.ts'],
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      name: 'mobile-chromium',
      testMatch: ['stats-page.spec.ts'],
      use: {
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
        userAgent:
          'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/131.0.0.0 Mobile/15E148 Safari/604.1',
      },
    },
  ],
});
