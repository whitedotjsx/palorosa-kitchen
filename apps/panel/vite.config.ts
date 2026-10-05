import { svelte } from '@sveltejs/vite-plugin-svelte'
import { defineConfig } from 'vite'
import { viteSingleFile } from 'vite-plugin-singlefile'

// The panel is served by the desktop panelserver (PANEL_PORT, default 5211).
// `pnpm dev` proxies the API there so the SPA can run with hot reload.
export default defineConfig({
  plugins: [
    svelte({ compilerOptions: { runes: true } }),
    viteSingleFile(),
  ],
  build: {
    target: 'es2022',
    // Inline the brand fonts (woff2) into the single file.
    assetsInlineLimit: 100_000_000,
  },
  server: {
    port: 5197,
    strictPort: true,
    proxy: {
      '/api': 'http://127.0.0.1:5211',
      '/health': 'http://127.0.0.1:5211',
      // PWA files are served by the Go panel server, not by Vite.
      '/manifest.webmanifest': 'http://127.0.0.1:5211',
      '/sw.js': 'http://127.0.0.1:5211',
      '/icon.png': 'http://127.0.0.1:5211',
    },
  },
})
