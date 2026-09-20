/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** SPA 用の API ベース URL。デフォルトは '/api'（同一オリジンのプロキシ）。 */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
