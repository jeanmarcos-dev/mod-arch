import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

const backendHost = (typeof process !== 'undefined' && process.env?.VITE_BACKEND) || 'http://localhost:8080';

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': backendHost,
      '/ws': { target: backendHost, ws: true, changeOrigin: true },
      // The admin REST surface and the magic-link viewer entry must reach the
      // Go backend in dev; without these rules Vite would serve them as
      // static SPA paths and `admin.fetchMeStatus()` would return HTML, so
      // the AdminShell would never render.
      '/admin/api': backendHost,
      '/admin/login': backendHost,
      '/admin/logout': backendHost,
      '/v': backendHost,
    },
  },
  build: {
    // outputtea directo al paquete Go que lo embebe vía //go:embed.
    // Tiene que existir antes de `go build` o el embed falla.
    outDir: '../internal/staticui/dist',
    emptyOutDir: true,
  },
});
