import { zodResolver } from "@hookform/resolvers/zod";
import type { ReactNode } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { ConfirmDialog } from "../../shared/ui/Dialog";
import { TextAreaField } from "../../shared/ui/Field";

const MAX_REASON = 500;

function schema(required: boolean) {
  const base = z.string().trim().max(MAX_REASON, "tooLong");
  return z.object({ reason: required ? base.min(1, "required") : base });
}

export interface ReasonDialogProps {
  open: boolean;
  title: ReactNode;
  description: ReactNode;
  confirmLabel: ReactNode;
  requireReason: boolean;
  danger?: boolean;
  busy?: boolean;
  onConfirm: (reason: string) => void;
  onCancel: () => void;
}

/** Confirmation dialog with an audited "reason" field (React Hook Form + Zod). */
export function ReasonDialog(props: ReasonDialogProps) {
  const { t } = useTranslation();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<{ reason: string }>({
    resolver: zodResolver(schema(props.requireReason)),
    defaultValues: { reason: "" },
  });

  const errorKey = errors.reason?.message;
  const error =
    errorKey === "tooLong"
      ? t("validation.tooLong", { max: MAX_REASON })
      : errorKey
        ? t("validation.required")
        : undefined;

  return (
    <ConfirmDialog
      open={props.open}
      title={props.title}
      description={props.description}
      confirmLabel={props.confirmLabel}
      danger={props.danger}
      busy={props.busy}
      onCancel={() => {
        reset();
        props.onCancel();
      }}
      onConfirm={() =>
        void handleSubmit(({ reason }) => {
          props.onConfirm(reason.trim());
          reset();
        })()
      }
    >
      <TextAreaField
        label={t("customers.reason")}
        hint={t("customers.reasonHint")}
        error={error}
        rows={3}
        required={props.requireReason}
        {...register("reason")}
      />
    </ConfirmDialog>
  );
}
