import { useTranslation } from "react-i18next";

import type { LoginType } from "../../api/client";

/** Small type marker ("Email" / "Phone") shown before a login value. */
export function LoginTypeLabel({ type }: { type: LoginType }) {
  const { t } = useTranslation();
  return (
    <span className="mr-2 inline-flex items-center rounded bg-slate-100 px-1.5 py-0.5 text-xs font-medium text-slate-700">
      <svg
        aria-hidden="true"
        viewBox="0 0 16 16"
        className="mr-1 h-3 w-3"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
      >
        {type === "email" ? (
          <>
            <rect x="1.5" y="3.5" width="13" height="9" rx="1" />
            <path d="M2 4.5l6 4.5 6-4.5" />
          </>
        ) : (
          <>
            <rect x="4.5" y="1.5" width="7" height="13" rx="1" />
            <path d="M7 12.5h2" />
          </>
        )}
      </svg>
      {t(`customers.loginTypes.${type}`)}
    </span>
  );
}

export interface LoginValueProps {
  login?: { type: LoginType; masked: string } | null;
  /** The key manager was unavailable, so the login could not be read. */
  unavailable?: boolean;
}

/**
 * Masked login identifier (PLI-FR-10): type marker + masked value, a neutral
 * "unavailable" marker when the key manager is down, or a dash when absent.
 */
export function LoginValue({ login, unavailable }: LoginValueProps) {
  const { t } = useTranslation();
  if (login) {
    return (
      <span className="inline-flex items-center">
        <LoginTypeLabel type={login.type} />
        <span className="font-mono">{login.masked}</span>
      </span>
    );
  }
  if (unavailable) {
    return (
      <span className="text-slate-500 italic" data-testid="login-unavailable">
        {t("customers.loginUnavailable")}
      </span>
    );
  }
  return <>—</>;
}
