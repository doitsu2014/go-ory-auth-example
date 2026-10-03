function required(name: "VITE_KRATOS_PUBLIC_URL" | "VITE_API_URL"): string {
  const value = import.meta.env[name];
  if (!value) {
    throw new Error(`Missing ${name}; copy .env.example to .env`);
  }
  return value.replace(/\/+$/, "");
}

function optional(name: "VITE_HYDRA_PUBLIC_URL", fallback: string): string {
  return (import.meta.env[name] || fallback).replace(/\/+$/, "");
}

/** Public, non-secret configuration. The bundle is public: never put secrets here. */
export const env = {
  kratosPublicUrl: required("VITE_KRATOS_PUBLIC_URL"),
  apiUrl: required("VITE_API_URL"),
  /** Hydra public URL, shown in token-request examples only (the app never calls it). */
  hydraPublicUrl: optional("VITE_HYDRA_PUBLIC_URL", "http://localhost:4444"),
} as const;
