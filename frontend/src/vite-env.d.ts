/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** API base URL for the SPA; defaults to '/api' (same-origin proxy). */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
