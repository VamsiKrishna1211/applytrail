import { fileURLToPath, URL } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The bundle is embedded into the Go binary, so it builds straight into the
// package that carries the //go:embed directive. That directory is gitignored
// and only compiled under `-tags webui`, which is why `go build ./...` still
// works on a clone that has never run npm.
const EMBED_DIR = '../internal/transport/webui/dist'

// In development Vite serves the UI and proxies the API, so the browser sees a
// single origin and the server needs no CORS configuration at any point.
const DEV_API = process.env.APPLYTRAIL_DEV_API ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  build: {
    outDir: EMBED_DIR,
    emptyOutDir: true,
    sourcemap: false,
    // Hashed filenames are what make the year-long cache header on /assets
    // safe; index.html is the only file served no-cache.
    rollupOptions: {
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/v1': { target: DEV_API, changeOrigin: false },
      '/healthz': { target: DEV_API, changeOrigin: false },
      '/version': { target: DEV_API, changeOrigin: false },
      '/auth': { target: DEV_API, changeOrigin: false },
    },
  },
})
