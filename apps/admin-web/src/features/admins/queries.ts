import { keepPreviousData, queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";

import { type InviteAdminRequest, type Role, api, unwrap } from "../../api/client";
import { meKeys } from "../../auth/me";
import { auditKeys } from "../audit/queries";

export const adminKeys = {
  all: ["admins"] as const,
  lists: () => [...adminKeys.all, "list"] as const,
  list: (pageToken: string | undefined) => [...adminKeys.lists(), { pageToken }] as const,
};

export function adminListQuery(pageToken: string | undefined) {
  return queryOptions({
    queryKey: adminKeys.list(pageToken),
    queryFn: () =>
      unwrap(
        api.GET("/admin/v1/admins", {
          params: { query: { page_size: 25, page_token: pageToken } },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}

export function useInviteAdmin() {
  const qc = useQueryClient();
  return useMutation({
    /** `idempotencyKey` is kept stable across retries of the same submission. */
    mutationFn: ({ body, idempotencyKey }: { body: InviteAdminRequest; idempotencyKey: string }) =>
      unwrap(
        api.POST("/admin/v1/admins", {
          params: { header: { "Idempotency-Key": idempotencyKey } },
          body,
        }),
      ),
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: adminKeys.lists() }),
        qc.invalidateQueries({ queryKey: auditKeys.all }),
      ]),
  });
}

export function useChangeAdminRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) =>
      unwrap(api.PUT("/admin/v1/admins/{id}/role", { params: { path: { id } }, body: { role } })),
    // Roles/permissions may have changed (Keto tuples): re-ask /me for UI gating.
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: adminKeys.lists() }),
        qc.invalidateQueries({ queryKey: meKeys.all }),
        qc.invalidateQueries({ queryKey: auditKeys.all }),
      ]),
  });
}
