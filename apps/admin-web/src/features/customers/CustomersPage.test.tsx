import type { QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import type { CustomerLookupResult } from "../../api/client";
import { MASKED_LOGIN, adminMe, customer, maskedPersonalInfo, problem } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, server } from "../../test/server";

describe("CustomersPage", () => {
  it("paginates with next_page_token and goes back", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, ({ request }) => {
        const q = new URL(request.url).searchParams;
        requests.push(q);
        return q.get("page_token") === "tok-2"
          ? HttpResponse.json({ items: [customer(3)] })
          : HttpResponse.json({ items: [customer(1), customer(2)], next_page_token: "tok-2" });
      }),
    );
    const user = userEvent.setup();
    renderApp("/customers");

    expect(await screen.findByText("Customer 1")).toBeInTheDocument();
    expect(screen.getByText("Page 1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("Customer 3")).toBeInTheDocument();
    expect(screen.getByText("Page 2")).toBeInTheDocument();
    expect(requests.at(-1)?.get("page_token")).toBe("tok-2");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByText("Customer 1")).toBeInTheDocument();
    expect(requests.at(-1)?.get("page_token")).toBeNull();
  });

  it("filters by state via the URL, with no email filter anywhere (PLI-FR-11)", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, ({ request }) => {
        requests.push(new URL(request.url).searchParams);
        return HttpResponse.json({ items: [customer(1, { state: "inactive" })] });
      }),
    );
    const user = userEvent.setup();
    // A stale bookmark with ?email= is ignored: never sent to the API.
    const { router } = renderApp("/customers?email=old%40example.com");
    await screen.findByText("Customer 1");
    expect(requests.at(-1)?.has("email")).toBe(false);
    expect(screen.queryByLabelText("Email (exact match)")).toBeNull();

    await user.selectOptions(screen.getByLabelText("State"), "inactive");
    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => expect(requests.at(-1)?.get("state")).toBe("inactive"));
    for (const q of requests) expect(q.has("email")).toBe(false);
    expect(router.state.location.search).toBe("?state=inactive");
  });

  it("removes a stale ?email= from the URL on mount, keeping other params (replace)", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, ({ request }) => {
        requests.push(new URL(request.url).searchParams);
        return HttpResponse.json({ items: [customer(1, { state: "inactive" })] });
      }),
    );
    const { router } = renderApp("/customers?state=inactive&email=old%40example.com");
    await screen.findByText("Customer 1");
    await waitFor(() => expect(router.state.location.search).toBe("?state=inactive"));
    expect(router.state.historyAction).toBe("REPLACE");
    expect(screen.getByLabelText("State")).toHaveValue("inactive");
    for (const q of requests) expect(q.has("email")).toBe(false);
  });

  it("shows the masked login with its type, or a neutral unavailable marker", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, () =>
        HttpResponse.json({
          items: [
            customer(1),
            customer(2, { login: { type: "phone", masked: "+84*******567" } }),
            customer(3, { login: null, login_unavailable: true }),
          ],
        }),
      ),
    );
    renderApp("/customers");
    const table = await screen.findByRole("table", { name: "Customers" });
    const rows = within(table).getAllByRole("row");
    expect(within(table).getByRole("columnheader", { name: "Login" })).toBeInTheDocument();
    expect(rows[1]).toHaveTextContent(`Email${MASKED_LOGIN}`);
    expect(rows[2]).toHaveTextContent("Phone+84*******567");
    expect(within(rows[3] as HTMLElement).getByTestId("login-unavailable")).toHaveTextContent(
      "Unavailable",
    );
    expect(table).not.toHaveTextContent("@example.com");
  });

  it("shows the unavailable marker in Vietnamese", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, () =>
        HttpResponse.json({ items: [customer(1, { login: null, login_unavailable: true })] }),
      ),
    );
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { default: i18n } = await import("../../i18n");
    await act(() => i18n.changeLanguage("vi"));
    expect(screen.getByText("Không khả dụng")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Thông tin đăng nhập" })).toBeInTheDocument();
  });
});

const C = customer(4);

function lookupResult(): CustomerLookupResult {
  return {
    truncated: false,
    items: [
      {
        id: C.id,
        state: "active",
        personal_info: maskedPersonalInfo(),
        login: { type: "email", masked: MASKED_LOGIN },
      },
    ],
  };
}

function setupLookup(lookup: () => Response = () => HttpResponse.json(lookupResult())) {
  const lookups: { body: unknown; url: string }[] = [];
  const listUrls: string[] = [];
  server.use(
    http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
    http.get(`${API}/admin/v1/customers`, ({ request }) => {
      listUrls.push(request.url);
      return HttpResponse.json({ items: [customer(1)] });
    }),
    http.get(`${API}/admin/v1/customers/:id`, () => HttpResponse.json(C)),
    http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
      HttpResponse.json(maskedPersonalInfo()),
    ),
    http.post(`${API}/admin/v1/customers/lookup`, async ({ request }) => {
      lookups.push({ body: await request.json(), url: request.url });
      return lookup();
    }),
  );
  return { lookups, listUrls };
}

function loginForm() {
  const form = screen.getByRole("form", { name: "Find by login" });
  return { form, input: within(form).getByLabelText("Email or phone used to sign in") };
}

function cacheDump(qc: QueryClient): string {
  return JSON.stringify([
    qc
      .getQueryCache()
      .getAll()
      .map((q) => [q.queryKey, q.state.data]),
    qc
      .getMutationCache()
      .getAll()
      .map((m) => [m.state.variables, m.state.data]),
  ]);
}

describe("Login lookup", () => {
  it("POSTs {login: {type: email, value}} in the body only and links masked results", async () => {
    const { lookups, listUrls } = setupLookup();
    const user = userEvent.setup();
    const { router, queryClient } = renderApp("/customers");
    await screen.findByText("Customer 1");

    const { form, input } = loginForm();
    await user.type(input, "  Alice@Example.com ");
    await user.click(within(form).getByRole("button", { name: "Search" }));

    await waitFor(() => expect(lookups).toHaveLength(1));
    expect(lookups[0]?.body).toEqual({ login: { type: "email", value: "Alice@Example.com" } });
    expect(lookups[0]?.url).toBe(`${API}/admin/v1/customers/lookup`);

    const results = await screen.findByRole("table", { name: "Login lookup results" });
    expect(within(results).getByText(MASKED_LOGIN)).toBeInTheDocument();
    expect(within(results).getByText(C.id)).toBeInTheDocument();
    expect(within(results).getByText("Active")).toBeInTheDocument();

    // The value never reaches a URL, router state or the TanStack caches.
    const loc = router.state.location;
    expect(`${loc.pathname}${loc.search}${loc.hash}`.toLowerCase()).not.toContain("alice");
    expect(loc.state).toBeNull();
    for (const u of listUrls) expect(u.toLowerCase()).not.toContain("alice");
    expect(cacheDump(queryClient).toLowerCase()).not.toContain("alice");

    await user.click(within(results).getByRole("link", { name: `View ${C.id}` }));
    expect(await screen.findByRole("heading", { name: MASKED_LOGIN })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/customers/${C.id}`);
  });

  it("clears previous results when the value changes", async () => {
    setupLookup();
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { form, input } = loginForm();
    await user.type(input, "alice@example.com");
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByRole("table", { name: "Login lookup results" })).toBeInTheDocument();

    await user.type(input, "x");
    expect(screen.queryByRole("table", { name: "Login lookup results" })).toBeNull();
  });

  it("detects a phone login when the value has no @", async () => {
    const { lookups } = setupLookup();
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { form, input } = loginForm();
    await user.type(input, "+84 901 234 567");
    await user.click(within(form).getByRole("button", { name: "Search" }));
    await waitFor(() =>
      expect(lookups[0]?.body).toEqual({ login: { type: "phone", value: "+84 901 234 567" } }),
    );
  });

  it("requires a value before calling the API", async () => {
    const { lookups } = setupLookup();
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { form } = loginForm();
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await within(form).findByText("This field is required.")).toBeInTheDocument();
    expect(lookups).toHaveLength(0);
  });

  it("shows an empty state when nothing matches", async () => {
    setupLookup(() => HttpResponse.json({ items: [], truncated: false }));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { form, input } = loginForm();
    await user.type(input, "nobody@example.com");
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(
      await screen.findByText("No customer signs in with this email or phone."),
    ).toBeInTheDocument();
  });

  it.each([
    [422, "validation_failed", "Enter a valid email address or phone number."],
    [
      429,
      "rate_limited",
      "Too many lookups (limit 30 per minute, 200 per day). Please wait and try again.",
    ],
    [
      503,
      "dependency_unavailable",
      "Lookup is temporarily unavailable (encryption service). Please try again later.",
    ],
  ])("shows a localised error on %i %s", async (status, code, message) => {
    setupLookup(() => HttpResponse.json(problem(status, code), { status }));
    const user = userEvent.setup();
    renderApp("/customers");
    await screen.findByText("Customer 1");
    const { form, input } = loginForm();
    await user.type(input, "not-a-login");
    await user.click(within(form).getByRole("button", { name: "Search" }));
    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Login lookup results" })).toBeNull();
  });
});
