import { useTranslation } from "react-i18next";

import type { IdentityState } from "../../api/client";
import { cx } from "../../shared/ui/cx";

export function StateBadge({ state }: { state: IdentityState }) {
  const { t } = useTranslation();
  return (
    <span
      className={cx(
        "inline-flex rounded-full px-2 py-0.5 text-xs font-medium",
        state === "active" ? "bg-green-100 text-green-800" : "bg-slate-200 text-slate-700",
      )}
    >
      {t(`customers.states.${state}`)}
    </span>
  );
}
