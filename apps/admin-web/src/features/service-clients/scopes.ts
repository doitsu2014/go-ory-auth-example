import type { MachineScope } from "../../api/client";
import { env } from "../../shared/env";

/** i18n-safe keys for scopes (":" is i18next's namespace separator). */
export const SCOPE_KEY: Record<MachineScope, "customersRead" | "auditRead"> = {
  "customers:read": "customersRead",
  "audit:read": "auditRead",
};

/** Public token endpoint of Hydra (VITE_HYDRA_PUBLIC_URL). Not a secret. */
export const TOKEN_URL = `${env.hydraPublicUrl}/oauth2/token`;

/** The audience every identity-service machine client is registered with. */
export const AUDIENCE = "identity-service";

/** Shell variable used in place of the secret in examples: the real value is never rendered there. */
export const SECRET_PLACEHOLDER = "$CLIENT_SECRET";

/** curl example for the client_credentials grant (client_secret_basic). */
export function tokenRequestExample(clientId: string, scopes: readonly MachineScope[]): string {
  return [
    `curl -s -X POST ${TOKEN_URL} \\`,
    `  -u "${clientId}:${SECRET_PLACEHOLDER}" \\`,
    "  -d grant_type=client_credentials \\",
    `  -d scope="${scopes.join(" ")}" \\`,
    `  -d audience=${AUDIENCE}`,
  ].join("\n");
}
