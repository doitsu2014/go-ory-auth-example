/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_KRATOS_PUBLIC_URL: string;
  readonly VITE_API_URL: string;
  /** Optional; defaults to the local compose Hydra. */
  readonly VITE_HYDRA_PUBLIC_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
