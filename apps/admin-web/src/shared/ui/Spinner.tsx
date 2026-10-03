import { useTranslation } from "react-i18next";

export function Spinner({ label }: { label?: string }) {
  const { t } = useTranslation();
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center gap-2 p-4 text-sm text-slate-600"
    >
      <span
        aria-hidden="true"
        className="h-4 w-4 animate-spin rounded-full border-2 border-slate-300 border-t-blue-700"
      />
      {label ?? t("common.loading")}
    </div>
  );
}
