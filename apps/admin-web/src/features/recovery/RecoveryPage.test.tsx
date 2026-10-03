import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { CSRF, csrfNode, inputNode, loginFlow, text } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { KRATOS, server } from "../../test/server";

const RID = "22222222-2222-4222-8222-222222222222";
const SID = "33333333-3333-4333-8333-333333333333";

// Real Kratos v26.2 node layout (verified against the local stack).
function sentEmailFlow() {
  return {
    ...loginFlow(
      RID,
      [
        csrfNode(),
        inputNode("code", "code", "text", {
          label: text(1070010, "Recovery code"),
          required: true,
        }),
        inputNode("code", "method", "hidden", { value: "code" }),
        inputNode("code", "method", "submit", { value: "code", label: text(1070009, "Continue") }),
        inputNode("code", "email", "submit", {
          value: "a@example.com",
          label: text(1070008, "Resend code"),
        }),
      ],
      [text(1060003, "An email containing a recovery code has been sent.")],
    ),
    type: "browser",
    state: "sent_email",
  };
}

function recoveryWith(onCode: () => Response) {
  const bodies: Record<string, unknown>[] = [];
  server.use(
    http.get(`${KRATOS}/self-service/recovery/browser`, () => HttpResponse.json(sentEmailFlow())),
    http.post(`${KRATOS}/self-service/recovery`, async ({ request }) => {
      bodies.push((await request.json()) as Record<string, unknown>);
      return onCode();
    }),
    http.get(`${KRATOS}/self-service/settings/flows`, () =>
      HttpResponse.json({
        ...loginFlow(SID, [csrfNode()]),
        state: "show_form",
        identity: { id: "i", schema_id: "admin", traits: {} },
      }),
    ),
  );
  return bodies;
}

describe("RecoveryPage", () => {
  it("follows 422 browser_location_change_required into the settings flow", async () => {
    const bodies = recoveryWith(() =>
      HttpResponse.json(
        {
          error: { id: "browser_location_change_required", code: 422 },
          redirect_browser_to: `${window.location.origin}/settings?flow=${SID}`,
        },
        { status: 422 },
      ),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/recovery");
    await user.type(await screen.findByLabelText("Recovery code"), "123456");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(router.state.location.pathname).toBe("/settings"));
    expect(router.state.location.search).toBe(`?flow=${SID}`);
    expect(bodies[0]).toEqual({ csrf_token: CSRF, code: "123456", method: "code" });
  });

  it("resend submits the email button without native validation blocking it", async () => {
    const bodies = recoveryWith(() => HttpResponse.json(sentEmailFlow()));
    const user = userEvent.setup();
    renderApp("/recovery");
    await user.click(await screen.findByRole("button", { name: "Resend code" }));
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toMatchObject({ csrf_token: CSRF, method: "code", email: "a@example.com" });
  });

  it("follows continue_with show_settings_ui", async () => {
    recoveryWith(() =>
      HttpResponse.json({
        ...sentEmailFlow(),
        state: "passed_challenge",
        continue_with: [{ action: "show_settings_ui", flow: { id: SID } }],
      }),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/recovery");
    await user.type(await screen.findByLabelText("Recovery code"), "123456");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(router.state.location.search).toBe(`?flow=${SID}`));
  });
});
