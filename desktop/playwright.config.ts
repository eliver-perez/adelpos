import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  outputDir: `../.local/browser-artifacts/${process.env.ADEL_BROWSER_ENGINE ?? 'manual'}`,
  workers: 1,
  timeout: 60000,
  use: {
    baseURL: 'http://127.0.0.1:1421',
    viewport: { width: 1280, height: 720 },
    screenshot: 'only-on-failure',
    actionTimeout: 10000,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium', channel: 'chromium' } },
    { name: 'webkit', use: { browserName: 'webkit' } },
  ],
  webServer: {
    command: 'npm run build && npx vite preview --host 127.0.0.1 --port 1421 --strictPort',
    url: 'http://127.0.0.1:1421',
    reuseExistingServer: false,
  },
});
