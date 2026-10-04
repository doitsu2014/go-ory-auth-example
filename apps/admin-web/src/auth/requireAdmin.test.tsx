import { QueryClient } from "@tanstack/react-query";
import { screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { LoaderFunctionArgs } from "react-router";
import { describe, expect, it } from "vitest";

import { adminMe, customer, problem } from "../test/fixtures";
import { renderApp } from "../test/render";
import { API, server } from "../test/server";
import { meKeys } from "./me";
import { requireAdminLoader } from "./requireAdmin";

function args(url: string): LoaderFunctionArgs {
  return { request: new Request(url), params: {}, context: {} } as unknown as LoaderFunctionArgs;
}

async function runGuard(url = "http://localhost/customers?state=active") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  try {
    const me = await requireAdminLoader(qc)(args(url));
    return { me, qc };
  } catch (thrown) {
    return { thrown, qc };
  }
}

function meReturns(status: number, code?: string) {
  server.use(
    http.get(`${API}/admin/v1/me`, () =>
      status === 200
        ? HttpResponse.json(adminMe())
        : HttpResponse.json(problem(status, code ?? "x"), {
            status,
            headers: { "Content-Type": "application/problem+json" },
          }),
    ),
  );
}

describe("RequireAdmin guard redirect matrix", () => {
  const returnTo = encodeURIComponent("/customers?state=active");

  it.each([
    [401, "unauthenticated", `/login?return_to=${returnTo}`],
    [403, "aal2_required", `/login?aal=aal2&return_to=${returnTo}`],
    [403, "mfa_enrollment_required", "/settings?enroll=totp"],
    [403, "not_admin", "/no-access"],
    [403, "forbidden", "/no-access"],
  ])("%i %s -> %s", async (status, code, location) => {
    meReturns(status, code);
    const { thrown } = await runGuard();
    expect(thrown).toBeInstanceOf(Response);
    const res = thrown as Response;
    expect(res.status).toBe(302);
    expect(res.headers.get("Location")).toBe(location);
  });

  it("200 stores roles and permissions in the query cache", async () => {
    meReturns(200);
    const { me, qc, thrown } = await runGuard();
    expect(thrown).toBeUndefined();
    expect(me?.permissions).toContain("manage_customers");
    expect(qc.getQueryData(meKeys.all)).toEqual(adminMe());
  });

  it("does not swallow non-auth errors", async () => {
    meReturns(503, "dependency_unavailable");
    const { thrown } = await runGuard();
    expect(thrown).not.toBeInstanceOf(Response);
  });
});

describe("RequireAdmin in the router", () => {
  it("renders the console and hides nav items without permission", async () => {
    meReturns(200);
    server.use(
      http.get(`${API}/admin/v1/customers`, () => HttpResponse.json({ items: [customer(1)] })),
    );
    renderApp("/customers");
    expect(await screen.findByText("Customer 1")).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Main navigation" });
    expect(nav).toHaveTextContent("Customers");
    expect(nav).toHaveTextContent("Audit log");
    expect(nav).not.toHaveTextContent("Admins"); // no manage_admins
  });

  it("not_admin shows the No access page with sign out", async () => {
    meReturns(403, "not_admin");
    const { router } = renderApp("/customers");
    expect(await screen.findByRole("heading", { name: "No access" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/no-access");
  });
});
