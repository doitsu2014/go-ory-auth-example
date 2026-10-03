import { zodResolver } from "@hookform/resolvers/zod";
import { useId } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import {
  PII_FIELDS,
  type PiiField,
  REVEAL_REASON_CODES,
  type RevealReasonCode,
  type RevealRequest,
} from "../../api/client";
import { Alert } from "../../shared/ui/Alert";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { SelectField, TextField } from "../../shared/ui/Field";

/** Mirrors RevealRequest.ticket_ref in the OpenAPI contract. */
const TICKET_REF_PATTERN = /^[A-Z][A-Z0-9]{1,9}-[0-9]{1,8}$/;

const schema = z.object({
  reasonCode: z
    .string()
    .refine((v): v is RevealReasonCode => REVEAL_REASON_CODES.some((c) => c === v), "required"),
  ticketRef: z
    .string()
    .trim()
    .refine((v) => v === "" || TICKET_REF_PATTERN.test(v), "ticketRef"),
  fields: z.array(z.string()),
});

type FormValues = z.input<typeof schema>;
type FormOutput = z.output<typeof schema>;

const DEFAULTS: FormValues = { reasonCode: "", ticketRef: "", fields: [] };

export interface RevealDialogProps {
  open: boolean;
  /** Fields the customer has provided (only these can be selected). */
  available: readonly PiiField[];
  busy?: boolean;
  /** Localised error from the last attempt, shown inside the dialog. */
  error?: string;
  onConfirm: (body: RevealRequest) => void;
  onCancel: () => void;
}

/**
 * Reveal confirmation: a coded reason (no free text — the audit log is
 * append-only and must never hold PII), an optional ticket reference and an
 * optional subset of fields (none checked = all).
 */
export function RevealDialog(props: RevealDialogProps) {
  const { t } = useTranslation();
  const formId = useId();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<FormValues, unknown, FormOutput>({
    resolver: zodResolver(schema),
    defaultValues: DEFAULTS,
  });

  return (
    <ConfirmDialog
      open={props.open}
      title={t("pii.revealTitle")}
      description={t("pii.revealBody")}
      confirmLabel={t("pii.reveal")}
      danger
      busy={props.busy}
      onCancel={() => {
        reset(DEFAULTS);
        props.onCancel();
      }}
      onConfirm={() =>
        void handleSubmit((v) => {
          const body: RevealRequest = { reason_code: v.reasonCode };
          const ticket = v.ticketRef.trim();
          if (ticket) body.ticket_ref = ticket;
          const fields = PII_FIELDS.filter((f) => v.fields.includes(f));
          if (fields.length > 0) body.fields = fields;
          props.onConfirm(body);
        })()
      }
    >
      {props.error ? <Alert kind="error">{props.error}</Alert> : null}
      <SelectField
        id={`${formId}-reason`}
        label={t("pii.reasonCode")}
        required
        error={errors.reasonCode ? t("validation.required") : undefined}
        {...register("reasonCode")}
      >
        <option value="">{t("pii.selectReason")}</option>
        {REVEAL_REASON_CODES.map((c) => (
          <option key={c} value={c}>
            {t(`pii.reasonCodes.${c}`)}
          </option>
        ))}
      </SelectField>
      <TextField
        id={`${formId}-ticket`}
        label={t("pii.ticketRef")}
        hint={t("pii.ticketRefHint")}
        error={errors.ticketRef ? t("pii.ticketRefInvalid") : undefined}
        autoComplete="off"
        {...register("ticketRef")}
      />
      {props.available.length > 0 ? (
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium text-slate-700">{t("pii.fieldsToReveal")}</legend>
          <p className="text-xs text-slate-500">{t("pii.fieldsHint")}</p>
          {props.available.map((f) => (
            <label key={f} className="flex items-center gap-2 text-sm text-slate-800">
              <input
                type="checkbox"
                value={f}
                className="h-4 w-4 rounded border-slate-300"
                {...register("fields")}
              />
              {t(`pii.fields.${f}`)}
            </label>
          ))}
        </fieldset>
      ) : null}
    </ConfirmDialog>
  );
}
