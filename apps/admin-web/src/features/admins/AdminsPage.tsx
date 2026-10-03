import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { type Admin, ROLES, type Role } from "../../api/client";
import { useAdminMe, useCan } from "../../auth/me";
import { formatDateTime, personName } from "../../shared/format";
import { problemMessage } from "../../shared/problem";
import { showToast } from "../../shared/toast";
import { useCursorPagination } from "../../shared/useCursorPagination";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { PageHeader } from "../../shared/ui/Card";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { Pagination } from "../../shared/ui/Pagination";
import { Spinner } from "../../shared/ui/Spinner";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { StateBadge } from "../customers/StateBadge";
import { InviteAdminForm } from "./InviteAdminForm";
import { adminListQuery, useChangeAdminRole } from "./queries";

function RoleCell({
  admin,
  isSelf,
  canManage,
  onChange,
}: {
  admin: Admin;
  isSelf: boolean;
  canManage: boolean;
  onChange: (admin: Admin, role: Role) => void;
}) {
  const { t } = useTranslation();
  const [role, setRole] = useState<Role>(admin.role);
  if (!canManage || isSelf) return <>{t(`admins.roles.${admin.role}`)}</>;
  const selectId = `role-${admin.id}`;
  return (
    <div className="flex items-center gap-2">
      <label htmlFor={selectId} className="sr-only">
        {`${t("admins.role")} ${admin.email}`}
      </label>
      <select
        id={selectId}
        value={role}
        onChange={(e) => setRole(e.target.value as Role)}
        className="rounded-md border-0 py-1 pr-8 pl-2 text-sm ring-1 ring-slate-300 ring-inset"
      >
        {ROLES.map((r) => (
          <option key={r} value={r}>
            {t(`admins.roles.${r}`)}
          </option>
        ))}
      </select>
      <Button
        variant="secondary"
        disabled={role === admin.role}
        onClick={() => onChange(admin, role)}
        aria-label={`${t("admins.changeRole")} ${admin.email}`}
      >
        {t("admins.changeRole")}
      </Button>
    </div>
  );
}

/** /admins — list, invite (Idempotency-Key) and change role. Requires manage_admins (API enforced). */
export function AdminsPage() {
  const { t, i18n } = useTranslation();
  const me = useAdminMe();
  const canManage = useCan("manage_admins");
  const pager = useCursorPagination();
  const query = useQuery(adminListQuery(pager.pageToken));
  const changeRole = useChangeAdminRole();
  const [pending, setPending] = useState<{ admin: Admin; role: Role } | null>(null);

  return (
    <div className="space-y-6">
      <PageHeader title={t("admins.title")} />
      {canManage ? <InviteAdminForm /> : null}

      {query.isError ? (
        <Alert kind="error">{problemMessage(t, query.error)}</Alert>
      ) : query.isPending ? (
        <Spinner />
      ) : (
        <div>
          <Table
            caption={t("admins.title")}
            head={
              <tr>
                <Th>{t("admins.email")}</Th>
                <Th>{t("admins.name")}</Th>
                <Th>{t("admins.role")}</Th>
                <Th>{t("admins.state")}</Th>
                <Th>{t("admins.mfa")}</Th>
                <Th>{t("admins.created")}</Th>
              </tr>
            }
          >
            {query.data.items.length === 0 ? (
              <EmptyRow colSpan={6}>{t("common.noResults")}</EmptyRow>
            ) : (
              query.data.items.map((a) => (
                <tr key={a.id}>
                  <Td>
                    {a.email}{" "}
                    {a.id === me?.id ? (
                      <span className="text-slate-500">{t("admins.you")}</span>
                    ) : null}
                  </Td>
                  <Td>{personName(a.name)}</Td>
                  <Td>
                    <RoleCell
                      key={`${a.id}-${a.role}`}
                      admin={a}
                      isSelf={a.id === me?.id}
                      canManage={canManage}
                      onChange={(admin, role) => setPending({ admin, role })}
                    />
                  </Td>
                  <Td>
                    <StateBadge state={a.state} />
                  </Td>
                  <Td>{a.mfa_enrolled ? t("admins.yes") : t("admins.no")}</Td>
                  <Td>{formatDateTime(a.created_at, i18n.language)}</Td>
                </tr>
              ))
            )}
          </Table>
          <Pagination
            page={pager.page}
            hasPrevious={pager.hasPrevious}
            hasNext={!!query.data.next_page_token}
            onPrevious={pager.previous}
            onNext={() => pager.next(query.data.next_page_token)}
            busy={query.isFetching}
          />
        </div>
      )}

      <ConfirmDialog
        open={pending !== null}
        title={t("admins.changeRoleConfirmTitle")}
        description={
          pending
            ? t("admins.changeRoleConfirmBody", {
                email: pending.admin.email,
                role: t(`admins.roles.${pending.role}`),
              })
            : undefined
        }
        confirmLabel={t("admins.changeRole")}
        busy={changeRole.isPending}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (!pending) return;
          changeRole.mutate(
            { id: pending.admin.id, role: pending.role },
            {
              onSuccess: () => showToast("success", t("admins.roleChanged")),
              onSettled: () => setPending(null),
            },
          );
        }}
      />
    </div>
  );
}
