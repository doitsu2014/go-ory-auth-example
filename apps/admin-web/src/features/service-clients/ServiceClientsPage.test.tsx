import type { QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Permission, ServiceClient } from "../../api/client";
import { adminMe, loginFlow, passwordLoginNodes, problem } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, KRATOS, server } from "../../test/server";

const SECRET = "s3cr3t-Value_only-shown-once-xyz";
const ROTATED_SECRET = "r0tated-Secret_value-abc";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

const BASE: Permission[] = ["view_customers", "manage_customers", "manage_admins", "view_audit"];

function client(i: number, overrides: Partial<ServiceClient> = {}): ServiceClient {
  return {
    client_id: `9a1b0000-0000-4000-8000-${String(i).padStart(12, "0")}`,
    name: `billing-sync-${String(i)}`,
    owner: `team${String(i)}@example.com`,
    scopes: ["customers:read"],
    created_at: "2026-10-03T08:00:00Z",
    ...overrides,
  };
}

const C1 = client(1, { scopes: ["customers:read", "audit:read"] });
const NEW = client(9, { name: "reporting", owner: "data@example.com", scopes: ["audit:read"] });

interface Recorded {
  method: string;
  path: string;
  headers: Headers;
  body: unknown;
}

function setup(
  opts: {
    permissions?: Permission[];
    listStatus?: number;
    create?: { status: number; code?: string; omitSecret?: boolean };
    rotate?: { status: number; code?: string };
    remove?: { status: number; code?: string };
  } = {},
) {
  const calls: Recorded[] = [];
  let items: ServiceClient[] = [C1];
  async function record(request: Request) {
    const text = await request.text();
    calls.push({
      method: request.method,
      path: new URL(request.url).pathname,
      headers: request.headers,
      body: text ? (JSON.parse(text) as unknown) : undefined,
    });
  }
  function fail(s: { status: number; code?: string }) {
    return HttpResponse.json(problem(s.status, s.code ?? "internal"), { status: s.status });
  }
  server.use(
    http.get(`${API}/admin/v1/me`, () =>
      HttpResponse.json(
        adminMe({ permissions: opts.permissions ?? [...BASE, "manage_service_clients"] }),
      ),
    ),
    http.get(`${API}/admin/v1/customers`, () => HttpResponse.json({ items: [] })),
    http.get(`${API}/admin/v1/service-clients`, async ({ request }) => {
      await record(request);
      if (opts.listStatus) return fail({ status: opts.listStatus, code: "dependency_unavailable" });
      return HttpResponse.json({ items });
    }),
    http.post(`${API}/admin/v1/service-clients`, async ({ request }) => {
      await record(request);
      if (opts.create && opts.create.status !== 201) return fail(opts.create);
      if (!items.includes(NEW)) items = [...items, NEW];
      return HttpResponse.json(opts.create?.omitSecret ? NEW : { ...NEW, client_secret: SECRET }, {
        status: 201,
      });
    }),
    http.post(`${API}/admin/v1/service-clients/:id/rotate-secret`, async ({ request }) => {
      await record(request);
      if (opts.rotate && opts.rotate.status !== 200) return fail(opts.rotate);
      return HttpResponse.json({ ...C1, client_secret: ROTATED_SECRET });
    }),
    http.delete(`${API}/admin/v1/service-clients/:id`, async ({ request, params }) => {
      await record(request);
      if (opts.remove && opts.remove.status !== 204) return fail(opts.remove);
      items = items.filter((c) => c.client_id !== params.id);
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return { calls };
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

function spyConsole() {
  return (["log", "info", "warn", "error", "debug"] as const).map((m) =>
    vi.spyOn(console, m).mockImplementation(() => undefined),
  );
}

function consoleDump(spies: ReturnType<typeof spyConsole>): string {
  return JSON.stringify(spies.map((s) => s.mock.calls));
}

/** The only way out: tick "I have stored the secret", then Close. */
async function closeSecret(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement) {
  const close = within(dialog).getByRole("button", { name: "Close" });
  expect(close).toBeDisabled();
  await user.click(within(dialog).getByLabelText("I have stored the secret"));
  await user.click(close);
}

function posts(calls: Recorded[]): Recorded[] {
  return calls.filter((c) => c.method === "POST");
}

async function fillCreate(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Create client" }));
  const dialog = screen.getByRole("dialog", { name: "Create a service client" });
  return dialog;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ServiceClientsPage gating", () => {
  it("shows the nav entry only with manage_service_clients", async () => {
    setup();
    renderApp("/customers");
    expect(await screen.findByRole("link", { name: "Service clients" })).toHaveAttribute(
      "href",
      "/service-clients",
    );
  });

  it("hides the nav entry and does not call the API without the permission", async () => {
    const { calls } = setup({ permissions: BASE });
    renderApp("/service-clients");
    expect(
      await screen.findByText("You do not have permission to manage service clients."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Admins" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Service clients" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Create client" })).toBeNull();
    expect(calls).toHaveLength(0);
  });
});

describe("ServiceClientsPage list", () => {
  it("lists name, client_id, owner, scopes and created_at", async () => {
    setup();
    renderApp("/service-clients");
    const row = (await screen.findByText(C1.name)).closest("tr")!;
    expect(within(row).getByText(C1.client_id)).toBeInTheDocument();
    expect(within(row).getByText(C1.owner)).toBeInTheDocument();
    expect(within(row).getByText("customers:read audit:read")).toBeInTheDocument();
    expect(within(row).getByText(/2026/)).toBeInTheDocument();
  });

  it("shows a localised error when the list is unavailable (503)", async () => {
    setup({ listStatus: 503 });
    renderApp("/service-clients");
    expect(
      await screen.findByText(
        "The authorization server is unavailable, so nothing was changed. Please try again later.",
        {},
        { timeout: 5000 },
      ),
    ).toBeInTheDocument();
  });
});

describe("ServiceClientsPage create", () => {
  it("validates like the server, then sends the body and a UUID Idempotency-Key", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    const dialog = await fillCreate(user);

    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(await within(dialog).findAllByText("This field is required.")).toHaveLength(2);
    expect(within(dialog).getByText("Select at least one scope.")).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText("Name"), "ab");
    await user.type(within(dialog).getByLabelText("Owner"), "not-an-email");
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(await within(dialog).findByText("Too short (min 3 characters).")).toBeInTheDocument();
    expect(within(dialog).getByText("Enter a valid email address.")).toBeInTheDocument();

    await user.clear(within(dialog).getByLabelText("Name"));
    await user.type(within(dialog).getByLabelText("Name"), "Bad_Name");
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(
      await within(dialog).findByText(
        "Use lowercase letters, digits and dashes only, starting with a letter or a digit.",
      ),
    ).toBeInTheDocument();
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(0);

    await user.clear(within(dialog).getByLabelText("Name"));
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.clear(within(dialog).getByLabelText("Owner"));
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));

    await waitFor(() => expect(calls.filter((c) => c.method === "POST")).toHaveLength(1));
    const post = calls.find((c) => c.method === "POST")!;
    expect(post.path).toBe("/admin/v1/service-clients");
    expect(post.body).toEqual({
      name: "reporting",
      owner: "data@example.com",
      scopes: ["audit:read"],
    });
    expect(post.headers.get("Idempotency-Key")).toMatch(UUID);
    expect(await screen.findByRole("dialog", { name: "Client reporting created" })).toBeVisible();
  });

  it("reuses the Idempotency-Key on retries and uses a new one when reopened", async () => {
    const { calls } = setup({ create: { status: 503, code: "dependency_unavailable" } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    let dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/customers:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(
      await within(dialog).findByText(
        "The authorization server is unavailable, so nothing was changed. Please try again later.",
      ),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(2));

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/customers:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(3));

    const [a, b, c] = posts(calls).map((p) => p.headers.get("Idempotency-Key"));
    expect(a).toMatch(UUID);
    expect(b).toBe(a);
    expect(c).toMatch(UUID);
    expect(c).not.toBe(a);
  });

  it("uses a new Idempotency-Key when the values change after a failed attempt", async () => {
    const { calls } = setup({ create: { status: 503, code: "dependency_unavailable" } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/customers:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(1));
    await within(dialog).findByRole("alert");

    await user.type(within(dialog).getByLabelText("Name"), "-v2");
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(2));
    // Unchanged retry of the edited values keeps the new key.
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(posts(calls)).toHaveLength(3));

    const [a, b, c] = posts(calls).map((p) => p.headers.get("Idempotency-Key"));
    expect((posts(calls)[1]!.body as { name: string }).name).toBe("reporting-v2");
    expect(b).toMatch(UUID);
    expect(b).not.toBe(a);
    expect(c).toBe(b);
  });

  it("falls back to the generic message for an unexpected status (400)", async () => {
    setup({ create: { status: 400, code: "invalid_request" } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(await within(dialog).findByText("Request failed (invalid_request).")).toBeVisible();
  });

  it("uses a new Idempotency-Key for the next create after a success", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    for (const name of ["reporting", "reporting-two"]) {
      const dialog = await fillCreate(user);
      await user.type(within(dialog).getByLabelText("Name"), name);
      await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
      await user.click(within(dialog).getByLabelText(/audit:read/));
      await user.click(within(dialog).getByRole("button", { name: "Create client" }));
      await closeSecret(user, await screen.findByRole("dialog", { name: /created|New secret/ }));
    }
    const [a, b] = posts(calls).map((p) => p.headers.get("Idempotency-Key"));
    expect(a).toMatch(UUID);
    expect(b).toMatch(UUID);
    expect(b).not.toBe(a);
  });

  it("shows the secret once, copies it, and leaves no trace after close", async () => {
    const spies = spyConsole();
    setup();
    const user = userEvent.setup();
    const { queryClient } = renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));

    const secretDialog = await screen.findByRole("dialog", { name: "Client reporting created" });
    expect(within(secretDialog).getByText(NEW.client_id)).toBeInTheDocument();
    expect(within(secretDialog).getByTestId("client-secret")).toHaveTextContent(SECRET);
    expect(within(secretDialog).getByText(/It won't be shown again/)).toBeInTheDocument();
    expect(within(secretDialog).queryByRole("button", { name: "Cancel" })).toBeNull();

    // The token example uses a placeholder, never the secret.
    const example = within(secretDialog).getByTestId("token-example").textContent;
    expect(example).toContain("http://localhost:4444/oauth2/token");
    expect(example).toContain("grant_type=client_credentials");
    expect(example).toContain('scope="audit:read"');
    expect(example).toContain("audience=identity-service");
    expect(example).toContain(`${NEW.client_id}:$CLIENT_SECRET`);
    expect(example).not.toContain(SECRET);

    await user.click(within(secretDialog).getByRole("button", { name: "Copy secret" }));
    expect(await within(secretDialog).findByText("Secret copied to the clipboard.")).toBeVisible();
    expect(await navigator.clipboard.readText()).toBe(SECRET);

    // While shown: still only in component state.
    expect(cacheDump(queryClient)).not.toContain(SECRET);
    expect(storageDump()).not.toContain(SECRET);

    // The list refetched (without any secret) and shows the new client.
    expect(await screen.findByText(NEW.name)).toBeInTheDocument();

    await closeSecret(user, secretDialog);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByText(SECRET)).toBeNull();
    expect(document.body.innerHTML).not.toContain(SECRET);
    expect(cacheDump(queryClient)).not.toContain(SECRET);
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
    expect(storageDump()).not.toContain(SECRET);
    expect(consoleDump(spies)).not.toContain(SECRET);
  });

  it("ignores Escape on the secret dialog and drops the secret on unmount", async () => {
    setup();
    const user = userEvent.setup();
    const { queryClient, router, unmount } = renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await screen.findByTestId("client-secret");

    await user.keyboard("{Escape}");
    expect(screen.getByTestId("client-secret")).toHaveTextContent(SECRET);
    await closeSecret(user, screen.getByRole("dialog", { name: "Client reporting created" }));
    expect(screen.queryByTestId("client-secret")).toBeNull();

    await act(() => router.navigate("/customers"));
    await act(() => router.navigate("/service-clients"));
    expect(await screen.findByText(NEW.name)).toBeInTheDocument();
    expect(screen.queryByText(SECRET)).toBeNull();
    expect(router.state.location.state).toBeNull();

    unmount();
    expect(cacheDump(queryClient)).not.toContain(SECRET);
    expect(storageDump()).not.toContain(SECRET);
  });

  it("explains a replayed create that returns no secret", async () => {
    setup({ create: { status: 201, omitSecret: true } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    const secretDialog = await screen.findByRole("dialog", { name: "Client reporting created" });
    expect(within(secretDialog).queryByTestId("client-secret")).toBeNull();
    expect(within(secretDialog).getByText(/Rotate the secret to get a new one/)).toBeVisible();
    // Nothing to store: Close is available without the acknowledgement.
    await user.click(within(secretDialog).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it.each([
    [403, "forbidden", "You do not have permission to manage service clients."],
    [
      409,
      "conflict",
      "This conflicts with an existing service client or a request still in progress. Refresh the list and try again.",
    ],
    [422, "validation_failed", "Some fields are invalid. Check the name, owner and scopes."],
    [
      503,
      "dependency_unavailable",
      "The authorization server is unavailable, so nothing was changed. Please try again later.",
    ],
  ])("shows a localised error in the create dialog on %i %s", async (status, code, message) => {
    setup({ create: { status, code } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    expect(await within(dialog).findByText(message)).toBeInTheDocument();
    expect(screen.queryByTestId("client-secret")).toBeNull();
  });

  it("routes to sign-in on 401", async () => {
    setup({ create: { status: 401, code: "unauthenticated" } });
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () =>
        HttpResponse.json(loginFlow("66666666-6666-4666-8666-666666666666", passwordLoginNodes())),
      ),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/service-clients");
    const dialog = await fillCreate(user);
    await user.type(within(dialog).getByLabelText("Name"), "reporting");
    await user.type(within(dialog).getByLabelText("Owner"), "data@example.com");
    await user.click(within(dialog).getByLabelText(/audit:read/));
    await user.click(within(dialog).getByRole("button", { name: "Create client" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(router.state.location.search).toContain("return_to=%2Fservice-clients");
  });
});

describe("ServiceClientsPage rotate", () => {
  it("confirms, rotates, then shows the new secret once", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    const { queryClient } = renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Rotate secret of ${C1.name}` }));
    const confirm = screen.getByRole("dialog", { name: `Rotate the secret of ${C1.name}?` });
    expect(within(confirm).getByText(/stop working immediately/)).toBeInTheDocument();
    const example = within(confirm).getByTestId("token-example").textContent;
    expect(example).toContain(`${C1.client_id}:$CLIENT_SECRET`);
    expect(example).toContain('scope="customers:read audit:read"');

    await user.click(within(confirm).getByRole("button", { name: "Rotate secret" }));
    const secretDialog = await screen.findByRole("dialog", { name: `New secret for ${C1.name}` });
    expect(within(secretDialog).getByTestId("client-secret")).toHaveTextContent(ROTATED_SECRET);
    expect(calls.filter((c) => c.method === "POST").map((c) => c.path)).toEqual([
      `/admin/v1/service-clients/${C1.client_id}/rotate-secret`,
    ]);
    expect(cacheDump(queryClient)).not.toContain(ROTATED_SECRET);

    await closeSecret(user, secretDialog);
    expect(screen.queryByText(ROTATED_SECRET)).toBeNull();
    expect(cacheDump(queryClient)).not.toContain(ROTATED_SECRET);
    expect(storageDump()).not.toContain(ROTATED_SECRET);
  });

  it("does nothing when cancelled", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Rotate secret of ${C1.name}` }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(calls.filter((c) => c.method === "POST")).toHaveLength(0);
  });

  it.each([
    [404, "not_found", "This service client no longer exists. Refresh the list."],
    [
      503,
      "dependency_unavailable",
      "The authorization server is unavailable, so nothing was changed. Please try again later.",
    ],
  ])("shows a localised error on %i %s", async (status, code, message) => {
    setup({ rotate: { status, code } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Rotate secret of ${C1.name}` }));
    const confirm = screen.getByRole("dialog", { name: `Rotate the secret of ${C1.name}?` });
    await user.click(within(confirm).getByRole("button", { name: "Rotate secret" }));
    expect(await within(confirm).findByText(message)).toBeInTheDocument();
    expect(screen.queryByTestId("client-secret")).toBeNull();
  });
});

describe("ServiceClientsPage delete", () => {
  it("deletes only after confirmation and refreshes the list", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Delete ${C1.name}` }));
    const confirm = screen.getByRole("dialog", { name: `Delete ${C1.name}?` });
    expect(within(confirm).getByText(/cannot be undone/)).toBeInTheDocument();
    expect(within(confirm).getByTestId("token-example").textContent).toContain("$CLIENT_SECRET");
    expect(calls.filter((c) => c.method === "DELETE")).toHaveLength(0);

    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    expect(await screen.findByText(`Service client ${C1.name} deleted.`)).toBeInTheDocument();
    expect(calls.filter((c) => c.method === "DELETE").map((c) => c.path)).toEqual([
      `/admin/v1/service-clients/${C1.client_id}`,
    ]);
    expect(await screen.findByText("No service clients yet.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the client when cancelled", async () => {
    const { calls } = setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Delete ${C1.name}` }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(calls.filter((c) => c.method === "DELETE")).toHaveLength(0);
    expect(screen.getByText(C1.name)).toBeInTheDocument();
  });

  it.each([
    [403, "forbidden", "You do not have permission to manage service clients."],
    [404, "not_found", "This service client no longer exists. Refresh the list."],
    [
      503,
      "dependency_unavailable",
      "The authorization server is unavailable, so nothing was changed. Please try again later.",
    ],
  ])("shows a localised error on %i %s", async (status, code, message) => {
    setup({ remove: { status, code } });
    const user = userEvent.setup();
    renderApp("/service-clients");
    await user.click(await screen.findByRole("button", { name: `Delete ${C1.name}` }));
    const confirm = screen.getByRole("dialog", { name: `Delete ${C1.name}?` });
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    expect(await within(confirm).findByText(message)).toBeInTheDocument();
  });
});

describe("ServiceClientsPage i18n", () => {
  it("is shown in Vietnamese", async () => {
    setup();
    const user = userEvent.setup();
    renderApp("/service-clients");
    await screen.findByText(C1.name);
    const { default: i18n } = await import("../../i18n");
    await act(() => i18n.changeLanguage("vi"));
    expect(screen.getByRole("heading", { name: "Ứng dụng dịch vụ" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Ứng dụng dịch vụ" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tạo ứng dụng" }));
    expect(screen.getByRole("dialog", { name: "Tạo ứng dụng dịch vụ" })).toBeInTheDocument();
  });
});
