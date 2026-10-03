import { queryOptions } from "@tanstack/react-query";

import {
  type CreateServiceClientRequest,
  type ServiceClientWithSecret,
  api,
  unwrap,
} from "../../api/client";

export const serviceClientKeys = {
  all: ["service-clients"] as const,
  lists: () => [...serviceClientKeys.all, "list"] as const,
};

/** GET /admin/v1/service-clients. `ServiceClient` never carries a secret, so it may be cached. */
export function serviceClientListQuery() {
  return queryOptions({
    queryKey: serviceClientKeys.lists(),
    queryFn: () => unwrap(api.GET("/admin/v1/service-clients")),
  });
}

/**
 * POST /admin/v1/service-clients. Deliberately a plain call, not useMutation:
 * the 201 body holds the one-time `client_secret`, and TanStack keeps mutation
 * data in its cache. The secret must live only in the calling component's state.
 */
export function createServiceClient(
  body: CreateServiceClientRequest,
  idempotencyKey: string,
): Promise<ServiceClientWithSecret> {
  return unwrap(
    api.POST("/admin/v1/service-clients", {
      params: { header: { "Idempotency-Key": idempotencyKey } },
      body,
    }),
  );
}

/** POST /admin/v1/service-clients/{client_id}/rotate-secret. Plain call for the same reason as create. */
export function rotateServiceClientSecret(clientId: string): Promise<ServiceClientWithSecret> {
  return unwrap(
    api.POST("/admin/v1/service-clients/{client_id}/rotate-secret", {
      params: { path: { client_id: clientId } },
    }),
  );
}

/**
 * DELETE /admin/v1/service-clients/{client_id}. A plain call too, so its
 * errors are shown inside the confirm dialog instead of the global toast.
 */
export function deleteServiceClient(clientId: string): Promise<unknown> {
  return unwrap(
    api.DELETE("/admin/v1/service-clients/{client_id}", {
      params: { path: { client_id: clientId } },
    }),
  );
}
