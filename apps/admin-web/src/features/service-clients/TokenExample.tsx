import { useTranslation } from "react-i18next";

import type { MachineScope } from "../../api/client";
import { SECRET_PLACEHOLDER, tokenRequestExample } from "./scopes";

/** How to get an access token with this client. Never contains the secret value. */
export function TokenExample({
  clientId,
  scopes,
}: {
  clientId: string;
  scopes: readonly MachineScope[];
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <h3 className="text-sm font-medium text-slate-700">
        {t("serviceClients.tokenExampleTitle")}
      </h3>
      <pre
        className="overflow-x-auto rounded-md bg-slate-900 p-3 text-xs text-slate-100"
        data-testid="token-example"
      >
        <code>{tokenRequestExample(clientId, scopes)}</code>
      </pre>
      <p className="text-xs text-slate-500">
        {t("serviceClients.tokenExampleHint", { placeholder: SECRET_PLACEHOLDER })}
      </p>
    </div>
  );
}
