import { keepPreviousData, queryOptions } from "@tanstack/react-query";

import { api, unwrap } from "../../api/client";

export interface AuditFilters {
  targetType?: string;
  targetId?: string;
  actorId?: string;
}

export const auditKeys = {
  all: ["audit-events"] as const,
  list: (f: AuditFilters, pageToken: string | undefined) =>
    [...auditKeys.all, "list", f, { pageToken }] as const,
};

export function auditListQuery(f: AuditFilters, pageToken: string | undefined) {
  return queryOptions({
    queryKey: auditKeys.list(f, pageToken),
    queryFn: () =>
      unwrap(
        api.GET("/admin/v1/audit-events", {
          params: {
            query: {
              target_type: f.targetType || undefined,
              target_id: f.targetId || undefined,
              actor_id: f.actorId || undefined,
              page_size: 25,
              page_token: pageToken,
            },
          },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}
