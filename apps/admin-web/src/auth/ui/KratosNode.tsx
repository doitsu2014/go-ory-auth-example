import {
  type UiNode,
  type UiText,
  isUiNodeAnchorAttributes,
  isUiNodeImageAttributes,
  isUiNodeInputAttributes,
  isUiNodeTextAttributes,
} from "@ory/client-fetch";
import { useTranslation } from "react-i18next";

import { kratosText } from "../../i18n/kratos";
import { Button } from "../../shared/ui/Button";
import { cx } from "../../shared/ui/cx";

/** Text ids whose `text` is data (secrets / codes), never translated. */
const DATA_TEXT_IDS = new Set([1050006, 1050009, 1050015]);

const messageStyles: Record<string, string> = {
  error: "text-red-700",
  success: "text-green-700",
  info: "text-slate-600",
};

export function NodeMessages({ id, messages }: { id?: string; messages: UiText[] }) {
  const { t } = useTranslation();
  if (messages.length === 0) return null;
  return (
    <div id={id}>
      {messages.map((m) => (
        <p
          key={`${String(m.id)}-${m.text}`}
          className={cx("text-xs", messageStyles[m.type] ?? messageStyles.info)}
          data-message-id={m.id}
          role={m.type === "error" ? "alert" : undefined}
        >
          {kratosText(t, m)}
        </p>
      ))}
    </div>
  );
}

function isSafeImageSrc(src: string): boolean {
  return /^data:image\/(png|jpeg|gif|svg\+xml|webp);base64,/i.test(src);
}

function isSafeHref(href: string): boolean {
  try {
    const u = new URL(href, window.location.origin);
    return u.protocol === "https:" || u.protocol === "http:";
  } catch {
    return false;
  }
}

function secretsOf(text: UiText): UiText[] | undefined {
  const ctx = text.context as { secrets?: unknown } | undefined;
  return Array.isArray(ctx?.secrets) ? (ctx.secrets as UiText[]) : undefined;
}

export interface KratosNodeProps {
  node: UiNode;
  /** Unique per form so ids don't collide when a node is rendered in several forms. */
  idPrefix: string;
  submitting: boolean;
}

/**
 * Renders one Kratos `ui.node`. Scripts and unknown node types are ignored
 * (CSP: no inline/remote scripts; WebAuthn/passkeys are not enabled).
 */
export function KratosNode({ node, idPrefix, submitting }: KratosNodeProps) {
  const { t } = useTranslation();
  const attrs = node.attributes;
  const label = node.meta.label ? kratosText(t, node.meta.label) : undefined;

  if (isUiNodeImageAttributes(attrs)) {
    if (!isSafeImageSrc(attrs.src)) return null;
    return (
      <figure className="flex flex-col items-center gap-2">
        <img
          src={attrs.src}
          width={attrs.width}
          height={attrs.height}
          alt={t("auth.totpQrAlt")}
          className="rounded bg-white p-2 ring-1 ring-slate-200"
        />
        {label ? <figcaption className="text-xs text-slate-600">{label}</figcaption> : null}
      </figure>
    );
  }

  if (isUiNodeTextAttributes(attrs)) {
    const secrets = secretsOf(attrs.text);
    return (
      <div className="space-y-1" data-node-id={attrs.id}>
        {label ? <p className="text-sm text-slate-700">{label}</p> : null}
        {secrets ? (
          <ul className="grid grid-cols-2 gap-1 rounded bg-slate-100 p-3 font-mono text-sm">
            {secrets.map((s, i) => (
              <li key={`${String(i)}-${s.text}`}>
                {DATA_TEXT_IDS.has(s.id) ? s.text : kratosText(t, s)}
              </li>
            ))}
          </ul>
        ) : (
          <code className="block rounded bg-slate-100 p-2 font-mono text-sm break-all">
            {DATA_TEXT_IDS.has(attrs.text.id) ? attrs.text.text : kratosText(t, attrs.text)}
          </code>
        )}
      </div>
    );
  }

  if (isUiNodeAnchorAttributes(attrs)) {
    if (!isSafeHref(attrs.href)) return null;
    return (
      <a href={attrs.href} id={attrs.id} className="text-sm text-blue-700 underline">
        {kratosText(t, attrs.title)}
      </a>
    );
  }

  if (!isUiNodeInputAttributes(attrs)) {
    // script, div, unknown -> ignored
    return null;
  }

  const id = `${idPrefix}-${attrs.name}`;
  const msgId = node.messages.length > 0 ? `${id}-messages` : undefined;
  const hasError = node.messages.some((m) => m.type === "error");

  switch (attrs.type) {
    case "hidden":
      return <input type="hidden" name={attrs.name} value={String(attrs.value ?? "")} />;
    case "submit":
    case "button": {
      // WebAuthn / passkey triggers need Ory's scripts; not used here.
      if (attrs.onclickTrigger || attrs.onclick) return null;
      return (
        <div className="space-y-1">
          <Button
            type="submit"
            name={attrs.name}
            value={String(attrs.value ?? "")}
            disabled={attrs.disabled || submitting}
            variant={node.group === "default" || attrs.name === "method" ? "primary" : "secondary"}
            className="w-full"
          >
            {label ?? String(attrs.value ?? attrs.name)}
          </Button>
          <NodeMessages id={msgId} messages={node.messages} />
        </div>
      );
    }
    case "checkbox":
      return (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <input
              id={id}
              type="checkbox"
              name={attrs.name}
              defaultChecked={attrs.value === true}
              disabled={attrs.disabled}
              required={attrs.required}
              aria-describedby={msgId}
              aria-invalid={hasError || undefined}
              className="h-4 w-4 rounded border-slate-300"
            />
            <label htmlFor={id} className="text-sm text-slate-700">
              {label ?? attrs.name}
            </label>
          </div>
          <NodeMessages id={msgId} messages={node.messages} />
        </div>
      );
    default:
      return (
        <div className="space-y-1">
          <label htmlFor={id} className="block text-sm font-medium text-slate-700">
            {label ?? attrs.name}
          </label>
          <input
            id={id}
            type={attrs.type}
            name={attrs.name}
            defaultValue={
              attrs.type === "password" || attrs.value == null ? undefined : String(attrs.value)
            }
            required={attrs.required}
            disabled={attrs.disabled}
            pattern={attrs.pattern}
            maxLength={attrs.maxlength}
            autoComplete={attrs.autocomplete}
            inputMode={attrs.autocomplete === "one-time-code" ? "numeric" : undefined}
            aria-describedby={msgId}
            aria-invalid={hasError || undefined}
            className="block w-full rounded-md border-0 px-3 py-2 text-sm text-slate-900 ring-1 ring-slate-300 ring-inset focus:ring-2 focus:ring-blue-600 aria-[invalid=true]:ring-red-600"
          />
          <NodeMessages id={msgId} messages={node.messages} />
        </div>
      );
  }
}
