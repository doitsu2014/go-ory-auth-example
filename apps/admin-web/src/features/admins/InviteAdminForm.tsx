import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { ROLES, type Role } from "../../api/client";
import { showToast } from "../../shared/toast";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { SelectField, TextField } from "../../shared/ui/Field";
import { useInviteAdmin } from "./queries";

const inviteSchema = z.object({
  email: z.string().trim().min(1, "required").max(320, "tooLong").pipe(z.email("email")),
  first: z.string().trim().max(100, "tooLong"),
  last: z.string().trim().max(100, "tooLong"),
  role: z.enum(["support", "admin", "super_admin"]),
});

type InviteForm = z.infer<typeof inviteSchema>;

function newKey(): string {
  return crypto.randomUUID();
}

/** Invite form. Sends a UUID `Idempotency-Key`, regenerated only after a successful invite. */
export function InviteAdminForm() {
  const { t } = useTranslation();
  const invite = useInviteAdmin();
  const [idempotencyKey, setIdempotencyKey] = useState(newKey);
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<InviteForm>({
    resolver: zodResolver(inviteSchema),
    defaultValues: { email: "", first: "", last: "", role: "support" },
  });

  function err(key: string | undefined, max?: number): string | undefined {
    if (!key) return undefined;
    if (key === "tooLong") return t("validation.tooLong", { max });
    if (key === "email") return t("validation.email");
    return t("validation.required");
  }

  const onSubmit = handleSubmit((v) => {
    const name =
      v.first || v.last ? { first: v.first || undefined, last: v.last || undefined } : undefined;
    invite.mutate(
      { body: { email: v.email, role: v.role, ...(name ? { name } : {}) }, idempotencyKey },
      {
        onSuccess: (res) => {
          showToast("success", t("admins.invited", { email: res.email }));
          reset();
          setIdempotencyKey(newKey());
        },
      },
    );
  });

  return (
    <Card>
      <h2 className="text-lg font-semibold text-slate-900">{t("admins.inviteTitle")}</h2>
      <p className="mb-4 text-sm text-slate-600">{t("admins.inviteHint")}</p>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="grid gap-4 sm:grid-cols-2">
        <TextField
          label={t("admins.email")}
          type="email"
          autoComplete="off"
          required
          error={err(errors.email?.message, 320)}
          {...register("email")}
        />
        <SelectField label={t("admins.role")} {...register("role")}>
          {ROLES.map((r: Role) => (
            <option key={r} value={r}>
              {t(`admins.roles.${r}`)}
            </option>
          ))}
        </SelectField>
        <TextField
          label={t("admins.firstName")}
          error={err(errors.first?.message, 100)}
          {...register("first")}
        />
        <TextField
          label={t("admins.lastName")}
          error={err(errors.last?.message, 100)}
          {...register("last")}
        />
        <div className="sm:col-span-2">
          <Button type="submit" disabled={invite.isPending}>
            {t("admins.invite")}
          </Button>
        </div>
      </form>
    </Card>
  );
}
