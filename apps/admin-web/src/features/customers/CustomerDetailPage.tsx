import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import { useCan } from "../../auth/me";
import { formatDateTime, personName } from "../../shared/format";
import { problemMessage } from "../../shared/problem";
import { showToast } from "../../shared/toast";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card, PageHeader } from "../../shared/ui/Card";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { Spinner } from "../../shared/ui/Spinner";
import { PersonalInfoCard } from "./PersonalInfoCard";
import { ReasonDialog } from "./ReasonDialog";
import { StateBadge } from "./StateBadge";
import {
  customerQuery,
  useDisableCustomer,
  useEnableCustomer,
  useRevokeCustomerSessions,
} from "./queries";

type DialogKind = "disable" | "enable" | "revoke" | null;

/** /customers/:id — detail + personal info (masked / reveal) + disable/enable (with reason) + revoke sessions, each behind a confirm dialog. */
export function CustomerDetailPage() {
  const { id = "" } = useParams();
  const { t, i18n } = useTranslation();
  const canManage = useCan("manage_customers");
  const [dialog, setDialog] = useState<DialogKind>(null);

  const query = useQuery(customerQuery(id));
  const disable = useDisableCustomer(id);
  const enable = useEnableCustomer(id);
  const revoke = useRevokeCustomerSessions(id);

  const back = (
    <Link to="/customers" className="text-sm text-blue-700 underline">
      {t("customers.back")}
    </Link>
  );

  if (query.isPending) return <Spinner />;
  if (query.isError) {
    return (
      <div className="space-y-4">
        <Alert kind="error">{problemMessage(t, query.error)}</Alert>
        {back}
      </div>
    );
  }

  const c = query.data;
  const close = () => setDialog(null);

  return (
    <div className="space-y-6">
      {back}
      <PageHeader
        title={c.email}
        actions={
          canManage ? (
            <div className="flex flex-wrap gap-2">
              {c.state === "active" ? (
                <Button variant="danger" onClick={() => setDialog("disable")}>
                  {t("customers.disable")}
                </Button>
              ) : (
                <Button onClick={() => setDialog("enable")}>{t("customers.enable")}</Button>
              )}
              <Button variant="secondary" onClick={() => setDialog("revoke")}>
                {t("customers.revokeSessions")}
              </Button>
            </div>
          ) : null
        }
      />
      <Card>
        <dl className="grid gap-4 sm:grid-cols-2">
          {(
            [
              [t("customers.id"), <code key="id">{c.id}</code>],
              [t("customers.name"), personName(c.name)],
              [t("customers.displayName"), c.display_name ?? "—"],
              [t("customers.state"), <StateBadge key="s" state={c.state} />],
              [t("customers.emailVerified"), c.email_verified ? t("admins.yes") : t("admins.no")],
              [t("customers.created"), formatDateTime(c.created_at, i18n.language)],
            ] as const
          ).map(([label, value]) => (
            <div key={label}>
              <dt className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                {label}
              </dt>
              <dd className="mt-1 text-sm text-slate-900">{value}</dd>
            </div>
          ))}
        </dl>
      </Card>
      <PersonalInfoCard key={c.id} customerId={c.id} />

      <ReasonDialog
        open={dialog === "disable"}
        title={t("customers.disableConfirmTitle")}
        description={t("customers.disableConfirmBody", { email: c.email })}
        confirmLabel={t("customers.disable")}
        requireReason
        danger
        busy={disable.isPending}
        onCancel={close}
        onConfirm={(reason) =>
          disable.mutate(reason, {
            onSuccess: () => {
              showToast("success", t("customers.disabled"));
              close();
            },
          })
        }
      />
      <ReasonDialog
        open={dialog === "enable"}
        title={t("customers.enableConfirmTitle")}
        description={t("customers.enableConfirmBody", { email: c.email })}
        confirmLabel={t("customers.enable")}
        requireReason={false}
        busy={enable.isPending}
        onCancel={close}
        onConfirm={(reason) =>
          enable.mutate(reason, {
            onSuccess: () => {
              showToast("success", t("customers.enabled"));
              close();
            },
          })
        }
      />
      <ConfirmDialog
        open={dialog === "revoke"}
        title={t("customers.revokeConfirmTitle")}
        description={t("customers.revokeConfirmBody", { email: c.email })}
        confirmLabel={t("customers.revokeSessions")}
        danger
        busy={revoke.isPending}
        onCancel={close}
        onConfirm={() =>
          revoke.mutate(undefined, {
            onSuccess: () => {
              showToast("success", t("customers.sessionsRevoked"));
              close();
            },
          })
        }
      />
    </div>
  );
}
