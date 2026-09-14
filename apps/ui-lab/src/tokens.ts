import { UI_EASE_OUT_CSS, UI_EASE_IN } from "@multica/ui/lib/motion";
import source from "@multica/ui/styles/tokens.css?raw";

export type Theme = "light" | "dark";
export type Scope = Theme | "shared";
export type TokenValues = Record<string, string>;
export type Draft = Record<Scope, TokenValues>;
export const emptyDraft = (): Draft => ({ light: {}, dark: {}, shared: {} });

export const colorTokens = [
  ["--brand", "brand"],
  ["--primary", "primary"],
  ["--primary-foreground", "primaryText"],
  ["--secondary", "secondary"],
  ["--secondary-foreground", "secondaryText"],
  ["--destructive", "destructive"],
  ["--ring", "ring"],
  ["--app-shell", "shell"],
  ["--page-canvas", "page"],
  ["--surface", "surface"],
  ["--surface-raised", "raised"],
  ["--surface-hover", "hover"],
  ["--surface-selected", "selected"],
  ["--foreground", "foreground"],
  ["--muted-foreground", "muted"],
] as const;
export const buttonScales = ["xs", "sm", "default", "lg"] as const;
export type ButtonScale = (typeof buttonScales)[number];
export const buttonTokens = buttonScales.flatMap(
  (scale) =>
    [
      {
        key: `--button-height-${scale}`,
        label: "height",
        min: 20,
        max: 48,
        step: 1,
        unit: "px",
        group: "button",
        scale,
      },
      {
        key: `--button-padding-${scale}`,
        label: "padding",
        min: 4,
        max: 24,
        step: 1,
        unit: "px",
        group: "button",
        scale,
      },
      {
        key: `--button-gap-${scale}`,
        label: "gap",
        min: 0,
        max: 16,
        step: 1,
        unit: "px",
        group: "button",
        scale,
      },
    ] as const,
);
export const motionTokens = [
  {
    key: "--dialog-enter-duration",
    label: "enterDuration",
    min: 0,
    max: 1000,
    step: 10,
    unit: "ms",
    group: "motion",
  },
  {
    key: "--dialog-exit-duration",
    label: "exitDuration",
    min: 0,
    max: 1000,
    step: 10,
    unit: "ms",
    group: "motion",
  },
] as const;
export const easingTokens = [
  { key: "--dialog-enter-easing", label: "enterEasing" },
  { key: "--dialog-exit-easing", label: "exitEasing" },
] as const;
export const motionEasings = [
  { label: "standard", value: "ease" },
  { label: "linear", value: "linear" },
  { label: "easeOut", value: UI_EASE_OUT_CSS },
  { label: "easeIn", value: `cubic-bezier(${UI_EASE_IN.join(", ")})` },
] as const;
export const sizeTokens = [
  ...motionTokens,
  ...buttonTokens,
  {
    key: "--issue-row-height",
    label: "rowHeight",
    min: 32,
    max: 48,
    step: 2,
    unit: "px",
    group: "density",
  },
  {
    key: "--radius",
    label: "radius",
    min: 0,
    max: 16,
    step: 1,
    unit: "px",
    group: "radius",
  },
  {
    key: "--text-caption",
    label: "caption",
    min: 10,
    max: 16,
    step: 1,
    unit: "px",
    group: "type",
  },
  {
    key: "--text-label",
    label: "label",
    min: 11,
    max: 17,
    step: 1,
    unit: "px",
    group: "type",
  },
  {
    key: "--text-body",
    label: "body",
    min: 12,
    max: 18,
    step: 1,
    unit: "px",
    group: "type",
  },
  {
    key: "--text-body--line-height",
    label: "lineHeight",
    min: 18,
    max: 30,
    step: 1,
    unit: "px",
    group: "type",
  },
  {
    key: "--text-title-lg",
    label: "title",
    min: 18,
    max: 28,
    step: 1,
    unit: "px",
    group: "type",
  },
] as const;

// Read the authoritative file at build time. Defaults never become a second theme.
export function readSourceTokens(css: string): Draft {
  const clean = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const block = (selector: RegExp): TokenValues => {
    const body = clean.match(selector)?.[1];
    if (!body) throw new Error("Cannot locate the shared design tokens.");
    return Object.fromEntries(
      [...body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [
        m[1]!,
        m[2]!.trim(),
      ]),
    );
  };
  const light = block(/:root\s*\{([^}]+)\}/);
  const dark = block(/\.dark\s*\{([^}]+)\}/);
  const typography = block(/@theme\s*\{([^}]+)\}/);
  return {
    light,
    dark: { ...light, ...dark },
    shared: {
      ...typography,
      ...Object.fromEntries(
        Object.entries(light).filter(
          ([key]) => key.startsWith("--button-") || key.startsWith("--dialog-"),
        ),
      ),
      "--radius": light["--radius"]!,
      "--issue-row-height": light["--issue-row-height"]!,
    },
  };
}
export const baseline = readSourceTokens(source);
export const sourceRevision = [...source]
  .reduce(
    (hash, char) => Math.imul(hash ^ char.charCodeAt(0), 16777619),
    2166136261,
  )
  .toString(16);

export function tokenValue(draft: Draft, scope: Scope, key: string): string {
  const value = draft[scope][key] ?? baseline[scope][key];
  if (value === undefined) throw new Error(`Unknown token: ${key}`);
  return value;
}
export function sizeValue(value: string): number {
  return parseFloat(value) * (value.endsWith("rem") ? 16 : 1);
}
export function numericTokenValue(key: string, value: number): string {
  const token = sizeTokens.find((item) => item.key === key);
  if (!token) throw new Error(`Unknown numeric token: ${key}`);
  return `${value}${token.unit}`;
}
export function parseColor(value: string): [number, number, number] {
  const match = /^oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)$/.exec(value);
  if (!match) throw new Error(`Unsupported color: ${value}`);
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}
export function updateToken(
  draft: Draft,
  scope: Scope,
  key: string,
  value: string,
): Draft {
  const values = { ...draft[scope] };
  const original = baseline[scope][key];
  const equal =
    value === original ||
    (scope === "shared" && sizeValue(value) === sizeValue(original ?? ""));
  if (equal) delete values[key];
  else values[key] = value;
  return { ...draft, [scope]: values };
}
export function changeCount(draft: Draft): number {
  return Object.values(draft).reduce(
    (sum, values) => sum + Object.keys(values).length,
    0,
  );
}
function block(selector: string, values: TokenValues): string {
  const entries = Object.entries(values).sort(([a], [b]) => a.localeCompare(b));
  return entries.length
    ? `${selector} {\n${entries.map(([key, value]) => `  ${key}: ${value};`).join("\n")}\n}`
    : "";
}
export function previewCss(draft: Draft): string {
  return [
    block(":root", draft.shared),
    block(":root:not(.dark)", draft.light),
    block(".dark", draft.dark),
  ]
    .filter(Boolean)
    .join("\n\n");
}
export function exportCss(draft: Draft): string {
  if (!changeCount(draft))
    return "/* No changes to the current Multica tokens. */\n";
  const typography = Object.fromEntries(
    Object.entries(draft.shared).filter(([key]) => key.startsWith("--text-")),
  );
  const geometry = Object.fromEntries(
    Object.entries(draft.shared).filter(([key]) => !key.startsWith("--text-")),
  );
  return (
    [
      "/* Multica UI Lab — merge these declarations into the matching blocks in\n   packages/ui/styles/tokens.css. Review both themes before applying.\n   Existing aliases and radius multipliers remain unchanged. */",
      block("@theme", typography),
      block(":root", {
        ...geometry,
        ...draft.light,
      }),
      block(".dark", draft.dark),
    ]
      .filter(Boolean)
      .join("\n\n") + "\n"
  );
}

export function isDraft(value: unknown): value is Draft {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Record<string, unknown>;
  if (Object.keys(candidate).sort().join() !== "dark,light,shared")
    return false;
  return (["light", "dark", "shared"] as const).every((scope) => {
    const values = candidate[scope];
    if (!values || typeof values !== "object" || Array.isArray(values))
      return false;
    return Object.entries(values).every(([key, v]) => {
      if (typeof v !== "string") return false;
      if (scope === "shared") {
        const definition = sizeTokens.find((token) => token.key === key);
        if (easingTokens.some((token) => token.key === key))
          return motionEasings.some((easing) => easing.value === v);
        if (
          !definition ||
          !/^\d+(\.\d+)?(px|ms)$/.test(v) ||
          !v.endsWith(definition.unit)
        )
          return false;
        const numeric = sizeValue(v);
        return numeric >= definition.min && numeric <= definition.max;
      }
      if (!colorTokens.some(([token]) => token === key)) return false;
      try {
        const [l, c, h] = parseColor(v);
        return l >= 0 && l <= 1 && c >= 0 && c <= 0.4 && h >= 0 && h <= 360;
      } catch {
        return false;
      }
    });
  });
}

export type History = { past: Draft[]; present: Draft; future: Draft[] };
export type EditAction =
  | { type: "edit"; draft: Draft }
  | { type: "undo" }
  | { type: "redo" };
export function editHistory(state: History, action: EditAction): History {
  if (action.type === "edit") {
    if (JSON.stringify(state.present) === JSON.stringify(action.draft))
      return state;
    return {
      past: [...state.past.slice(-99), state.present],
      present: action.draft,
      future: [],
    };
  }
  if (action.type === "undo") {
    const previous = state.past.at(-1);
    return previous
      ? {
          past: state.past.slice(0, -1),
          present: previous,
          future: [state.present, ...state.future],
        }
      : state;
  }
  const next = state.future[0];
  return next
    ? {
        past: [...state.past, state.present],
        present: next,
        future: state.future.slice(1),
      }
    : state;
}
