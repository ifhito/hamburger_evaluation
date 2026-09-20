import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  // '.' = frontend ディレクトリ（vite の cwd）。process.cwd() のために @types/node を必要としないようにする。
  const env = loadEnv(mode, '.', '')
  // /api と /photos に共通の dev サーバー用プロキシ先。デフォルトはホスト上の Go API。
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
        // レビュー写真は API の /photos から配信される（path の rewrite なし）。
        '/photos': {
          target: proxyTarget,
          changeOrigin: true,
        },
      },
    },
  }
})
