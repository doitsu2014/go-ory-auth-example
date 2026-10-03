import type { UiContainer, UiNode } from "@ory/client-fetch";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import i18n from "../../i18n";
import { CSRF, inputNode, passwordLoginNodes, text } from "../../test/fixtures";
import { KratosFlowForms } from "./KratosFlowForms";

const QR = "data:image/png;base64,iVBORw0KGgo=";

function settingsUi(): UiContainer {
  const nodes: UiNode[] = [
    inputNode("default", "csrf_token", "hidden", { value: CSRF }),
    {
      type: "img",
      group: "totp",
      attributes: { node_type: "img", id: "totp_qr", src: QR, width: 256, height: 256 },
      messages: [],
      meta: { label: text(1050005, "Authenticator app QR code") },
    } as unknown as UiNode,
    {
      type: "text",
      group: "totp",
      attributes: {
        node_type: "text",
        id: "totp_secret_key",
        text: text(1050006, "JBSWY3DPEHPK3PXP"),
      },
      messages: [],
      meta: { label: text(1050017, "This is your authenticator app secret.") },
    } as unknown as UiNode,
    inputNode("totp", "totp_code", "text", { label: text(1070006, "Verify code") }),
    inputNode("totp", "method", "submit", { value: "totp", label: text(1070003, "Save") }),
    {
      type: "text",
      group: "lookup_secret",
      attributes: {
        node_type: "text",
        id: "lookup_secret_codes",
        text: text(1050015, "abcd1234, efgh5678", "info", {
          secrets: [text(1050009, "abcd1234"), text(1050009, "efgh5678")],
        }),
      },
      messages: [],
      meta: {},
    } as unknown as UiNode,
    inputNode("lookup_secret", "lookup_secret_confirm", "submit", {
      value: true,
      label: text(1050011, "Confirm backup recovery codes"),
    }),
    {
      type: "script",
      group: "webauthn",
      attributes: { node_type: "script", id: "webauthn_script", src: "https://evil.example/x.js" },
      messages: [],
      meta: {},
    } as unknown as UiNode,
  ];
  return {
    action: "",
    method: "POST",
    nodes,
    messages: [text(1050001, "Your changes have been saved!", "success")],
  };
}

describe("KratosFlowForms (generic ui.nodes renderer)", () => {
  it("renders inputs with labels, hidden csrf_token, node and flow messages", () => {
    const ui: UiContainer = {
      action: "",
      method: "POST",
      nodes: passwordLoginNodes({
        identifier: [
          text(4000002, "Property identifier is missing.", "error", { property: "identifier" }),
        ],
      }),
      messages: [
        text(4000006, "The provided credentials are invalid.", "error"),
        text(9999999, "Unmapped text", "info"),
      ],
    };
    const { container } = render(<KratosFlowForms ui={ui} submitting={false} onSubmit={vi.fn()} />);

    const csrf = container.querySelector<HTMLInputElement>('input[name="csrf_token"]');
    expect(csrf).toHaveAttribute("type", "hidden");
    expect(csrf).toHaveValue(CSRF);

    expect(screen.getByLabelText("E-Mail")).toHaveAttribute("name", "identifier");
    expect(screen.getByLabelText("Password")).toHaveAttribute("type", "password");
    const submit = screen.getByRole("button", { name: "Sign in with password" });
    expect(submit).toHaveAttribute("name", "method");
    expect(submit).toHaveValue("password");

    // Messages render by id; unknown ids fall back to Kratos text.
    expect(screen.getByText("Property identifier is missing.")).toBeInTheDocument();
    expect(screen.getByText("The provided credentials are invalid.")).toBeInTheDocument();
    expect(screen.getByText("Unmapped text")).toBeInTheDocument();
    expect(screen.getByLabelText("E-Mail")).toHaveAttribute("aria-invalid", "true");
  });

  it("translates Kratos message ids to Vietnamese with context interpolation", async () => {
    await i18n.changeLanguage("vi");
    const ui: UiContainer = {
      action: "",
      method: "POST",
      nodes: passwordLoginNodes({
        identifier: [
          text(4000002, "Property identifier is missing.", "error", { property: "identifier" }),
        ],
      }),
      messages: [text(4000006, "The provided credentials are invalid.", "error")],
    };
    render(<KratosFlowForms ui={ui} submitting={false} onSubmit={vi.fn()} />);
    expect(screen.getByText("Thiếu trường identifier.")).toBeInTheDocument();
    expect(screen.getByText(/Thông tin đăng nhập không hợp lệ/)).toBeInTheDocument();
    expect(screen.getByLabelText("Mật khẩu")).toBeInTheDocument();
    // generated label (1070002) keeps Kratos text
    expect(screen.getByLabelText("E-Mail")).toBeInTheDocument();
  });

  it("renders TOTP QR as <img data:>, secrets as text, ignores scripts", () => {
    const { container } = render(
      <KratosFlowForms ui={settingsUi()} submitting={false} onSubmit={vi.fn()} groupTitles />,
    );
    const img = screen.getByRole("img", { name: "QR code to scan with your authenticator app" });
    expect(img).toHaveAttribute("src", QR);
    expect(screen.getByText("JBSWY3DPEHPK3PXP")).toBeInTheDocument();
    expect(screen.getByText("abcd1234")).toBeInTheDocument();
    expect(screen.getByText("efgh5678")).toBeInTheDocument();
    expect(screen.getByText("Your changes have been saved!")).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    // one form per group, each with the csrf token
    expect(container.querySelectorAll("form")).toHaveLength(2);
    expect(container.querySelectorAll('input[name="csrf_token"]')).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "Authenticator app (TOTP)" })).toBeInTheDocument();
  });

  it("refuses non-data image sources", () => {
    const ui: UiContainer = {
      action: "",
      method: "POST",
      nodes: [
        {
          type: "img",
          group: "totp",
          attributes: {
            node_type: "img",
            id: "x",
            src: "https://tracker.example/x.png",
            width: 1,
            height: 1,
          },
          messages: [],
          meta: {},
        } as unknown as UiNode,
      ],
    };
    render(<KratosFlowForms ui={ui} submitting={false} onSubmit={vi.fn()} />);
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("submits a JSON body with csrf_token and the clicked button's typed value", async () => {
    const onSubmit = vi.fn();
    render(<KratosFlowForms ui={settingsUi()} submitting={false} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole("button", { name: "Confirm backup recovery codes" }));
    expect(onSubmit).toHaveBeenCalledWith(
      { csrf_token: CSRF, lookup_secret_confirm: true },
      "lookup_secret",
    );

    await userEvent.type(screen.getByLabelText("Verify code"), "123456");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSubmit).toHaveBeenLastCalledWith(
      { csrf_token: CSRF, totp_code: "123456", method: "totp" },
      "totp",
    );
  });
});
