import type { UiNode, UiText } from "@ory/client-fetch";

import type {
  AdminMe,
  Customer,
  MaskedPersonalInfo,
  PersonalInfo,
  RevealedPersonalInfo,
} from "../api/client";

export const CSRF = "csrf-token-value";

/** Masked login every `customer()` fixture carries (PLI-FR-10 shape). */
export const MASKED_LOGIN = "c***@e***.com";

export function text(
  id: number,
  t: string,
  type: "info" | "error" | "success" = "info",
  context?: object,
): UiText {
  return { id, text: t, type, ...(context ? { context } : {}) };
}

export function inputNode(
  group: string,
  name: string,
  type: string,
  opts: { value?: unknown; label?: UiText; messages?: UiText[]; required?: boolean } = {},
): UiNode {
  return {
    type: "input",
    group,
    attributes: {
      node_type: "input",
      name,
      type,
      value: opts.value,
      disabled: false,
      required: opts.required,
    },
    messages: opts.messages ?? [],
    meta: opts.label ? { label: opts.label } : {},
  } as unknown as UiNode;
}

export function csrfNode(): UiNode {
  return inputNode("default", "csrf_token", "hidden", { value: CSRF, required: true });
}

export function passwordLoginNodes(messages: { identifier?: UiText[] } = {}): UiNode[] {
  return [
    csrfNode(),
    inputNode("default", "identifier", "text", {
      value: "",
      label: text(1070002, "E-Mail"),
      messages: messages.identifier,
      required: true,
    }),
    inputNode("password", "password", "password", {
      label: text(1070001, "Password"),
      required: true,
    }),
    inputNode("password", "method", "submit", {
      value: "password",
      label: text(1010022, "Sign in with password"),
    }),
  ];
}

export function loginFlow(
  id: string,
  nodes: UiNode[],
  messages: UiText[] = [],
  extra: object = {},
) {
  const now = new Date();
  return {
    id,
    type: "browser",
    state: "choose_method",
    expires_at: new Date(now.getTime() + 600_000).toISOString(),
    issued_at: now.toISOString(),
    request_url: "http://kratos.test/self-service/login/browser",
    refresh: false,
    requested_aal: "aal1",
    ui: {
      action: `http://kratos.test/self-service/login?flow=${id}`,
      method: "POST",
      nodes,
      messages,
    },
    ...extra,
  };
}

export function adminMe(overrides: Partial<AdminMe> = {}): AdminMe {
  return {
    id: "7f3c0000-0000-4000-8000-000000000001",
    email: "ops@example.com",
    aal: "aal2",
    roles: ["admin"],
    permissions: ["view_customers", "manage_customers", "view_audit"],
    session_expires_at: "2026-10-03T20:00:00Z",
    ...overrides,
  };
}

export function customer(i: number, overrides: Partial<Customer> = {}): Customer {
  return {
    id: `5d9c0000-0000-4000-8000-${String(i).padStart(12, "0")}`,
    login: { type: "email", masked: MASKED_LOGIN },
    login_unavailable: false,
    email_verified: true,
    state: "active",
    display_name: `Customer ${String(i)}`,
    created_at: "2026-10-03T08:00:00Z",
    ...overrides,
  };
}

export function maskedPersonalInfo(
  overrides: Partial<MaskedPersonalInfo> = {},
): MaskedPersonalInfo {
  return {
    name: { first: "A***", last: "N***" },
    phone_number: "+84*******567",
    date_of_birth: "1990-**-**",
    address: { city: "Ha Noi", country: "VN" },
    national_id: null,
    has_name: true,
    has_phone_number: true,
    has_date_of_birth: true,
    has_address: true,
    has_national_id: false,
    updated_at: "2026-10-03T08:00:00Z",
    ...overrides,
  };
}

export function personalInfo(overrides: Partial<PersonalInfo> = {}): PersonalInfo {
  return {
    name: { first: "An", last: "Nguyễn" },
    phone_number: "+84901234567",
    date_of_birth: "1990-05-17",
    address: { line1: "12 Trang Tien", city: "Ha Noi", country: "VN" },
    national_id: null,
    ...overrides,
  };
}

export function problem(status: number, code: string) {
  return {
    type: `https://docs.example.com/problems/${code}`,
    title: code,
    status,
    code,
  };
}

/** Reveal response: personal info plus the full login identifier. */
export function revealedPersonalInfo(
  overrides: Partial<RevealedPersonalInfo> = {},
): RevealedPersonalInfo {
  return {
    ...personalInfo(),
    login: { type: "email", value: "customer@example.com" },
    ...overrides,
  };
}
