import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  outputDir: './e2e/screenshots',
  use: {
    baseURL: 'http://localhost:19881',
    screenshot: 'off',
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
