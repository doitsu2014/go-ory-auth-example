import { useTranslation } from "react-i18next";

import { Button } from "./Button";

export interface PaginationProps {
  page: number;
  hasPrevious: boolean;
  hasNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  busy?: boolean;
}

export function Pagination({
  page,
  hasPrevious,
  hasNext,
  onPrevious,
  onNext,
  busy,
}: PaginationProps) {
  const { t } = useTranslation();
  return (
    <nav aria-label={t("common.pagination")} className="mt-4 flex items-center justify-between">
      <span className="text-sm text-slate-600">{t("common.page", { page })}</span>
      <div className="flex gap-2">
        <Button variant="secondary" onClick={onPrevious} disabled={!hasPrevious || busy}>
          {t("common.previous")}
        </Button>
        <Button variant="secondary" onClick={onNext} disabled={!hasNext || busy}>
          {t("common.next")}
        </Button>
      </div>
    </nav>
  );
}
