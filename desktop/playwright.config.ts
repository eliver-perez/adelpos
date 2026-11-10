import { defineConfig } from '@playwright/test';

// Puerto local de la vista previa de Vite. Por omisión 1421; ADEL_BROWSER_PREVIEW_PORT permite ejecutar
// dos aceptaciones aisladas a la vez.
const previewPort = process.env.ADEL_BROWSER_PREVIEW_PORT ?? '1421';

export default defineConfig({
  testDir: './e2e',
  outputDir: `../.local/browser-artifacts/${process.env.ADEL_BROWSER_ENGINE ?? 'manual'}`,
  workers: 1,
  timeout: 60000,
  use: {
    baseURL: `http://127.0.0.1:${previewPort}`,
    viewport: { width: 1280, height: 720 },
    screenshot: 'only-on-failure',
    actionTimeout: 10000,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium', channel: 'chromium' } },
    { name: 'webkit', use: { browserName: 'webkit' } },
  ],
  webServer: {
    command: `npm run build && npx vite preview --host 127.0.0.1 --port ${previewPort} --strictPort`,
    url: `http://127.0.0.1:${previewPort}`,
    reuseExistingServer: false,
  },
});
