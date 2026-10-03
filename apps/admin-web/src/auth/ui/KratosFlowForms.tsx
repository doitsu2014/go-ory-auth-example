import type { UiContainer, UiNode } from "@ory/client-fetch";
import { type SyntheticEvent, useId } from "react";
import { useTranslation } from "react-i18next";

import { kratosText } from "../../i18n/kratos";
import { Alert } from "../../shared/ui/Alert";
import { type FlowBody, collectBody } from "./collectBody";
import { groupNodes } from "./groupNodes";
import { KratosNode } from "./KratosNode";

export interface KratosFlowFormsProps {
  ui: UiContainer;
  submitting: boolean;
  onSubmit: (body: FlowBody, group: string) => void;
  /** Render only these groups (default: all non-default groups). */
  groups?: string[];
  /** Show a heading per group (settings page). */
  groupTitles?: boolean;
  /** Hide the flow-level `ui.messages` (when the page renders them itself). */
  hideMessages?: boolean;
}

export function FlowMessages({ ui }: { ui: UiContainer }) {
  const { t } = useTranslation();
  const messages = ui.messages ?? [];
  if (messages.length === 0) return null;
  return (
    <div className="space-y-2">
      {messages.map((m) => (
        <Alert
          key={`${String(m.id)}-${m.text}`}
          kind={m.type === "error" ? "error" : m.type === "success" ? "success" : "info"}
        >
          <span data-message-id={m.id}>{kratosText(t, m)}</span>
        </Alert>
      ))}
    </div>
  );
}

function GroupForm({
  group,
  nodes,
  submitting,
  onSubmit,
  title,
}: {
  group: string;
  nodes: UiNode[];
  submitting: boolean;
  onSubmit: (body: FlowBody, group: string) => void;
  title?: string;
}) {
  const prefix = useId();
  const titleId = `${prefix}-title`;

  function handleSubmit(e: SyntheticEvent<HTMLFormElement>) {
    e.preventDefault();
    const native = e.nativeEvent as SubmitEvent;
    const submitter =
      native.submitter instanceof HTMLButtonElement
        ? { name: native.submitter.name, value: native.submitter.value }
        : null; // falls back to the form's first submit node
    onSubmit(collectBody(e.currentTarget, nodes, submitter), group);
  }

  return (
    <form
      // Kratos validates server-side (400 + messages). Native validation would
      // block secondary buttons such as "Resend code" when `code` is required.
      noValidate
      onSubmit={handleSubmit}
      className="space-y-4"
      data-group={group}
      aria-labelledby={title ? titleId : undefined}
    >
      {title ? (
        <h2 id={titleId} className="text-base font-semibold text-slate-900">
          {title}
        </h2>
      ) : null}
      {nodes.map((node, i) => (
        <KratosNode
          key={`${node.group}-${String(i)}`}
          node={node}
          idPrefix={`${prefix}-${group}`}
          submitting={submitting}
        />
      ))}
    </form>
  );
}

/**
 * Generic renderer for a Kratos flow's `ui` (inputs incl. hidden csrf_token,
 * submit buttons with name/value, img for the TOTP QR, text nodes for
 * secrets, messages). Submission goes through `onSubmit` as JSON (AJAX
 * browser flow) — the form never posts natively.
 */
export function KratosFlowForms({
  ui,
  submitting,
  onSubmit,
  groups,
  groupTitles,
  hideMessages,
}: KratosFlowFormsProps) {
  const { t } = useTranslation();
  const parts = groupNodes(ui.nodes).filter((p) => !groups || groups.includes(p.group));
  return (
    <div className="space-y-6">
      {hideMessages ? null : <FlowMessages ui={ui} />}
      {parts.map((p, i) => (
        <div key={p.group} className={i > 0 ? "border-t border-slate-200 pt-6" : undefined}>
          <GroupForm
            group={p.group}
            nodes={p.nodes}
            submitting={submitting}
            onSubmit={onSubmit}
            title={groupTitles ? t(`auth.groups.${p.group}`, { defaultValue: p.group }) : undefined}
          />
        </div>
      ))}
    </div>
  );
}
