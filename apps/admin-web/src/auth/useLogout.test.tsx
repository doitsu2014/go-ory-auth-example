import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { loginFlow, passwordLoginNodes } from "../test/fixtures";
import { renderApp } from "../test/render";
import { KRATOS, server } from "../test/server";

function loginCreate() {
  return http.get(`${KRATOS}/self-service/login/browser`, () =>
    HttpResponse.json(loginFlow("66666666-6666-4666-8666-666666666666", passwordLoginNodes())),
  );
}

describe("useLogout", () => {
  it("logs out via the browser logout flow and clears the cache", async () => {
    const tokens: (string | null)[] = [];
    server.use(
      http.get(`${KRATOS}/self-service/logout/browser`, () =>
        HttpResponse.json({
          logout_url: `${KRATOS}/self-service/logout?token=t1`,
          logout_token: "t1",
        }),
      ),
      http.get(`${KRATOS}/self-service/logout`, ({ request }) => {
        tokens.push(new URL(request.url).searchParams.get("token"));
        return new HttpResponse(null, { status: 204 });
      }),
      loginCreate(),
    );
    const { router, queryClient } = renderApp("/no-access");
    queryClient.setQueryData(["admin-me"], { id: "x" });
    await userEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(tokens).toEqual(["t1"]);
    expect(queryClient.getQueryData(["admin-me"])).toBeUndefined();
  });

  it("treats 401 (no session) as already signed out", async () => {
    server.use(
      http.get(`${KRATOS}/self-service/logout/browser`, () =>
        HttpResponse.json({ error: { id: "session_inactive", code: 401 } }, { status: 401 }),
      ),
      loginCreate(),
    );
    const { router } = renderApp("/no-access");
    await userEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
  });

  it("on other failures stays put, keeps the cache and shows an error toast", async () => {
    server.use(
      http.get(`${KRATOS}/self-service/logout/browser`, () =>
        HttpResponse.json({ error: { code: 500 } }, { status: 500 }),
      ),
    );
    const { router, queryClient } = renderApp("/no-access");
    queryClient.setQueryData(["admin-me"], { id: "x" });
    const button = await screen.findByRole("button", { name: "Sign out" });
    await userEvent.click(button);
    expect(await screen.findByText(/Sign out failed/)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/no-access");
    expect(queryClient.getQueryData(["admin-me"])).toEqual({ id: "x" });
    expect(button).toBeEnabled(); // can retry
  });
});
