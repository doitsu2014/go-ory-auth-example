import { useState } from "react";
import { useTranslation } from "react-i18next";

import type { MachineScope } from "../../api/client";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { TokenExample } from "./TokenExample";

/** The one-time credentials. Held only in the page's component state. */
export interface OneTimeCredentials {
  clientId: string;
  name: string;
  scopes: readonly MachineScope[];
  /** Absent on an idempotent replay of a create: the secret was already returned once. */
  secret: string | undefined;
  reason: "created" | "rotated";
}

/**
 * Shows client_id + client_secret once, with a copy button and a "won't be
 * shown again" warning. Render only while there are credentials to show; the
 * caller drops them on close, so nothing survives the dialog.
 *
 * Closing is only possible through the explicit Close button, enabled once the
 * admin ticks "I have stored the secret". Escape is ignored (there is no
 * backdrop dismissal), so the one-time secret cannot be thrown away by accident.
 */
export function SecretDialog({
  credentials,
  onClose,
}: {
  credentials: OneTimeCredentials;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [copy, setCopy] = useState<"idle" | "copied" | "failed">("idle");
  const { secret } = credentials;
  const [stored, setStored] = useState(false);

  async function onCopy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setCopy("copied");
    } catch {
      setCopy("failed");
    }
  }

  return (
    <ConfirmDialog
      open
      wide
      hideCancel
      title={t(`serviceClients.secretTitle.${credentials.reason}`, { name: credentials.name })}
      confirmLabel={t("common.close")}
      confirmDisabled={secret !== undefined && !stored}
      onConfirm={() => {
        if (secret === undefined || stored) onClose();
      }}
      // Escape must not discard the one-time secret.
      onCancel={() => undefined}
    >
      <dl className="space-y-3">
        <div>
          <dt className="text-xs font-medium tracking-wide text-slate-500 uppercase">
            {t("serviceClients.clientId")}
          </dt>
          <dd className="mt-1 font-mono text-sm break-all text-slate-900">
            {credentials.clientId}
          </dd>
        </div>
        {secret ? (
          <div>
            <dt className="text-xs font-medium tracking-wide text-slate-500 uppercase">
              {t("serviceClients.clientSecret")}
            </dt>
            <dd className="mt-1 flex flex-wrap items-center gap-2">
              <code
                className="rounded bg-slate-100 px-2 py-1 font-mono text-sm break-all text-slate-900 select-all"
                data-testid="client-secret"
              >
                {secret}
              </code>
              <Button variant="secondary" onClick={() => void onCopy(secret)}>
                {t("serviceClients.copySecret")}
              </Button>
            </dd>
          </div>
        ) : null}
      </dl>
      {secret ? (
        <Alert kind="warning">{t("serviceClients.secretWarning")}</Alert>
      ) : (
        <Alert kind="warning">{t("serviceClients.secretMissing")}</Alert>
      )}
      {copy === "copied" ? <Alert kind="success">{t("serviceClients.copied")}</Alert> : null}
      {copy === "failed" ? <Alert kind="error">{t("serviceClients.copyFailed")}</Alert> : null}
      <TokenExample clientId={credentials.clientId} scopes={credentials.scopes} />
      {secret ? (
        <label className="flex items-center gap-2 text-sm font-medium text-slate-800">
          <input
            type="checkbox"
            checked={stored}
            onChange={(e) => setStored(e.target.checked)}
            className="h-4 w-4 rounded border-slate-300"
          />
          {t("serviceClients.secretDone")}
        </label>
      ) : null}
    </ConfirmDialog>
  );
}
