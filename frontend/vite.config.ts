/// <reference types="vitest/config" />
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  // '.' は frontend ディレクトリ(vite の cwd)。process.cwd() のために @types/node を入れずに済ませる。
  const env = loadEnv(mode, '.', '')
  // 開発時だけ使う、/api と /photos に共通のプロキシ先。既定はホスト上の Go API。
  const proxyTarget = env.VITE_API_PROXY_TARGET ?? 'http://host.docker.internal:8080'
  return {
    plugins: [react()],
    test: {
      // 既定では、テストの中で CSS を ?raw で読むと空の文字列になる。デザインのトークン(CSS)との一致を確かめるテストのために、
      // ?raw で読む CSS だけは、中身をそのまま返す(CSS モジュールの処理には影響しない)。
      css: { include: /\.css\?raw$/ },
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        '/api': {
          target: proxyTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
          configure: (proxy) => {
            // バックエンドのコンテナが落ちているとき、index.html にフォールバックせず JSON のエラーを返す。
            proxy.on('error', (_err, _req, res) => {
              res.writeHead(503, { 'Content-Type': 'application/json' })
              res.end(JSON.stringify({ error: 'Backend service unavailable' }))
            })
          },
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
