import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * The custom classes of the old plain-CSS stylesheet. They no longer exist in any stylesheet, so
 * a leftover in a `className` would silently style nothing. `grid`, `table`, `group`, `sr-only`
 * and `text-secondary` were also old names, but they are valid Tailwind utilities now.
 */
const OLD_CLASSES = [
  "ab-marker",
  "ab-marker-wrap",
  "actions",
  "axis-text",
  "axis-title",
  "badge",
  "badge-alert",
  "badge-live",
  "badge-warning",
  "bar-fill",
  "bar-row",
  "bars",
  "bar-track",
  "bar-value",
  "button",
  "button-block",
  "button-icon",
  "button-link",
  "button-primary",
  "button-secondary",
  "button-small",
  "button-tonal",
  "card",
  "chart",
  "chart-empty",
  "chart-loading",
  "chart-panel",
  "chart-title",
  "data-value",
  "definition-row",
  "definitions",
  "definitions-compact",
  "dot",
  "dot-clickable",
  "dot-pulse",
  "event-marker",
  "events-table",
  "field",
  "field-error",
  "field-row",
  "form",
  "form-error",
  "grid-2",
  "group-items",
  "header",
  "header-driver",
  "header-row",
  "header-title",
  "heat",
  "heat-cell",
  "heat-scale",
  "heat-scale-bar",
  "indicator",
  "indicator-label",
  "indicator-note",
  "indicator-value",
  "indicators-row",
  "instant-tooltip",
  "leaflet-map",
  "legend-item",
  "legend-list",
  "legend-markers",
  "legend-name",
  "legend-value",
  "login-card",
  "map",
  "map-frame",
  "map-loading",
  "map-no-base",
  "map-notice",
  "map-tooltip",
  "median-line",
  "no-print",
  "note",
  "notice",
  "notice-bar",
  "num",
  "option",
  "options",
  "overline",
  "page-narrow",
  "page-wide",
  "plain-list",
  "provisional-banner",
  "quality",
  "quality-body",
  "scatter",
  "section",
  "section-content",
  "skeleton",
  "skeleton-block",
  "state",
  "state-title",
  "summary",
  "summary-groups",
  "summary-indicators",
  "summary-map",
  "summary-total",
  "swatch",
  "table-scroll",
  "theme-button-text",
  "tile-dark",
  "tile-light",
  "timeline",
  "tooltip-empty",
  "unit",
];

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sourceFiles(path);
    return /\.tsx?$/.test(name) && !/\.test\./.test(name) ? [path] : [];
  });
}

/** The text of the value after each `className=`, `className:` or `class=` (up to the matching `}`). */
function classValues(source: string): string[] {
  const out: string[] = [];
  for (const m of source.matchAll(/\b(?:className|class)\s*[=:]\s*/g)) {
    let i = (m.index ?? 0) + m[0].length;
    const open = source[i];
    if (open === "{") {
      let depth = 0;
      const start = i;
      for (; i < source.length; i++) {
        if (source[i] === "{") depth++;
        if (source[i] === "}" && --depth === 0) break;
      }
      out.push(source.slice(start, i + 1));
    } else if (open === '"' || open === "'" || open === "`") {
      const end = source.indexOf(open, i + 1);
      out.push(source.slice(i, end + 1));
    }
  }
  return out;
}

function tokens(value: string): string[] {
  const out: string[] = [];
  for (const m of value.matchAll(/"([^"]*)"|'([^']*)'|`([^`]*)`/g)) {
    const text = (m[1] ?? m[2] ?? m[3] ?? "").replace(/\$\{[^}]*\}/g, " ");
    out.push(...text.split(/\s+/).filter(Boolean));
  }
  return out;
}

describe("classes of the old stylesheet", () => {
  const files = sourceFiles(join(__dirname, ".."));
  const used = files.flatMap((file) =>
    classValues(readFileSync(file, "utf8")).flatMap((v) => tokens(v).map((t) => ({ file, t }))),
  );

  it("the scan finds the classes in use", () => {
    const all = new Set(used.map((u) => u.t));
    // If the scanner stopped finding the classes, the next test would pass for nothing.
    expect(used.length).toBeGreaterThan(100);
    expect(all.has("sr-only")).toBe(true);
    expect(all.has("bg-card")).toBe(true);
  });

  it("no className uses an old custom class", () => {
    const old = new Set(OLD_CLASSES);
    const found = used.filter((u) => old.has(u.t)).map((u) => `${u.file}: ${u.t}`);
    expect(found).toEqual([]);
  });
});
