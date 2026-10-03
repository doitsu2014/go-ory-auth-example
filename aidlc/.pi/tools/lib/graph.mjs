// core/tools/lib/graph.mjs — loads the phase/stage/scope methodology from the
// engine's core directory and derives the workflow order for a scope.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { parseFrontmatter } from "./frontmatter.mjs";

function readFrontmatterFile(path) {
  const text = readFileSync(path, "utf8");
  const { data, body, hasFrontmatter } = parseFrontmatter(text);
  return { data, body, hasFrontmatter };
}

function listDirs(path) {
  if (!existsSync(path)) return [];
  return readdirSync(path, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function listMarkdown(path) {
  if (!existsSync(path)) return [];
  return readdirSync(path, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
    .map((entry) => entry.name)
    .sort();
}

export function titleCase(slug) {
  return slug
    .split("-")
    .map((part) => (part.length ? part[0].toUpperCase() + part.slice(1) : part))
    .join(" ");
}

function asArray(value) {
  if (value === null || value === undefined) return [];
  return Array.isArray(value) ? value : [value];
}

/** Load all phases, stages, and scopes from core/. */
export function loadMethodology(coreRoot) {
  const phasesRoot = join(coreRoot, "phases");
  const scopesRoot = join(coreRoot, "scopes");
  const agentsRoot = join(coreRoot, "agents");

  const phases = [];
  const stages = [];

  for (const phaseSlug of listDirs(phasesRoot)) {
    const phaseFile = join(phasesRoot, phaseSlug, "phase.md");
    if (!existsSync(phaseFile)) continue;
    const { data } = readFrontmatterFile(phaseFile);
    phases.push({
      slug: data.slug || phaseSlug,
      name: data.name || titleCase(phaseSlug),
      order: typeof data.order === "number" ? data.order : phases.length + 1,
      focus: data.focus || "",
      aiRole: data.ai_role || "",
      output: data.output || "",
      description: data.description || "",
      keyActivities: asArray(data.key_activities),
      examplePrompts: asArray(data.example_prompts),
      review:
        data.review === undefined || data.review === null
          ? "auto"
          : String(data.review).trim().toLowerCase(),
      file: phaseFile,
    });

    const stagesRoot = join(phasesRoot, phaseSlug, "stages");
    for (const file of listMarkdown(stagesRoot)) {
      const path = join(stagesRoot, file);
      const { data, body } = readFrontmatterFile(path);
      const slug = data.slug || file.replace(/\.md$/, "");
      if (data.phase && data.phase !== phaseSlug) {
        throw new Error(
          `Stage ${path} declares phase "${data.phase}" but lives under "${phaseSlug}".`,
        );
      }
      stages.push({
        slug,
        name: data.name || titleCase(slug),
        phase: data.phase || phaseSlug,
        execution: (data.execution || "ALWAYS").toUpperCase(),
        condition: data.condition || "",
        leadAgent: data.lead_agent || null,
        supportAgents: asArray(data.support_agents),
        mode: data.mode || "inline",
        reviewer: data.reviewer || null,
        reviewClass: data.review_class || (data.reviewer ? "adversarial" : null),
        review:
          data.review === undefined || data.review === null
            ? "auto"
            : String(data.review).trim().toLowerCase(),
        forEach: data.for_each || null,
        workspaceRequires: data.workspace_requires === true,
        questionBudget: data.question_budget ?? null,
        produces: asArray(data.produces),
        consumes: asArray(data.consumes).map((entry) => ({
          artifact: entry.artifact,
          required: entry.required !== false,
        })),
        requiresStage: asArray(data.requires_stage),
        scopes: asArray(data.scopes),
        inputs: data.inputs || "",
        outputs: data.outputs || "",
        file: path,
        body,
      });
    }
  }

  phases.sort((a, b) => a.order - b.order);

  const scopes = [];
  for (const file of listMarkdown(scopesRoot)) {
    const path = join(scopesRoot, file);
    const { data, body } = readFrontmatterFile(path);
    scopes.push({
      name: data.name || file.replace(/\.md$/, ""),
      depth: data.depth || "Standard",
      keywords: asArray(data.keywords).map((k) => String(k).toLowerCase()),
      description: data.description || "",
      skeleton: data.skeleton || "off",
      reviewCap: data.review_cap || "advisory",
      guardPolicy: data.guard_policy || "relaxed",
      sensors: data.sensors || "on",
      learnings: data.learnings || "on",
      summaryConfirmation: data.summary_confirmation || "off",
      mode: typeof data.mode === "string" ? data.mode.trim().toLowerCase() : null,
      reviewRequired: data.review_required ?? null,
      questionBudget: data.question_budget ?? null,
      phases: asArray(data.phases),
      include: asArray(data.include),
      skip: asArray(data.skip),
      file: path,
      body,
    });
  }
  scopes.sort((a, b) => a.name.localeCompare(b.name));

  const agents = [];
  for (const file of listMarkdown(agentsRoot)) {
    const path = join(agentsRoot, file);
    const { data, body } = readFrontmatterFile(path);
    agents.push({
      slug: file.replace(/\.md$/, ""),
      name: data.name || file.replace(/\.md$/, ""),
      displayName: data.display_name || titleCase(file.replace(/\.md$/, "")),
      description: (data.description || "").toString().trim(),
      tier: data.tier || "balanced",
      file: path,
      body,
    });
  }
  agents.sort((a, b) => a.slug.localeCompare(b.slug));

  return { phases, stages, scopes, agents };
}

export function scopeByName(methodology, name) {
  return methodology.scopes.find((scope) => scope.name === name) || null;
}

/** Does a stage execute under a scope? */
export function stageRunsInScope(stage, scope) {
  if (stage.scopes.length > 0) return stage.scopes.includes(scope.name);
  if (scope.include.includes(stage.slug)) return true;
  if (scope.skip.includes(stage.slug)) return false;
  return scope.phases.includes(stage.phase);
}

/** The ordered, applicable stage list for a scope (topological by requires_stage). */
export function workflowForScope(methodology, scope) {
  const phaseOrder = new Map(methodology.phases.map((phase) => [phase.slug, phase.order]));
  const applicable = methodology.stages.filter((stage) => stageRunsInScope(stage, scope));
  const bySlug = new Map(applicable.map((stage) => [stage.slug, stage]));

  const dependencies = new Map();
  for (const stage of applicable) {
    const deps = stage.requiresStage.filter((slug) => bySlug.has(slug));
    dependencies.set(stage.slug, deps);
  }

  const indegree = new Map();
  for (const stage of applicable) indegree.set(stage.slug, dependencies.get(stage.slug).length);
  const dependents = new Map();
  for (const stage of applicable) {
    for (const dep of dependencies.get(stage.slug)) {
      if (!dependents.has(dep)) dependents.set(dep, []);
      dependents.get(dep).push(stage.slug);
    }
  }

  const ready = applicable
    .filter((stage) => indegree.get(stage.slug) === 0)
    .map((stage) => stage.slug);
  const sortReady = (slugs) =>
    slugs.sort((a, b) => {
      const sa = bySlug.get(a);
      const sb = bySlug.get(b);
      const pa = phaseOrder.get(sa.phase) ?? 0;
      const pb = phaseOrder.get(sb.phase) ?? 0;
      if (pa !== pb) return pa - pb;
      return a.localeCompare(b);
    });

  const ordered = [];
  while (ready.length > 0) {
    sortReady(ready);
    const slug = ready.shift();
    ordered.push(bySlug.get(slug));
    for (const dependent of dependents.get(slug) || []) {
      indegree.set(dependent, indegree.get(dependent) - 1);
      if (indegree.get(dependent) === 0) ready.push(dependent);
    }
  }

  if (ordered.length !== applicable.length) {
    const missing = applicable
      .map((stage) => stage.slug)
      .filter((slug) => !ordered.some((stage) => stage.slug === slug));
    throw new Error(`Cycle detected in workflow for scope "${scope.name}": ${missing.join(", ")}`);
  }
  return ordered;
}

/** Auto-detect a scope from a freeform description. Longest keyword wins. */
export function detectScope(methodology, description, fallback = "classic") {
  if (!description) return fallback;
  const text = ` ${description.toLowerCase()} `;
  let best = null;
  let bestLength = 0;
  for (const scope of methodology.scopes) {
    for (const keyword of scope.keywords) {
      if (!keyword) continue;
      const pattern = new RegExp(`(^|[^a-z0-9])${escapeRegExp(keyword)}([^a-z0-9]|$)`, "i");
      if (pattern.test(text) && keyword.length > bestLength) {
        best = scope.name;
        bestLength = keyword.length;
      }
    }
  }
  return best || fallback;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/** Compile the method into a serializable graph for `package` output. */
export function compileGraph(coreRoot) {
  const methodology = loadMethodology(coreRoot);
  const scopeGrid = {};
  for (const scope of methodology.scopes) {
    scopeGrid[scope.name] = workflowForScope(methodology, scope).map((stage) => stage.slug);
  }
  return {
    phases: methodology.phases.map((phase) => ({
      slug: phase.slug,
      name: phase.name,
      order: phase.order,
      focus: phase.focus,
      aiRole: phase.aiRole,
      output: phase.output,
      review: phase.review,
    })),
    stages: methodology.stages.map((stage) => ({
      slug: stage.slug,
      name: stage.name,
      phase: stage.phase,
      execution: stage.execution,
      leadAgent: stage.leadAgent,
      supportAgents: stage.supportAgents,
      mode: stage.mode,
      reviewer: stage.reviewer,
      review: stage.review,
      produces: stage.produces,
      consumes: stage.consumes,
      requiresStage: stage.requiresStage,
    })),
    scopes: methodology.scopes.map((scope) => ({
      name: scope.name,
      depth: scope.depth,
      keywords: scope.keywords,
      phases: scope.phases,
      skip: scope.skip,
    })),
    scopeGrid,
  };
}
