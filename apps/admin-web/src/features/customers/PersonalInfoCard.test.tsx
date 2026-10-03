import type { QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Permission } from "../../api/client";
import { adminMe, customer, maskedPersonalInfo, personalInfo, problem } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, server } from "../../test/server";
import { REVEAL_TTL_MS } from "./PersonalInfoCard";

const C = customer(7);
const PLAIN_PHONE = "+84901234567";
const PLAIN_LINE1 = "12 Trang Tien";

const BASE: Permission[] = ["view_customers", "manage_customers", "view_audit"];

function setup(opts: { permissions?: Permission[]; revealStatus?: number; code?: string } = {}) {
  const revealBodies: unknown[] = [];
  server.use(
    http.get(`${API}/admin/v1/me`, () =>
      HttpResponse.json(
        adminMe({ permissions: opts.permissions ?? [...BASE, "reveal_customer_pii"] }),
      ),
    ),
    http.get(`${API}/admin/v1/customers`, () => HttpResponse.json({ items: [C] })),
    http.get(`${API}/admin/v1/customers/:id`, () => HttpResponse.json(C)),
    http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
      HttpResponse.json(maskedPersonalInfo()),
    ),
    http.post(`${API}/admin/v1/customers/:id/personal-info/reveal`, async ({ request }) => {
      revealBodies.push(await request.json());
      const status = opts.revealStatus ?? 200;
      if (status !== 200) {
        return HttpResponse.json(problem(status, opts.code ?? "internal"), { status });
      }
      return HttpResponse.json(personalInfo());
    }),
  );
  return { revealBodies };
}

/** Everything TanStack holds: query data and mutation variables/data. */
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

function storageDump(): string {
  const out: string[] = [];
  for (const s of [localStorage, sessionStorage]) {
    for (let i = 0; i < s.length; i++) {
      const k = s.key(i) ?? "";
      out.push(k, s.getItem(k) ?? "");
    }
  }
  return out.join("\n");
}

async function openReveal(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Reveal" }));
  return screen.getByRole("dialog", { name: "Reveal personal information?" });
}

afterEach(() => {
  vi.useRealTimers();
});

describe("PersonalInfoCard masked view", () => {
  it("shows masked values, and 'Not provided' for fields the customer has not given", async () => {
    setup({ permissions: BASE });
    renderApp(`/customers/${C.id}`);

    expect(await screen.findByText("+84*******567")).toBeInTheDocument();
    expect(screen.getByText("1990-**-**")).toBeInTheDocument();
    expect(screen.getByText("Ha Noi, VN")).toBeInTheDocument();
    // has_national_id = false → not provided (distinct from a hidden value).
    expect(screen.getByText("Not provided")).toBeInTheDocument();
    expect(screen.queryByText("Hidden")).toBeNull();
  });

  it("says 'Hidden' when a value exists but the masked form is withheld", async () => {
    setup({ permissions: BASE });
    server.use(
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(maskedPersonalInfo({ has_national_id: true, national_id: null })),
      ),
    );
    renderApp(`/customers/${C.id}`);
    expect(await screen.findByText("Hidden")).toBeInTheDocument();
    expect(screen.queryByText("Not provided")).toBeNull();
  });

  it("hides Reveal without reveal_customer_pii", async () => {
    setup({ permissions: BASE });
    renderApp(`/customers/${C.id}`);
    expect(await screen.findByText("+84*******567")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reveal" })).toBeNull();
  });

  it("shows an inline message when the masked view is unavailable (503)", async () => {
    setup();
    server.use(
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(problem(503, "dependency_unavailable"), { status: 503 }),
      ),
    );
    renderApp(`/customers/${C.id}`);
    expect(
      await screen.findByText(
        "Personal information is temporarily unavailable (encryption service). Please try again later.",
        {},
        { timeout: 5000 },
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reveal" })).toBeNull();
  });
});

describe("PersonalInfoCard reveal", () => {
  it("requires a reason code, validates the ticket ref and sends the selected fields", async () => {
    const { revealBodies } = setup();
    const user = userEvent.setup();
    const { queryClient } = renderApp(`/customers/${C.id}`);
    const dialog = await openReveal(user);

    // Only provided fields are offered.
    expect(within(dialog).queryByLabelText("National ID")).toBeNull();

    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));
    expect(await within(dialog).findByText("This field is required.")).toBeInTheDocument();

    await user.selectOptions(within(dialog).getByLabelText("Reason"), "fraud_investigation");
    await user.type(within(dialog).getByLabelText("Ticket reference (optional)"), "sup 1");
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));
    expect(await within(dialog).findByText(/Use the format ABC-123/)).toBeInTheDocument();
    expect(revealBodies).toHaveLength(0);

    await user.clear(within(dialog).getByLabelText("Ticket reference (optional)"));
    await user.type(within(dialog).getByLabelText("Ticket reference (optional)"), "SUP-1234");
    await user.click(within(dialog).getByLabelText("Address"));
    await user.click(within(dialog).getByLabelText("Phone number"));
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));

    await waitFor(() =>
      expect(revealBodies).toEqual([
        {
          reason_code: "fraud_investigation",
          ticket_ref: "SUP-1234",
          fields: ["phone_number", "address"],
        },
      ]),
    );
    expect(await screen.findByText(PLAIN_PHONE)).toBeInTheDocument();
    expect(screen.getByText(`${PLAIN_LINE1}, Ha Noi, VN`)).toBeInTheDocument();
    // Not requested → still masked.
    expect(screen.getByText("1990-**-**")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();

    // Plaintext lives in component state only.
    expect(cacheDump(queryClient)).not.toContain(PLAIN_PHONE);
    expect(cacheDump(queryClient)).not.toContain(PLAIN_LINE1);
    expect(storageDump()).not.toContain(PLAIN_PHONE);

    await user.click(screen.getByRole("button", { name: "Hide" }));
    expect(screen.queryByText(PLAIN_PHONE)).toBeNull();
    expect(screen.getByText("+84*******567")).toBeInTheDocument();
  });

  it("omits ticket_ref and fields when not given (reveal all)", async () => {
    const { revealBodies } = setup();
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    const dialog = await openReveal(user);
    await user.selectOptions(within(dialog).getByLabelText("Reason"), "customer_support_request");
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));

    await waitFor(() =>
      expect(revealBodies).toEqual([{ reason_code: "customer_support_request" }]),
    );
    expect(await screen.findByText(PLAIN_PHONE)).toBeInTheDocument();
    expect(screen.getByText("1990-05-17")).toBeInTheDocument();
  });

  it("hides revealed values automatically after 60 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    setup();
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    renderApp(`/customers/${C.id}`);
    const dialog = await openReveal(user);
    await user.selectOptions(within(dialog).getByLabelText("Reason"), "identity_verification");
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));
    expect(await screen.findByText(PLAIN_PHONE)).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(REVEAL_TTL_MS - 1000);
    });
    expect(screen.getByText(PLAIN_PHONE)).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.queryByText(PLAIN_PHONE)).toBeNull();
    expect(screen.getByText("+84*******567")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reveal" })).toBeInTheDocument();
  });

  it("drops revealed values when the page unmounts", async () => {
    setup();
    const user = userEvent.setup();
    const { router, queryClient, unmount } = renderApp(`/customers/${C.id}`);
    const dialog = await openReveal(user);
    await user.selectOptions(within(dialog).getByLabelText("Reason"), "legal_request");
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));
    expect(await screen.findByText(PLAIN_PHONE)).toBeInTheDocument();

    await act(() => router.navigate("/customers"));
    expect(await screen.findByText(C.email)).toBeInTheDocument();
    expect(screen.queryByText(PLAIN_PHONE)).toBeNull();

    await act(() => router.navigate(`/customers/${C.id}`));
    expect(await screen.findByText("+84*******567")).toBeInTheDocument();
    expect(screen.queryByText(PLAIN_PHONE)).toBeNull();
    expect(cacheDump(queryClient)).not.toContain(PLAIN_PHONE);
    expect(router.state.location.state).toBeNull();

    unmount();
    expect(cacheDump(queryClient)).not.toContain(PLAIN_PHONE);
    expect(storageDump()).not.toContain(PLAIN_PHONE);
  });

  it.each([
    [429, "rate_limited", "Reveal limit reached (20 per hour). Please try again later."],
    [403, "forbidden", "You do not have permission to reveal personal information."],
    [404, "not_found", "This customer was not found."],
    [
      503,
      "dependency_unavailable",
      "The encryption service is unavailable, so nothing was revealed. Please try again later.",
    ],
  ])("shows a localised error in the dialog on %i %s", async (status, code, message) => {
    setup({ revealStatus: status, code });
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    const dialog = await openReveal(user);
    await user.selectOptions(within(dialog).getByLabelText("Reason"), "fraud_investigation");
    await user.click(within(dialog).getByRole("button", { name: "Reveal" }));

    expect(await within(dialog).findByText(message)).toBeInTheDocument();
    expect(screen.queryByText(PLAIN_PHONE)).toBeNull();
    expect(screen.getByText("+84*******567")).toBeInTheDocument();
  });

  it("is shown in Vietnamese", async () => {
    setup();
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    await screen.findByRole("button", { name: "Reveal" });
    const { default: i18n } = await import("../../i18n");
    await act(() => i18n.changeLanguage("vi"));
    expect(screen.getByRole("heading", { name: "Thông tin cá nhân" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hiển thị" }));
    expect(screen.getByRole("dialog", { name: "Hiển thị thông tin cá nhân?" })).toBeInTheDocument();
  });
});
