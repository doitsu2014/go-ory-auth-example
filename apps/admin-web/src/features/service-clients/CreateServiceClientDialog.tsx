import { zodResolver } from "@hookform/resolvers/zod";
import { useId, useRef } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { type CreateServiceClientRequest, MACHINE_SCOPES } from "../../api/client";
import { Alert } from "../../shared/ui/Alert";
import { ConfirmDialog } from "../../shared/ui/Dialog";
import { TextField } from "../../shared/ui/Field";
import { SCOPE_KEY } from "./scopes";

/** Mirrors CreateServiceClientRequest.name in the OpenAPI contract. */
const NAME_PATTERN = /^[a-z0-9][a-z0-9-]*$/;
const NAME_MIN = 3;
const NAME_MAX = 64;
const OWNER_MAX = 254;

const schema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "required")
    .min(NAME_MIN, "tooShort")
    .max(NAME_MAX, "tooLong")
    .regex(NAME_PATTERN, "namePattern"),
  owner: z.string().trim().min(1, "required").max(OWNER_MAX, "tooLong").pipe(z.email("email")),
  scopes: z.array(z.enum(["customers:read", "audit:read"])).min(1, "scopesRequired"),
});

type FormValues = z.input<typeof schema>;
type FormOutput = z.output<typeof schema>;

const DEFAULTS: FormValues = { name: "", owner: "", scopes: [] };

export interface CreateServiceClientDialogProps {
  busy?: boolean;
  /** Localised error from the last attempt, shown inside the dialog. */
  error?: string;
  /** `idempotencyKey` is reused while the submitted values are unchanged. */
  onConfirm: (body: CreateServiceClientRequest, idempotencyKey: string) => void;
  onCancel: () => void;
}

/**
 * Create form (react-hook-form + zod mirroring the server rules). Render only
 * while open. Sends a UUID `Idempotency-Key`, reused on unchanged retries.
 */
export function CreateServiceClientDialog(props: CreateServiceClientDialogProps) {
  const { t } = useTranslation();
  const formId = useId();
  // One key per opened dialog and request body: a retry with unchanged values
  // reuses it; edited values after a failed attempt get a new one (the server
  // binds the key to the request hash). The dialog unmounts on success or close,
  // so the next open starts fresh. A replay of a completed create returns no secret.
  const attempt = useRef<{ key: string; body: string } | null>(null);
  function keyFor(body: CreateServiceClientRequest): string {
    const json = JSON.stringify(body);
    if (attempt.current?.body !== json) {
      attempt.current = { key: crypto.randomUUID(), body: json };
    }
    return attempt.current.key;
  }
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues, unknown, FormOutput>({
    resolver: zodResolver(schema),
    defaultValues: DEFAULTS,
  });

  function err(key: string | undefined, max?: number): string | undefined {
    if (!key) return undefined;
    if (key === "tooLong") return t("validation.tooLong", { max });
    if (key === "tooShort") return t("validation.tooShort", { min: NAME_MIN });
    if (key === "email") return t("validation.email");
    if (key === "namePattern") return t("serviceClients.nameInvalid");
    if (key === "scopesRequired") return t("serviceClients.scopesRequired");
    return t("validation.required");
  }

  const scopesError = err(errors.scopes?.message);

  return (
    <ConfirmDialog
      open
      title={t("serviceClients.createTitle")}
      description={t("serviceClients.createBody")}
      confirmLabel={t("serviceClients.create")}
      busy={props.busy}
      onCancel={props.onCancel}
      onConfirm={() =>
        void handleSubmit((v) => {
          const body: CreateServiceClientRequest = {
            name: v.name,
            owner: v.owner,
            scopes: MACHINE_SCOPES.filter((s) => v.scopes.includes(s)),
          };
          props.onConfirm(body, keyFor(body));
        })()
      }
    >
      {props.error ? <Alert kind="error">{props.error}</Alert> : null}
      <TextField
        id={`${formId}-name`}
        label={t("serviceClients.name")}
        hint={t("serviceClients.nameHint")}
        required
        autoComplete="off"
        error={err(errors.name?.message, NAME_MAX)}
        {...register("name")}
      />
      <TextField
        id={`${formId}-owner`}
        label={t("serviceClients.owner")}
        hint={t("serviceClients.ownerHint")}
        type="email"
        required
        autoComplete="off"
        error={err(errors.owner?.message, OWNER_MAX)}
        {...register("owner")}
      />
      <fieldset
        className="space-y-2"
        aria-describedby={scopesError ? `${formId}-scopes-error` : undefined}
      >
        <legend className="text-sm font-medium text-slate-700">{t("serviceClients.scopes")}</legend>
        {MACHINE_SCOPES.map((s) => (
          <label key={s} className="flex items-start gap-2 text-sm text-slate-800">
            <input
              type="checkbox"
              value={s}
              className="mt-0.5 h-4 w-4 rounded border-slate-300"
              {...register("scopes")}
            />
            <span>
              <span className="font-mono">{s}</span>
              <span className="block text-xs text-slate-500">
                {t(`serviceClients.scopeDescriptions.${SCOPE_KEY[s]}`)}
              </span>
            </span>
          </label>
        ))}
        {scopesError ? (
          <p id={`${formId}-scopes-error`} className="text-xs text-red-700" role="alert">
            {scopesError}
          </p>
        ) : null}
      </fieldset>
    </ConfirmDialog>
  );
}
