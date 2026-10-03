import { useTranslation } from "react-i18next";

import { dismissToast, useToasts } from "../toast";
import { cx } from "./cx";

const styles = {
  success: "bg-green-700",
  error: "bg-red-700",
  info: "bg-slate-800",
} as const;

export function Toaster() {
  const { t } = useTranslation();
  const toasts = useToasts();
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed right-4 bottom-4 z-50 flex w-80 flex-col gap-2"
    >
      {toasts.map((toast) => (
        <div
          key={toast.id}
          role={toast.kind === "error" ? "alert" : "status"}
          className={cx(
            "pointer-events-auto flex items-start justify-between gap-3 rounded-md px-4 py-3 text-sm text-white shadow-lg",
            styles[toast.kind],
          )}
        >
          <span>{toast.message}</span>
          <button
            type="button"
            onClick={() => dismissToast(toast.id)}
            className="text-white/80 hover:text-white"
            aria-label={t("common.dismiss")}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
