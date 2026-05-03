import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

const devProxyTarget = process.env.VITE_DEV_PROXY_TARGET ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: devProxyTarget,
        changeOrigin: true,
      },
      '/healthz': {
        target: devProxyTarget,
        changeOrigin: true,
      },
      '/ready': {
        target: devProxyTarget,
        changeOrigin: true,
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/__tests__/setup.ts'],
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})
