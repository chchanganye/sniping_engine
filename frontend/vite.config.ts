import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    proxy: {
      // Go backend (dev)
      '/api/v1': {
        target: 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://127.0.0.1:8090',
        ws: true,
        changeOrigin: true,
      },
      '/api': {
        target: 'https://m.4008117117.com',
        changeOrigin: true,
        secure: true,
        configure: (proxy) => {
          proxy.on('proxyRes', (proxyRes) => {
            const setCookie = proxyRes.headers['set-cookie']
            if (!setCookie) return
            const cookies = Array.isArray(setCookie) ? setCookie : [String(setCookie)]
            proxyRes.headers['set-cookie'] = cookies.map((cookie) => {
              return cookie
                .replace(/;\s*Secure/gi, '')
                .replace(/;\s*Domain=[^;]+/gi, '')
            })
          })
        },
      },
    },
  },
})
