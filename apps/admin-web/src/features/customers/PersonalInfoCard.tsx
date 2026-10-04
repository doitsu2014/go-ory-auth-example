import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  type MaskedPersonalInfo,
  PII_FIELDS,
  type PersonalInfo,
  type PiiField,
  REVEAL_FIELDS,
  type RevealField,
  type RevealRequest,
  type RevealedPersonalInfo,
} from "../../api/client";
import { useCan } from "../../auth/me";
import { useAuthRedirect } from "../../auth/useAuthRedirect";
import { localizedName } from "../../shared/format";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";
import { auditKeys } from "../audit/queries";
import { LoginTypeLabel } from "./LoginValue";
import { piiErrorMessage } from "./piiErrors";
import { maskedPersonalInfoQuery, revealPersonalInfo } from "./queries";
import { RevealDialog } from "./RevealDialog";

/** Revealed values are hidden again after this long. */
export const REVEAL_TTL_MS = 60_000;

const HAS: Record<PiiField, keyof MaskedPersonalInfo> = {
  name: "has_name",
  phone_number: "has_phone_number",
  date_of_birth: "has_date_of_birth",
  address: "has_address",
  national_id: "has_national_id",
};

interface Revealed {
  info: RevealedPersonalInfo;
  fields: readonly RevealField[];
}

function join(parts: readonly (string | null | undefined)[]): string {
  return parts.filter(Boolean).join(", ");
}

type Fmt = { idType: (s: string) => string; lng: string };

function maskedValue(m: MaskedPersonalInfo, f: PiiField, { idType, lng }: Fmt) {
  switch (f) {
    case "name":
      return localizedName(m.name, lng);
    case "phone_number":
      return m.phone_number ?? "";
    case "date_of_birth":
      return m.date_of_birth ?? "";
    case "address":
      return m.address ? join([m.address.city, m.address.country]) : "";
    case "national_id":
      return m.national_id
        ? join([m.national_id.type ? idType(m.national_id.type) : "", m.national_id.number])
        : "";
  }
}

function revealedValue(p: PersonalInfo, f: PiiField, { idType, lng }: Fmt) {
  switch (f) {
    case "name":
      return localizedName(p.name, lng);
    case "phone_number":
      return p.phone_number ?? "";
    case "date_of_birth":
      return p.date_of_birth ?? "";
    case "address":
      return p.address
        ? join([
            p.address.line1,
            p.address.line2,
            p.address.city,
            p.address.region,
            p.address.postal_code,
            p.address.country,
          ])
        : "";
    case "national_id":
      return p.national_id ? join([idType(p.national_id.type), p.national_id.number]) : "";
  }
}

/**
 * Customer personal information on the detail page. Shows the masked view
 * (cached like any other query) and, for admins with `reveal_customer_pii`,
 * a Reveal action that can also reveal the login identifier (`login`,
 * PLI-FR-12; its masked form is on the account card). Revealed plaintext
 * lives ONLY in this component's state:
 * it is never written to the TanStack caches, URL, router state, web storage
 * or logs, is hidden after REVEAL_TTL_MS and is dropped on unmount.
 * Render with `key={customerId}` so switching customers starts clean.
 */
export function PersonalInfoCard({
  customerId,
  hasLogin = false,
}: {
  customerId: string;
  /** The customer has a readable login identifier that can be revealed. */
  hasLogin?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const canReveal = useCan("reveal_customer_pii");
  const redirectOnAuthError = useAuthRedirect();
  const query = useQuery(maskedPersonalInfoQuery(customerId));

  const [revealed, setRevealed] = useState<Revealed | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();

  // A reveal that resolves after unmount must not resurrect the data.
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  // Auto-hide; the cleanup also cancels the timer on unmount / manual hide.
  useEffect(() => {
    if (!revealed) return;
    const timer = setTimeout(() => setRevealed(null), REVEAL_TTL_MS);
    return () => clearTimeout(timer);
  }, [revealed]);

  const fmt: Fmt = {
    idType: (type: string) => t(`pii.nationalIdTypes.${type}`, { defaultValue: type }),
    lng: i18n.language,
  };

  async function onReveal(body: RevealRequest) {
    setBusy(true);
    setError(undefined);
    try {
      const info = await revealPersonalInfo(customerId, body);
      if (!mounted.current) return;
      setRevealed({ info, fields: body.fields ?? REVEAL_FIELDS });
      setDialogOpen(false);
      // The reveal wrote an audit event (no PII is cached by this).
      void qc.invalidateQueries({ queryKey: auditKeys.all });
    } catch (err) {
      if (!mounted.current || redirectOnAuthError(err)) return;
      setError(piiErrorMessage(t, err, "reveal"));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }

  let body: ReactNode;
  if (query.isPending) {
    body = <Spinner />;
  } else if (query.isError) {
    body = <Alert kind="error">{piiErrorMessage(t, query.error, "masked")}</Alert>;
  } else {
    const m = query.data;
    const available: RevealField[] = [
      ...(hasLogin ? (["login"] as const) : []),
      ...PII_FIELDS.filter((f) => m[HAS[f]] === true),
    ];
    // `undefined` = not revealed (masked form is on the account card); `null` = revealed but absent.
    const revealedLogin =
      hasLogin && revealed?.fields.includes("login") ? (revealed.info.login ?? null) : undefined;
    body = (
      <>
        <dl className="grid gap-4 sm:grid-cols-2">
          {revealedLogin !== undefined ? (
            <div>
              <dt className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                {t("pii.fields.login")}
              </dt>
              <dd className="mt-1 text-sm text-slate-900">
                {revealedLogin ? (
                  <span className="inline-flex items-center">
                    <LoginTypeLabel type={revealedLogin.type} />
                    <span className="font-mono" data-testid="pii-revealed-login">
                      {revealedLogin.value}
                    </span>
                  </span>
                ) : (
                  <span data-testid="pii-revealed-login">—</span>
                )}
              </dd>
            </div>
          ) : null}
          {PII_FIELDS.map((f) => {
            let value: ReactNode;
            if (m[HAS[f]] !== true) {
              value = <span className="text-slate-500 italic">{t("pii.notProvided")}</span>;
            } else if (revealed?.fields.includes(f)) {
              value = (
                <span className="font-mono" data-testid={`pii-revealed-${f}`}>
                  {revealedValue(revealed.info, f, fmt) || "—"}
                </span>
              );
            } else {
              const masked = maskedValue(m, f, fmt);
              value = masked ? (
                <span className="font-mono">{masked}</span>
              ) : (
                <span className="text-slate-500 italic">{t("pii.hidden")}</span>
              );
            }
            return (
              <div key={f}>
                <dt className="text-xs font-medium tracking-wide text-slate-500 uppercase">
                  {t(`pii.fields.${f}`)}
                </dt>
                <dd className="mt-1 text-sm text-slate-900">{value}</dd>
              </div>
            );
          })}
        </dl>
        {revealed ? (
          <div className="mt-4">
            <Alert kind="warning">
              {t("pii.revealedNotice", { seconds: REVEAL_TTL_MS / 1000 })}
            </Alert>
          </div>
        ) : null}
        {dialogOpen ? (
          <RevealDialog
            open
            available={available}
            busy={busy}
            error={error}
            onCancel={() => {
              setDialogOpen(false);
              setError(undefined);
            }}
            onConfirm={(b) => void onReveal(b)}
          />
        ) : null}
      </>
    );
  }

  const hasAny = query.data
    ? hasLogin || PII_FIELDS.some((f) => query.data[HAS[f]] === true)
    : false;

  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-slate-900">{t("pii.title")}</h2>
          <p className="text-xs text-slate-500">{t("pii.maskedHint")}</p>
        </div>
        {canReveal && hasAny ? (
          revealed ? (
            <Button variant="secondary" onClick={() => setRevealed(null)}>
              {t("pii.hide")}
            </Button>
          ) : (
            <Button variant="secondary" onClick={() => setDialogOpen(true)}>
              {t("pii.reveal")}
            </Button>
          )
        ) : null}
      </div>
      {body}
    </Card>
  );
}
