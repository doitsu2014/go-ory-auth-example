import createClient from "openapi-fetch";

import { env } from "../shared/env";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];
export type Problem = Schemas["Problem"];
export type AdminMe = Schemas["AdminMe"];
export type Customer = Schemas["Customer"];
export type CustomerPage = Schemas["CustomerPage"];
export type Admin = Schemas["Admin"];
export type AdminPage = Schemas["AdminPage"];
export type AuditEvent = Schemas["AuditEvent"];
export type AuditEventPage = Schemas["AuditEventPage"];
export type Role = Schemas["Role"];
export type Permission = Schemas["Permission"];
export type IdentityState = Schemas["IdentityState"];
export type InviteAdminRequest = Schemas["InviteAdminRequest"];
export type PersonalInfo = Schemas["PersonalInfo"];
export type MaskedPersonalInfo = Schemas["MaskedPersonalInfo"];
export type RevealRequest = Schemas["RevealRequest"];
export type RevealReasonCode = RevealRequest["reason_code"];
export type PiiField = NonNullable<RevealRequest["fields"]>[number];
export type CustomerLookupResult = Schemas["CustomerLookupResult"];
export type CustomerLookupItem = CustomerLookupResult["items"][number];
export type MachineScope = Schemas["MachineScope"];
export type ServiceClient = Schemas["ServiceClient"];
export type ServiceClientWithSecret = Schemas["ServiceClientWithSecret"];
export type ServiceClientList = Schemas["ServiceClientList"];
export type CreateServiceClientRequest = Schemas["CreateServiceClientRequest"];

export const ROLES: readonly Role[] = ["support", "admin", "super_admin"];

export const REVEAL_REASON_CODES: readonly RevealReasonCode[] = [
  "customer_support_request",
  "identity_verification",
  "fraud_investigation",
  "legal_request",
];

export const PII_FIELDS: readonly PiiField[] = [
  "name",
  "phone_number",
  "date_of_birth",
  "address",
  "national_id",
];

export const MACHINE_SCOPES: readonly MachineScope[] = ["customers:read", "audit:read"];

/**
 * Typed identity-service client generated from the OpenAPI contract.
 * The admin plane accepts only the Kratos session cookie, so every request
 * uses `credentials: "include"`; no token is ever attached by the app.
 */
export const api = createClient<paths>({
  baseUrl: env.apiUrl,
  credentials: "include",
  headers: { Accept: "application/json" },
  // Resolve fetch lazily so test interceptors (MSW) apply.
  fetch: (request: Request) => globalThis.fetch(request),
});

/** Error carrying an RFC 9457 problem from identity-service. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly problem: Problem | undefined;

  constructor(status: number, code: string, problem?: Problem) {
    super(problem?.title ?? code);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.problem = problem;
  }
}

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError;
}

function isProblem(value: unknown): value is Problem {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { code?: unknown }).code === "string"
  );
}

function fallbackCode(status: number): string {
  if (status === 401) return "unauthenticated";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 422) return "validation_failed";
  if (status === 429) return "rate_limited";
  if (status === 503) return "dependency_unavailable";
  return "internal";
}

interface FetchResult<T> {
  data?: T;
  error?: unknown;
  response: Response;
}

/** Resolves a typed openapi-fetch call to its data or throws ApiError. */
export async function unwrap<T>(call: Promise<FetchResult<T>>): Promise<T> {
  let result: FetchResult<T>;
  try {
    result = await call;
  } catch {
    throw new ApiError(0, "network_error");
  }
  const { data, error, response } = result;
  if (!response.ok) {
    const problem = isProblem(error) ? error : undefined;
    throw new ApiError(response.status, problem?.code ?? fallbackCode(response.status), problem);
  }
  return data as T;
}
