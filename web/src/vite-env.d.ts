/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** API root for a UI deployed apart from the server. Empty means same-origin. */
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
