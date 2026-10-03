import { Configuration, FrontendApi } from "@ory/client-fetch";

import { env } from "../shared/env";

/**
 * Kratos public client. Browser flows only (`createBrowser*Flow`); the native
 * `*\/api` flows are never used from the web (ADR-0003).
 * `credentials: "include"` lets the browser attach the HttpOnly session and
 * CSRF cookies; the app never reads or stores them.
 */
export const frontend = new FrontendApi(
  new Configuration({
    basePath: env.kratosPublicUrl,
    credentials: "include",
    headers: { Accept: "application/json" },
    // Resolve fetch lazily so test interceptors (MSW) and polyfills apply.
    fetchApi: (input, init) => globalThis.fetch(input, init),
  }),
);
