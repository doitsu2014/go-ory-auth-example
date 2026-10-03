import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import type { CustomerLookupResult } from "../../api/client";
import {
  adminMe,
  customer,
  loginFlow,
  maskedPersonalInfo,
  passwordLoginNodes,
  problem,
} from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, KRATOS, server } from "../../test/server";

const C = customer(4);
const PHONE = "+84901234567";

function lookupResult(truncated = false): CustomerLookupResult {
  return {
    truncated,
    items: [{ id: C.id, state: "active", personal_info: maskedPersonalInfo() }],
  };
}

const TRUNCATED_EN =
  "More than 20 customers use this number; phone numbers are self-declared and unverified — narrow down by other means.";

function setup(lookup: () => Response = () => HttpResponse.json(lookupResult())) {
  const lookups: { body: unknown; url: string }[] = [];
  server.use(
    http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
    http.get(`${API}/admin/v1/customers`, () => HttpResponse.json({ items: [customer(1)] })),
    http.get(`${API}/admin/v1/customers/:id`, () => HttpResponse.json(C)),
    http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
      HttpResponse.json(maskedPersonalInfo()),
    ),
    http.post(`${API}/admin/v1/customers/lookup`, async ({ request }) => {
      lookups.push({ body: await request.json(), url: request.url });
      return lookup();
    }),
  );
  return { lookups };
}

function phoneField() {
  const form = screen.getByRole("form", { name: "Find by phone number" });
  return { form, input: within(form).getByLabelText("Phone number") };
}

describe("PhoneLookup", () => {
  it("POSTs the normalised phone in the body only and links masked results to the detail page", async () => {
    const { lookups } = setup();
    const user = userEvent.setup();
    const { router, queryClient } = renderApp("/customers");
    await screen.findByText("customer1@example.com");

    const { form, input } = phoneField();
    await user.type(input, "+84 901-234.567");
    await user.click(within(form).getByRole("button", { name: "Search" }));

    await waitFor(() => expect(lookups).toHaveLength(1));
    expect(lookups[0]?.body).toEqual({ phone_number: PHONE });
    expect(lookups[0]?.url).toBe(`${API}/admin/v1/customers/lookup`);

    const results = await screen.findByRole("table", { name: "Phone lookup results" });
    expect(within(results).getByText("+84*******567")).toBeInTheDocument();
    expect(within(results).getByText("Ha Noi, VN")).toBeInTheDocument();
    expect(within(results).getByText("A*** N***")).toBeInTheDocument();

    // The number is never in the URL, router state or TanStack caches.
    const loc = router.state.location;
    expect(`${loc.pathname}${loc.search}${loc.hash}`).not.toContain("901");
    expect(loc.state).toBeNull();
    const dump = JSON.stringify([
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => [q.queryKey, q.state.data]),
      queryClient
        .getMutationCache()
        .getAll()
        .map((m) => m.state.variables),
    ]);
    expect(dump).not.toContain("901234567");

    await user.click(within(results).getByRole("link", { name: `View ${C.id}` }));
    expect(await screen.findByRole("heading", { name: C.email })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/customers/${C.id}`);
    expect(router.state.location.state).toBeNull();
  });

  it.each(["0901234567", "+8412345", "+0901234567", "+8490123456789012"])(
    "rejects %s (PII-FR-03) before calling the API",
    async (phone) => {
      const { lookups } = setup();
      const user = userEvent.setup();
      renderApp("/customers");
      await screen.findByText("customer1@example.com");
      const { form, input } = phoneField();
      await user.type(input, phone);
      await user.click(within(form).getByRole("button", { name: "Search" }));
      expect(
        await screen.findByText(
          "Enter a phone number in international format (+ and 8–15 digits).",
        ),
      ).toBeInTheDocument();
      expect(lookups).toHaveLength(0);
    },
  );

  it("clears previous results when the next number fails the client-side check", async () => {
    const { lookups } = setup();
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByRole("table", { name: "Phone lookup results" })).toBeInTheDocument();

    await user.clear(input);
    await user.type(input, "0901234567");
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(
      await screen.findByText("Enter a phone number in international format (+ and 8–15 digits)."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Phone lookup results" })).toBeNull();
    expect(screen.queryByText("+84*******567")).toBeNull();
    expect(lookups).toHaveLength(1);
  });

  it("notes that phone numbers are unverified and warns when the result is truncated", async () => {
    setup(() => HttpResponse.json(lookupResult(true)));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    expect(screen.getByText(/Phone numbers are self-declared by customers/)).toBeInTheDocument();
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByText(TRUNCATED_EN)).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Phone lookup results" })).toBeInTheDocument();
  });

  it("shows no truncation warning when truncated is false", async () => {
    setup();
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    await screen.findByRole("table", { name: "Phone lookup results" });
    expect(screen.queryByText(TRUNCATED_EN)).toBeNull();
  });

  it("shows the truncation warning in Vietnamese", async () => {
    setup(() => HttpResponse.json(lookupResult(true)));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { default: i18n } = await import("../../i18n");
    await act(() => i18n.changeLanguage("vi"));
    const form = screen.getByRole("form", { name: "Tìm theo số điện thoại" });
    await user.type(within(form).getByLabelText("Số điện thoại"), PHONE);
    await user.click(within(form).getByRole("button", { name: "Tìm kiếm" }));
    expect(
      await screen.findByText(
        "Có hơn 20 khách hàng dùng số này; số điện thoại do khách hàng tự khai báo và chưa được xác minh — hãy thu hẹp bằng cách khác.",
      ),
    ).toBeInTheDocument();
  });

  it("shows an empty state when nothing matches", async () => {
    setup(() => HttpResponse.json({ items: [], truncated: false }));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByText("No customer has this phone number.")).toBeInTheDocument();
  });

  it.each([
    [
      429,
      "rate_limited",
      "Too many lookups (limit 30 per minute, 200 per day). Please wait and try again.",
    ],
    [403, "forbidden", "You do not have permission to look up customers."],
    [
      503,
      "dependency_unavailable",
      "Lookup is temporarily unavailable (encryption service). Please try again later.",
    ],
  ])("shows a localised error on %i %s", async (status, code, message) => {
    setup(() => HttpResponse.json(problem(status, code), { status }));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Phone lookup results" })).toBeNull();
  });

  it("routes to sign-in on 401", async () => {
    setup(() => HttpResponse.json(problem(401, "unauthenticated"), { status: 401 }));
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () =>
        HttpResponse.json(loginFlow("flow-1", passwordLoginNodes())),
      ),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/customers");
    await screen.findByText("customer1@example.com");
    const { form, input } = phoneField();
    await user.type(input, PHONE);
    await user.click(within(form).getByRole("button", { name: "Search" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(router.state.location.search).not.toContain("901");
  });
});
