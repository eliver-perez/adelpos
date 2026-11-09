/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  server: {
    host: '127.0.0.1',
    port: 1420,
    strictPort: true,
    fs: { allow: ['..'] },
    watch: { ignored: ['**/src-tauri/**'] },
  },
  build: { target: ['es2022', 'safari15.4', 'chrome105'] },
  test: {
    // Monta la pila de avisos (Sonner) en cada prueba con DOM, como App.tsx en la aplicación.
    setupFiles: ['./src/testing/toasterSetup.tsx'],
  },
});
