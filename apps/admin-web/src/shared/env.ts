function required(name: "VITE_KRATOS_PUBLIC_URL" | "VITE_API_URL"): string {
  const value = import.meta.env[name];
  if (!value) {
    throw new Error(`Missing ${name}; copy .env.example to .env`);
  }
  return value.replace(/\/+$/, "");
}

/** Public, non-secret configuration. The bundle is public: never put secrets here. */
export const env = {
  kratosPublicUrl: required("VITE_KRATOS_PUBLIC_URL"),
  apiUrl: required("VITE_API_URL"),
} as const;
