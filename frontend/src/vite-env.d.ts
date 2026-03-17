/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
}

interface Window {
  __APP_CONFIG__?: {
    VITE_API_BASE_URL?: string
  }
}
