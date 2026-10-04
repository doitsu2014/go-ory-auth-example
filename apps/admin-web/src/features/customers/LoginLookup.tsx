import { type SyntheticEvent, useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import type { CustomerLookupItem, LoginIdentifier } from "../../api/client";
import { useAuthRedirect } from "../../auth/useAuthRedirect";
import { formString } from "../../shared/format";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { TextField } from "../../shared/ui/Field";
import { EmptyRow, Table, Td, Th } from "../../shared/ui/Table";
import { LoginValue } from "./LoginValue";
import { piiErrorMessage } from "./piiErrors";
import { lookupCustomersByLogin } from "./queries";
import { StateBadge } from "./StateBadge";

/**
 * The login type is auto-detected: anything with "@" is an email, the rest a
 * phone number. The server normalises and validates the value (422 if bad).
 */
function detectLogin(raw: string): LoginIdentifier | null {
  const value = raw.trim();
  if (!value) return null;
  return { type: value.includes("@") ? "email" : "phone", value };
}

/**
 * Lookup by the email or phone the customer signs in with (exact match,
 * PLI-FR-11). Replaces the old `?email=` list filter: the value is sent in the
 * POST body only — never the URL, query string, router state, TanStack caches,
 * web storage or logs — and the (masked) results live in component state.
 */
export function LoginLookup() {
  const { t } = useTranslation();
  const formId = useId();
  const redirectOnAuthError = useAuthRedirect();
  const [items, setItems] = useState<CustomerLookupItem[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const [invalid, setInvalid] = useState(false);

  // Bumped on every edit and search: a response for an older value is dropped.
  const attempt = useRef(0);

  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  async function onSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const login = detectLogin(formString(new FormData(e.currentTarget), "login"));
    if (!login) {
      setItems(null);
      setError(undefined);
      setInvalid(true);
      return;
    }
    setInvalid(false);
    setError(undefined);
    setBusy(true);
    const current = ++attempt.current;
    try {
      const res = await lookupCustomersByLogin(login);
      if (!mounted.current || current !== attempt.current) return;
      setItems(res.items);
    } catch (err) {
      if (!mounted.current || redirectOnAuthError(err) || current !== attempt.current) return;
      // Do not leave results for a previous value next to a failed one.
      setItems(null);
      setError(piiErrorMessage(t, err, "loginLookup"));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }

  /** Results (and errors) describe the previous value: drop them on edit. */
  function onValueChange() {
    attempt.current++;
    setItems(null);
    setError(undefined);
  }

  return (
    <Card className="mb-6">
      <h2 className="text-lg font-semibold text-slate-900">{t("customers.loginLookup.title")}</h2>
      {/* method=post: even without JS the value never lands in a query string. */}
      <form
        method="post"
        noValidate
        onSubmit={(e) => void onSubmit(e)}
        className="mb-4 grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end"
        aria-label={t("customers.loginLookup.title")}
      >
        <TextField
          id={`${formId}-login`}
          name="login"
          type="text"
          label={t("customers.loginLookup.value")}
          hint={t("customers.loginLookup.hint")}
          error={invalid ? t("validation.required") : undefined}
          autoComplete="off"
          spellCheck={false}
          required
          onChange={onValueChange}
        />
        <Button type="submit" disabled={busy}>
          {t("customers.loginLookup.search")}
        </Button>
      </form>

      {error ? <Alert kind="error">{error}</Alert> : null}
      {items ? (
        <Table
          caption={t("customers.loginLookup.results")}
          head={
            <tr>
              <Th>{t("customers.id")}</Th>
              <Th>{t("customers.state")}</Th>
              <Th>{t("customers.login")}</Th>
              <Th>
                <span className="sr-only">{t("common.actions")}</span>
              </Th>
            </tr>
          }
        >
          {items.length === 0 ? (
            <EmptyRow colSpan={4}>{t("customers.loginLookup.noMatches")}</EmptyRow>
          ) : (
            items.map((c) => (
              <tr key={c.id}>
                <Td>
                  <code>{c.id}</code>
                </Td>
                <Td>{c.state ? <StateBadge state={c.state} /> : "—"}</Td>
                <Td>
                  <LoginValue login={c.login} />
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
