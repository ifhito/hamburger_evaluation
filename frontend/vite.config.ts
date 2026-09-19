import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  // '.' = the frontend dir (vite's cwd); avoids needing @types/node for process.cwd().
  const env = loadEnv(mode, '.', '')
  // Dev-only proxy target for /api; defaults to the Go API on the host.
  const proxyTarget = env.VITE_API_PROXY_TARGET ?? 'http://host.docker.internal:8080'
  return {
    plugins: [react()],
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        '/api': {
          target: proxyTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
  }
})
