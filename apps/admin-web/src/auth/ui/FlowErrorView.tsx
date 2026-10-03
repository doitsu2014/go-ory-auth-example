import { useTranslation } from "react-i18next";

import { kratosErrorText } from "../../i18n/kratos";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import type { FlowErrorAction } from "../kratosErrors";

/** Unrecoverable flow error: message by Kratos error id (fallback reason) + retry with a fresh flow. */
export function FlowErrorView({
  error,
  onRetry,
}: {
  error: Extract<FlowErrorAction, { type: "error" }>;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <Alert kind="error">{kratosErrorText(t, { id: error.id, reason: error.message })}</Alert>
      <Button variant="secondary" onClick={onRetry}>
        {t("common.retry")}
      </Button>
    </div>
  );
}
