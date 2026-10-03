import {
  type UiNode,
  type UiNodeInputAttributes,
  isUiNodeInputAttributes,
} from "@ory/client-fetch";

export type FlowBody = Record<string, unknown>;

export function inputAttrs(node: UiNode): UiNodeInputAttributes | undefined {
  return isUiNodeInputAttributes(node.attributes) ? node.attributes : undefined;
}

export function isSubmitNode(node: UiNode): boolean {
  const a = inputAttrs(node);
  return !!a && (a.type === "submit" || a.type === "button");
}

/** "traits.name.first" -> { traits: { name: { first } } } */
export function setPath(target: FlowBody, path: string, value: unknown): void {
  const keys = path.split(".");
  let cursor: Record<string, unknown> = target;
  keys.forEach((key, i) => {
    if (i === keys.length - 1) {
      cursor[key] = value;
      return;
    }
    const existing = cursor[key];
    if (typeof existing !== "object" || existing === null) {
      cursor[key] = {};
    }
    cursor = cursor[key] as Record<string, unknown>;
  });
}

/**
 * Builds the JSON body for a Kratos update call from a rendered form.
 * - hidden inputs (csrf_token, …) keep their original, typed node value
 * - checkboxes become booleans
 * - the clicked submit button contributes its name with its original typed
 *   value (e.g. method="password", lookup_secret_reveal=true)
 * - dotted names are nested (traits.email -> {traits:{email}})
 */
export function collectBody(
  form: HTMLFormElement,
  nodes: UiNode[],
  submitter: { name: string; value: string } | null,
): FlowBody {
  const body: FlowBody = {};
  const data = new FormData(form);

  for (const node of nodes) {
    const a = inputAttrs(node);
    if (!a || a.disabled || isSubmitNode(node)) continue;
    if (a.type === "hidden") {
      setPath(body, a.name, a.value);
    } else if (a.type === "checkbox") {
      const el = form.elements.namedItem(a.name);
      setPath(body, a.name, el instanceof HTMLInputElement ? el.checked : false);
    } else {
      const v = data.get(a.name);
      if (typeof v === "string") {
        setPath(body, a.name, a.type === "number" && v !== "" ? Number(v) : v);
      }
    }
  }

  const submitNodes = nodes.filter(isSubmitNode);
  const clicked =
    (submitter &&
      submitNodes.find((n) => {
        const a = inputAttrs(n);
        return a?.name === submitter.name && String(a.value) === submitter.value;
      })) ??
    submitNodes[0];
  const ca = clicked ? inputAttrs(clicked) : undefined;
  if (ca) setPath(body, ca.name, ca.value);

  return body;
}
