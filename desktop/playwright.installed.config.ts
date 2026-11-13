import { defineConfig } from '@playwright/test';

// Prueba de humo de la aplicación INSTALADA en Windows (etapa 9E, §5). No arranca navegadores ni servidores:
// se conecta por el protocolo de depuración al WebView2 real de AD-EL POS, que se abre con
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222 (scripts/windows/installed-smoke.ps1). Todo
// pasa por el IPC, Go, PostgreSQL y el Administrador de credenciales reales de la instalación.
export default defineConfig({
  testDir: './e2e-installed',
  outputDir: '../.local/installed-artifacts',
  workers: 1,
  timeout: 300000,
  expect: { timeout: 15000 },
  use: {
    actionTimeout: 15000,
  },
});
