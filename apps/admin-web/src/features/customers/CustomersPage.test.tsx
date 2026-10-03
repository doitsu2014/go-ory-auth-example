import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { adminMe, customer } from "../../test/fixtures";
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

    expect(await screen.findByText("customer1@example.com")).toBeInTheDocument();
    expect(screen.getByText("Page 1")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByText("customer3@example.com")).toBeInTheDocument();
    expect(screen.getByText("Page 2")).toBeInTheDocument();
    expect(requests.at(-1)?.get("page_token")).toBe("tok-2");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Previous" }));
    expect(await screen.findByText("customer1@example.com")).toBeInTheDocument();
    expect(requests.at(-1)?.get("page_token")).toBeNull();
  });

  it("filters by email and state via the URL and resets pagination", async () => {
    const requests: URLSearchParams[] = [];
    server.use(
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
      http.get(`${API}/admin/v1/customers`, ({ request }) => {
        requests.push(new URL(request.url).searchParams);
        return HttpResponse.json({ items: [customer(1, { state: "inactive" })] });
      }),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/customers");
    await screen.findByText("customer1@example.com");

    await user.type(screen.getByLabelText("Email (exact match)"), "customer1@example.com");
    await user.selectOptions(screen.getByLabelText("State"), "inactive");
    await user.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => expect(requests.at(-1)?.get("email")).toBe("customer1@example.com"));
    expect(requests.at(-1)?.get("state")).toBe("inactive");
    expect(router.state.location.search).toBe("?email=customer1%40example.com&state=inactive");
  });
});
