import { useQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useEffect, useId } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import type { IdentityState } from "../../api/client";
import { formatDateTime, formString } from "../../shared/format";
import { useCursorPagination } from "../../shared/useCursorPagination";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { PageHeader } from "../../shared/ui/Card";
import { SelectField } from "../../shared/ui/Field";
import { Pagination } from "../../shared/ui/Pagination";
import { Spinner } from "../../shared/ui/Spinner";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { problemMessage } from "../../shared/problem";
import { LoginLookup } from "./LoginLookup";
import { LoginValue } from "./LoginValue";
import { PhoneLookup } from "./PhoneLookup";
import { StateBadge } from "./StateBadge";
import { customerListQuery } from "./queries";

function parseState(v: string | null): IdentityState | undefined {
  return v === "active" || v === "inactive" ? v : undefined;
}

/**
 * /customers — the state filter lives in the URL; cursor pagination via
 * next_page_token. Contact search (login / phone) is a POST-body lookup so no
 * PII ever reaches the URL (PLI-FR-11).
 */
export function CustomersPage() {
  const { t, i18n } = useTranslation();
  const [params, setParams] = useSearchParams();
  const state = parseState(params.get("state"));
  const pager = useCursorPagination();
  const formId = useId();

  // The old `?email=` filter is gone (PLI-FR-11): drop it from stale
  // bookmarks so the address does not linger in the URL / history.
  useEffect(() => {
    if (!params.has("email")) return;
    const next = new URLSearchParams(params);
    next.delete("email");
    setParams(next, { replace: true });
  }, [params, setParams]);

  const query = useQuery(customerListQuery({ state, pageToken: pager.pageToken }));

  function onFilter(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const next = new URLSearchParams();
    const st = parseState(formString(data, "state"));
    if (st) next.set("state", st);
    pager.reset();
    setParams(next);
  }

  return (
    <div>
      <PageHeader title={t("customers.title")} />
      <LoginLookup />
      <PhoneLookup />
      <form
        key={state ?? ""}
        onSubmit={onFilter}
        className="mb-4 grid gap-3 sm:grid-cols-[200px_auto] sm:items-end"
        aria-label={t("customers.title")}
      >
        <SelectField
          id={`${formId}-state`}
          name="state"
          label={t("customers.filterState")}
          defaultValue={state ?? ""}
        >
          <option value="">{t("customers.allStates")}</option>
          <option value="active">{t("customers.states.active")}</option>
          <option value="inactive">{t("customers.states.inactive")}</option>
        </SelectField>
        <Button type="submit">{t("common.apply")}</Button>
      </form>

      {query.isError ? (
        <Alert kind="error">{problemMessage(t, query.error)}</Alert>
      ) : query.isPending ? (
        <Spinner />
      ) : (
        <>
          <Table
            caption={t("customers.title")}
            head={
              <tr>
                <Th>{t("customers.login")}</Th>
                <Th>{t("customers.displayName")}</Th>
                <Th>{t("customers.state")}</Th>
                <Th>{t("customers.created")}</Th>
                <Th>
                  <span className="sr-only">{t("common.actions")}</span>
                </Th>
              </tr>
            }
          >
            {query.data.items.length === 0 ? (
              <EmptyRow colSpan={5}>{t("common.noResults")}</EmptyRow>
            ) : (
              query.data.items.map((c) => (
                <tr key={c.id}>
                  <Td>
                    <LoginValue login={c.login} unavailable={c.login_unavailable} />
                  </Td>
                  <Td>{c.display_name ?? "—"}</Td>
                  <Td>
                    <StateBadge state={c.state} />
                  </Td>
                  <Td>{formatDateTime(c.created_at, i18n.language)}</Td>
                  <Td>
                    <Link
                      to={`/customers/${c.id}`}
                      className="text-blue-700 underline"
                      aria-label={`${t("customers.view")} ${c.id}`}
                    >
                      {t("customers.view")}
                    </Link>
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
