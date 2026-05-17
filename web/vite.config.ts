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
    },
  },
  build: {
    // outputtea directo al paquete Go que lo embebe vía //go:embed.
    // Tiene que existir antes de `go build` o el embed falla.
    outDir: '../internal/staticui/dist',
    emptyOutDir: true,
  },
});
