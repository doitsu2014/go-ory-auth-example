import { keepPreviousData, queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";

import {
  type CustomerLookupResult,
  type IdentityState,
  type LoginIdentifier,
  type RevealRequest,
  type RevealedPersonalInfo,
  api,
  unwrap,
} from "../../api/client";
import { auditKeys } from "../audit/queries";

/** List filters. There is deliberately no contact filter: PII never goes in a URL (PLI-FR-11). */
export interface CustomerListParams {
  state?: IdentityState;
  pageToken?: string;
  pageSize?: number;
}

export const customerKeys = {
  all: ["customers"] as const,
  lists: () => [...customerKeys.all, "list"] as const,
  list: (p: CustomerListParams) => [...customerKeys.lists(), p] as const,
  detail: (id: string) => [...customerKeys.all, "detail", id] as const,
  /** Masked view only — revealed values are never cached (see revealPersonalInfo). */
  maskedPersonalInfo: (id: string) => [...customerKeys.detail(id), "personal-info"] as const,
};

export const PAGE_SIZE = 25;

export function customerListQuery(p: CustomerListParams) {
  return queryOptions({
    queryKey: customerKeys.list(p),
    queryFn: () =>
      unwrap(
        api.GET("/admin/v1/customers", {
          params: {
            query: {
              state: p.state,
              page_size: p.pageSize ?? PAGE_SIZE,
              page_token: p.pageToken,
            },
          },
        }),
      ),
    placeholderData: keepPreviousData,
  });
}

export function customerQuery(id: string) {
  return queryOptions({
    queryKey: customerKeys.detail(id),
    queryFn: () => unwrap(api.GET("/admin/v1/customers/{id}", { params: { path: { id } } })),
  });
}

export function maskedPersonalInfoQuery(id: string) {
  return queryOptions({
    queryKey: customerKeys.maskedPersonalInfo(id),
    queryFn: () =>
      unwrap(api.GET("/admin/v1/customers/{id}/personal-info", { params: { path: { id } } })),
  });
}

/**
 * POST /admin/v1/customers/{id}/personal-info/reveal. Deliberately a plain
 * call, not a query or useMutation: TanStack keeps query data and mutation
 * state (data + variables) in its caches, and plaintext PII must live only in
 * the calling component's state.
 */
export function revealPersonalInfo(id: string, body: RevealRequest): Promise<RevealedPersonalInfo> {
  return unwrap(
    api.POST("/admin/v1/customers/{id}/personal-info/reveal", {
      params: { path: { id } },
      body,
    }),
  );
}

/**
 * POST /admin/v1/customers/lookup. The phone number travels in the body only
 * (never URL, query string or router state) and, like reveal, is not passed
 * through TanStack so it never sits in the mutation cache as a variable.
 */
export function lookupCustomersByPhone(phoneNumber: string): Promise<CustomerLookupResult> {
  return unwrap(api.POST("/admin/v1/customers/lookup", { body: { phone_number: phoneNumber } }));
}

/**
 * POST /admin/v1/customers/lookup with the login identifier (exact match,
 * PLI-FR-11). Same rules as the phone lookup: body only, never cached.
 */
export function lookupCustomersByLogin(login: LoginIdentifier): Promise<CustomerLookupResult> {
  return unwrap(api.POST("/admin/v1/customers/lookup", { body: { login } }));
}

function useInvalidateCustomer(id: string) {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: customerKeys.detail(id) }),
      qc.invalidateQueries({ queryKey: customerKeys.lists() }),
      // The mutation wrote an audit event.
      qc.invalidateQueries({ queryKey: auditKeys.all }),
    ]);
}

export function useDisableCustomer(id: string) {
  const invalidate = useInvalidateCustomer(id);
  return useMutation({
    mutationFn: (reason: string) =>
      unwrap(
        api.POST("/admin/v1/customers/{id}/disable", {
          params: { path: { id } },
          body: { reason },
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useEnableCustomer(id: string) {
  const invalidate = useInvalidateCustomer(id);
  return useMutation({
    mutationFn: (reason: string) =>
      unwrap(
        api.POST("/admin/v1/customers/{id}/enable", {
          params: { path: { id } },
          body: reason ? { reason } : {},
        }),
      ),
    onSuccess: invalidate,
  });
}

export function useRevokeCustomerSessions(id: string) {
  const qc = useQueryClient();
  return useMutation({
    onSuccess: () => qc.invalidateQueries({ queryKey: auditKeys.all }),
    mutationFn: () =>
      unwrap(api.DELETE("/admin/v1/customers/{id}/sessions", { params: { path: { id } } })),
  });
}
