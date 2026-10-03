import type { UiNode } from "@ory/client-fetch";

/** Splits nodes into one form per method group; every form carries the default-group nodes (csrf_token, identifier). */
export function groupNodes(nodes: UiNode[]): { group: string; nodes: UiNode[] }[] {
  // Script nodes (WebAuthn/passkey JS) are never rendered (CSP), so they never form a group.
  nodes = nodes.filter((n) => n.type !== "script");
  const groups: string[] = [];
  for (const n of nodes) {
    if (n.group !== "default" && !groups.includes(n.group)) groups.push(n.group);
  }
  if (groups.length === 0) return [{ group: "default", nodes }];
  return groups.map((g) => ({
    group: g,
    nodes: nodes.filter((n) => n.group === "default" || n.group === g),
  }));
}
