import { queryOptions, useQuery } from "@tanstack/react-query";

import { type AdminMe, type Permission, api, unwrap } from "../api/client";

export const meKeys = {
  all: ["admin-me"] as const,
};

export const meQuery = queryOptions({
  queryKey: meKeys.all,
  queryFn: () => unwrap(api.GET("/admin/v1/me")),
  staleTime: 30_000,
});

/** The signed-in admin (guaranteed present below RequireAdmin). */
export function useAdminMe(): AdminMe | undefined {
  return useQuery(meQuery).data;
}

/** Cosmetic UI gating only — the API stays authoritative and its 403s are handled. */
export function useCan(permission: Permission): boolean {
  const me = useAdminMe();
  return me?.permissions.includes(permission) ?? false;
}
