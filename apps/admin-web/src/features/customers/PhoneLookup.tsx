import { type SyntheticEvent, useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import type { CustomerLookupItem } from "../../api/client";
import { useAuthRedirect } from "../../auth/useAuthRedirect";
import { formString } from "../../shared/format";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { TextField } from "../../shared/ui/Field";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { piiErrorMessage } from "./piiErrors";
import { lookupCustomersByPhone } from "./queries";
import { StateBadge } from "./StateBadge";

/** E.164 after stripping the separators the server also strips. */
const E164 = /^\+[1-9][0-9]{7,14}$/; // PII-FR-03

function normalizePhone(raw: string): string {
  return raw.replace(/[\s.-]/g, "");
}

/**
 * Phone lookup on the customers page. The number is sent in the POST body
 * only — never the URL, query string, router state, query cache, web storage
 * or logs — and results (masked) live in component state.
 */
export function PhoneLookup() {
  const { t } = useTranslation();
  const formId = useId();
  const redirectOnAuthError = useAuthRedirect();
  const [items, setItems] = useState<CustomerLookupItem[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [invalid, setInvalid] = useState(false);

  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  async function onSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const phone = normalizePhone(formString(new FormData(e.currentTarget), "phone"));
    if (!E164.test(phone)) {
      // Do not leave results for a previous number next to a rejected one.
      setItems(null);
      setTruncated(false);
      setError(undefined);
      setInvalid(true);
      return;
    }
    setInvalid(false);
    setError(undefined);
    setBusy(true);
    try {
      const res = await lookupCustomersByPhone(phone);
      if (!mounted.current) return;
      setItems(res.items);
      setTruncated(res.truncated);
    } catch (err) {
      if (!mounted.current || redirectOnAuthError(err)) return;
      setItems(null);
      setTruncated(false);
      setError(piiErrorMessage(t, err, "lookup"));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }

  return (
    <Card className="mb-6">
      <h2 className="text-lg font-semibold text-slate-900">{t("pii.lookup.title")}</h2>
      <p className="mb-3 text-xs text-slate-500">{t("pii.lookup.unverifiedNote")}</p>
      {/* method=post: even without JS the number never lands in a query string. */}
      <form
        method="post"
        noValidate
        onSubmit={(e) => void onSubmit(e)}
        className="mb-4 grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end"
        aria-label={t("pii.lookup.title")}
      >
        <TextField
          id={`${formId}-phone`}
          name="phone"
          type="tel"
          inputMode="tel"
          label={t("pii.lookup.phone")}
          hint={t("pii.lookup.phoneHint")}
          error={invalid ? t("pii.lookup.invalid") : undefined}
          autoComplete="off"
          required
        />
        <Button type="submit" disabled={busy}>
          {t("pii.lookup.search")}
        </Button>
      </form>

      {error ? <Alert kind="error">{error}</Alert> : null}
      {items && truncated ? (
        <div className="mb-4">
          <Alert kind="warning">{t("pii.lookup.truncated")}</Alert>
        </div>
      ) : null}
      {items ? (
        <Table
          caption={t("pii.lookup.results")}
          head={
            <tr>
              <Th>{t("customers.id")}</Th>
              <Th>{t("customers.state")}</Th>
              <Th>{t("pii.fields.phone_number")}</Th>
              <Th>{t("pii.fields.date_of_birth")}</Th>
              <Th>{t("pii.fields.address")}</Th>
              <Th>
                <span className="sr-only">{t("common.actions")}</span>
              </Th>
            </tr>
          }
        >
          {items.length === 0 ? (
            <EmptyRow colSpan={6}>{t("pii.lookup.noMatches")}</EmptyRow>
          ) : (
            items.map((c) => (
              <tr key={c.id}>
                <Td>
                  <code>{c.id}</code>
                </Td>
                <Td>{c.state ? <StateBadge state={c.state} /> : "—"}</Td>
                <Td>
                  <span className="font-mono">{c.personal_info.phone_number ?? "—"}</span>
                </Td>
                <Td>{c.personal_info.date_of_birth ?? "—"}</Td>
                <Td>
                  {c.personal_info.address
                    ? [c.personal_info.address.city, c.personal_info.address.country]
                        .filter(Boolean)
                        .join(", ")
                    : "—"}
                </Td>
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
      ) : null}
    </Card>
  );
}
