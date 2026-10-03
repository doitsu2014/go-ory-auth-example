import { useQuery } from "@tanstack/react-query";
import type { SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";

import { formatDateTime, formString } from "../../shared/format";
import { problemMessage } from "../../shared/problem";
import { useCursorPagination } from "../../shared/useCursorPagination";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { PageHeader } from "../../shared/ui/Card";
import { TextField } from "../../shared/ui/Field";
import { Pagination } from "../../shared/ui/Pagination";
import { Spinner } from "../../shared/ui/Spinner";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { auditListQuery } from "./queries";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** /audit — audit events (newest first) with filters in the URL and cursor pagination. */
export function AuditPage() {
  const { t, i18n } = useTranslation();
  const [params, setParams] = useSearchParams();
  const filters = {
    targetType: params.get("target_type") ?? "",
    targetId: params.get("target_id") ?? "",
    actorId: params.get("actor_id") ?? "",
  };
  const actorInvalid = !!filters.actorId && !UUID.test(filters.actorId);
  const pager = useCursorPagination();
  const query = useQuery({
    ...auditListQuery(filters, pager.pageToken),
    enabled: !actorInvalid,
  });

  function onFilter(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const next = new URLSearchParams();
    for (const key of ["target_type", "target_id", "actor_id"]) {
      const v = formString(data, key).trim();
      if (v) next.set(key, v);
    }
    pager.reset();
    setParams(next);
  }

  return (
    <div>
      <PageHeader title={t("audit.title")} />
      <form
        key={params.toString()}
        onSubmit={onFilter}
        aria-label={t("audit.title")}
        className="mb-4 grid gap-3 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end"
      >
        <TextField
          name="target_type"
          label={t("audit.filterTargetType")}
          defaultValue={filters.targetType}
        />
        <TextField
          name="target_id"
          label={t("audit.filterTargetId")}
          defaultValue={filters.targetId}
        />
        <TextField
          name="actor_id"
          label={t("audit.filterActorId")}
          defaultValue={filters.actorId}
          error={actorInvalid ? t("validation.uuid") : undefined}
        />
        <Button type="submit">{t("common.apply")}</Button>
      </form>

      {actorInvalid ? null : query.isError ? (
        <Alert kind="error">{problemMessage(t, query.error)}</Alert>
      ) : query.isPending ? (
        <Spinner />
      ) : (
        <>
          <Table
            caption={t("audit.title")}
            head={
              <tr>
                <Th>{t("audit.occurredAt")}</Th>
                <Th>{t("audit.action")}</Th>
                <Th>{t("audit.actor")}</Th>
                <Th>{t("audit.targetType")}</Th>
                <Th>{t("audit.targetId")}</Th>
                <Th>{t("audit.requestId")}</Th>
                <Th>{t("audit.details")}</Th>
              </tr>
            }
          >
            {query.data.items.length === 0 ? (
              <EmptyRow colSpan={7}>{t("common.noResults")}</EmptyRow>
            ) : (
              query.data.items.map((ev) => (
                <tr key={ev.id}>
                  <Td>{formatDateTime(ev.occurred_at, i18n.language)}</Td>
                  <Td>
                    <code>{ev.action}</code>
                  </Td>
                  <Td>
                    <code className="text-xs">{ev.actor_id}</code>
                  </Td>
                  <Td>{ev.target_type}</Td>
                  <Td>
                    <code className="text-xs">{ev.target_id}</code>
                  </Td>
                  <Td>
                    <code className="text-xs">{ev.request_id}</code>
                  </Td>
                  <Td>
                    <pre className="max-w-xs overflow-x-auto text-xs whitespace-pre-wrap">
                      {JSON.stringify(ev.details, null, 2)}
                    </pre>
                  </Td>
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
        </>
      )}
    </div>
  );
}
