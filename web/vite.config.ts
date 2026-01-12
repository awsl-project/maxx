import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      external: ['@wailsio/runtime'],
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/admin': {
        target: 'http://localhost:9880',
        changeOrigin: true,
      },
      '/antigravity': {
        target: 'http://localhost:9880',
        changeOrigin: true,
      },
      '/ws': {
        target: 'http://localhost:9880',
        ws: true,
      },
      '/health': {
        target: 'http://localhost:9880',
        changeOrigin: true,
      },
    },
  },
})
