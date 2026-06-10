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
      // /admin/login and /admin/logout are dual-purpose: POST hits the Go
      // backend (form submit / logout action), GET is the SPA route that
      // mounts Login.vue. Bypass GET so Vite serves index.html on direct
      // navigation or F5; without this, refreshing /admin/login proxies the
      // GET to the backend (which only handles POST) and the browser ends up
      // with an empty body — the SPA never gets a chance to render.
      '/admin/login': {
        target: backendHost,
        bypass: (req) => (req.method === 'GET' ? req.url : undefined),
      },
      '/admin/logout': {
        target: backendHost,
        bypass: (req) => (req.method === 'GET' ? req.url : undefined),
      },
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
