import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { MASKED_LOGIN, adminMe, customer, maskedPersonalInfo, problem } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, server } from "../../test/server";

const C = customer(7);

function setup(disableStatus = 200) {
  let state: "active" | "inactive" = "active";
  const disableBodies: unknown[] = [];
  server.use(
    http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
    http.get(`${API}/admin/v1/customers/:id`, () => HttpResponse.json({ ...C, state })),
    http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
      HttpResponse.json(maskedPersonalInfo()),
    ),
    http.post(`${API}/admin/v1/customers/:id/disable`, async ({ request }) => {
      disableBodies.push(await request.json());
      if (disableStatus !== 200) {
        return HttpResponse.json(problem(disableStatus, "forbidden"), { status: disableStatus });
      }
      state = "inactive";
      return HttpResponse.json({ id: C.id, state });
    }),
  );
  return { disableBodies };
}

describe("CustomerDetailPage details", () => {
  it("does not show a Kratos name, even if the API still sent one (NAME-FR-07)", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers/:id`, () =>
        HttpResponse.json({ ...C, name: { first: "Legacy", last: "Kratos" } }),
      ),
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(maskedPersonalInfo()),
      ),
    );
    renderApp(`/customers/${C.id}`);
    expect(await screen.findByRole("heading", { name: MASKED_LOGIN })).toBeInTheDocument();
    expect(screen.getByText(C.display_name ?? "")).toBeInTheDocument();
    expect(screen.getByText("Display name (nickname)")).toBeInTheDocument();
    expect(screen.queryByText(/Legacy|Kratos/)).toBeNull();
    expect(screen.queryByText("Name")).toBeNull();
    // The real name only appears masked, inside the personal-info card.
    expect(await screen.findByText("A*** N***")).toBeInTheDocument();
  });
});

describe("CustomerDetailPage login", () => {
  it("shows the masked login with its type, never the plaintext", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers/:id`, () =>
        HttpResponse.json(customer(7, { login: { type: "phone", masked: "+84*******321" } })),
      ),
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(maskedPersonalInfo()),
      ),
    );
    renderApp(`/customers/${C.id}`);
    expect(await screen.findByRole("heading", { name: "+84*******321" })).toBeInTheDocument();
    const login = screen.getByText("Login").closest("div");
    expect(login).toHaveTextContent("Phone+84*******321");
    expect(screen.queryByTestId("login-unavailable")).toBeNull();
  });

  it("shows a neutral unavailable marker when the key manager is down", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers/:id`, () =>
        HttpResponse.json(customer(7, { login: null, login_unavailable: true })),
      ),
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(maskedPersonalInfo()),
      ),
    );
    renderApp(`/customers/${C.id}`);
    // Falls back to the nickname as the page title.
    expect(await screen.findByRole("heading", { name: "Customer 7" })).toBeInTheDocument();
    expect(screen.getByTestId("login-unavailable")).toHaveTextContent("Unavailable");
  });

  it("names the customer by masked login in confirmations", async () => {
    setup();
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    await user.click(await screen.findByRole("button", { name: "Revoke all sessions" }));
    expect(
      screen.getByText(`${MASKED_LOGIN} will be signed out of all devices.`),
    ).toBeInTheDocument();
  });
});

describe("CustomerDetailPage disable", () => {
  it("requires confirmation with a reason, then disables and refreshes", async () => {
    const { disableBodies } = setup();
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);

    await user.click(await screen.findByRole("button", { name: "Disable" }));
    const dialog = screen.getByRole("dialog", { name: "Disable this customer?" });

    // Reason is mandatory.
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));
    expect(await within(dialog).findByText("This field is required.")).toBeInTheDocument();
    expect(disableBodies).toHaveLength(0);

    await user.type(within(dialog).getByLabelText("Reason"), "fraud report #123");
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));

    await waitFor(() => expect(disableBodies).toEqual([{ reason: "fraud report #123" }]));
    expect(await screen.findByText("Customer disabled.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByRole("button", { name: "Enable" })).toBeInTheDocument();
  });

  it("cancel closes the dialog without calling the API", async () => {
    const { disableBodies } = setup();
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    await user.click(await screen.findByRole("button", { name: "Disable" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(disableBodies).toHaveLength(0);
  });

  it("shows a toast mapped from the problem code on 403", async () => {
    setup(403);
    const user = userEvent.setup();
    renderApp(`/customers/${C.id}`);
    await user.click(await screen.findByRole("button", { name: "Disable" }));
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByLabelText("Reason"), "test");
    await user.click(within(dialog).getByRole("button", { name: "Disable" }));
    expect(await screen.findByText("You do not have permission to do that.")).toBeInTheDocument();
  });

  it("hides management actions without manage_customers", async () => {
    server.use(
      http.get(`${API}/admin/v1/me`, () =>
        HttpResponse.json(adminMe({ permissions: ["view_customers"] })),
      ),
      http.get(`${API}/admin/v1/customers/:id`, () => HttpResponse.json(C)),
      http.get(`${API}/admin/v1/customers/:id/personal-info`, () =>
        HttpResponse.json(maskedPersonalInfo()),
      ),
    );
    renderApp(`/customers/${C.id}`);
    expect(await screen.findByRole("heading", { name: MASKED_LOGIN })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Revoke all sessions" })).toBeNull();
  });
});
