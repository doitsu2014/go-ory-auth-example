import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import type {
  CreateServiceClientRequest,
  ServiceClient,
  ServiceClientWithSecret,
} from "../../api/client";
import { useCan } from "../../auth/me";
import { useAuthRedirect } from "../../auth/useAuthRedirect";
import { formatDateTime } from "../../shared/format";
import { showToast } from "../../shared/toast";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { PageHeader } from "../../shared/ui/Card";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { Spinner } from "../../shared/ui/Spinner";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { auditKeys } from "../audit/queries";
import { CreateServiceClientDialog } from "./CreateServiceClientDialog";
import { serviceClientErrorMessage } from "./errors";
import {
  createServiceClient,
  deleteServiceClient,
  rotateServiceClientSecret,
  serviceClientKeys,
  serviceClientListQuery,
} from "./queries";
import { type OneTimeCredentials, SecretDialog } from "./SecretDialog";
import { TokenExample } from "./TokenExample";

type Pending =
  | { kind: "create" }
  | { kind: "rotate"; client: ServiceClient }
  | { kind: "delete"; client: ServiceClient };

function toCredentials(
  c: ServiceClientWithSecret,
  reason: OneTimeCredentials["reason"],
): OneTimeCredentials {
  return {
    clientId: c.client_id,
    name: c.name,
    scopes: c.scopes,
    secret: c.client_secret || undefined,
    reason,
  };
}

/**
 * /service-clients — list, create, rotate secret and delete machine clients.
 * Requires manage_service_clients (API enforced; the UI gate is cosmetic).
 *
 * Create and rotate return the one-time `client_secret`. They are plain calls
 * (not useMutation) and the secret lives ONLY in this component's state: it is
 * never written to the TanStack caches, URL, router state, web storage or
 * logs, is dropped when the secret dialog closes and on unmount.
 */
export function ServiceClientsPage() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const canManage = useCan("manage_service_clients");
  const redirectOnAuthError = useAuthRedirect();
  const query = useQuery({ ...serviceClientListQuery(), enabled: canManage });

  const [pending, setPending] = useState<Pending | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [credentials, setCredentials] = useState<OneTimeCredentials | null>(null);

  // A call that resolves after unmount must not resurrect the secret.
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  function close() {
    setPending(null);
    setError(undefined);
  }

  function refresh() {
    return Promise.all([
      qc.invalidateQueries({ queryKey: serviceClientKeys.lists() }),
      // Every change writes an audit event.
      qc.invalidateQueries({ queryKey: auditKeys.all }),
    ]);
  }

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError(undefined);
    try {
      await action();
    } catch (err) {
      if (!mounted.current || redirectOnAuthError(err)) return;
      setError(serviceClientErrorMessage(t, err));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }

  function onCreate(body: CreateServiceClientRequest, idempotencyKey: string) {
    return run(async () => {
      const created = await createServiceClient(body, idempotencyKey);
      if (!mounted.current) return;
      setCredentials(toCredentials(created, "created"));
      close();
      void refresh();
    });
  }

  function onRotate(client: ServiceClient) {
    return run(async () => {
      const rotated = await rotateServiceClientSecret(client.client_id);
      if (!mounted.current) return;
      setCredentials(toCredentials(rotated, "rotated"));
      close();
      void refresh();
    });
  }

  function onDelete(client: ServiceClient) {
    return run(async () => {
      await deleteServiceClient(client.client_id);
      if (!mounted.current) return;
      showToast("success", t("serviceClients.deleted", { name: client.name }));
      close();
      void refresh();
    });
  }

  if (!canManage) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("serviceClients.title")} />
        <Alert kind="error">{t("serviceClients.errors.forbidden")}</Alert>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("serviceClients.title")}
        actions={
          <Button onClick={() => setPending({ kind: "create" })}>
            {t("serviceClients.create")}
          </Button>
        }
      />
      <p className="text-sm text-slate-600">{t("serviceClients.intro")}</p>

      {query.isError ? (
        <Alert kind="error">{serviceClientErrorMessage(t, query.error)}</Alert>
      ) : query.isPending ? (
        <Spinner />
      ) : (
        <Table
          caption={t("serviceClients.title")}
          head={
            <tr>
              <Th>{t("serviceClients.name")}</Th>
              <Th>{t("serviceClients.clientId")}</Th>
              <Th>{t("serviceClients.owner")}</Th>
              <Th>{t("serviceClients.scopes")}</Th>
              <Th>{t("serviceClients.created")}</Th>
              <Th>{t("common.actions")}</Th>
            </tr>
          }
        >
          {query.data.items.length === 0 ? (
            <EmptyRow colSpan={6}>{t("serviceClients.empty")}</EmptyRow>
          ) : (
            query.data.items.map((c) => (
              <tr key={c.client_id}>
                <Td>{c.name}</Td>
                <Td>
                  <span className="font-mono text-xs break-all">{c.client_id}</span>
                </Td>
                <Td>{c.owner}</Td>
                <Td>
                  <span className="font-mono text-xs">{c.scopes.join(" ")}</span>
                </Td>
                <Td>{formatDateTime(c.created_at, i18n.language)}</Td>
                <Td>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="secondary"
                      aria-label={t("serviceClients.rotateFor", { name: c.name })}
                      onClick={() => setPending({ kind: "rotate", client: c })}
                    >
                      {t("serviceClients.rotate")}
                    </Button>
                    <Button
                      variant="danger"
                      aria-label={t("serviceClients.deleteFor", { name: c.name })}
                      onClick={() => setPending({ kind: "delete", client: c })}
                    >
                      {t("serviceClients.delete")}
                    </Button>
                  </div>
                </Td>
              </tr>
            ))
          )}
        </Table>
      )}

      {pending?.kind === "create" ? (
        <CreateServiceClientDialog
          busy={busy}
          error={error}
          onCancel={close}
          onConfirm={(body, key) => void onCreate(body, key)}
        />
      ) : null}

      {pending?.kind === "rotate" ? (
        <ConfirmDialog
          open
          wide
          danger
          title={t("serviceClients.rotateConfirmTitle", { name: pending.client.name })}
          description={t("serviceClients.rotateConfirmBody")}
          confirmLabel={t("serviceClients.rotate")}
          busy={busy}
          onCancel={close}
          onConfirm={() => void onRotate(pending.client)}
        >
          {error ? <Alert kind="error">{error}</Alert> : null}
          <TokenExample clientId={pending.client.client_id} scopes={pending.client.scopes} />
        </ConfirmDialog>
      ) : null}

      {pending?.kind === "delete" ? (
        <ConfirmDialog
          open
          wide
          danger
          title={t("serviceClients.deleteConfirmTitle", { name: pending.client.name })}
          description={t("serviceClients.deleteConfirmBody")}
          confirmLabel={t("serviceClients.delete")}
          busy={busy}
          onCancel={close}
          onConfirm={() => void onDelete(pending.client)}
        >
          {error ? <Alert kind="error">{error}</Alert> : null}
          <TokenExample clientId={pending.client.client_id} scopes={pending.client.scopes} />
        </ConfirmDialog>
      ) : null}

      {credentials ? (
        <SecretDialog credentials={credentials} onClose={() => setCredentials(null)} />
      ) : null}
    </div>
  );
}
