// @vitest-environment node
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = resolve(__dirname, "../..");

/** Local modules (`@/...` and relative) a file imports, as absolute file paths. */
function localImports(file: string): string[] {
  const text = readFileSync(file, "utf8");
  const out: string[] = [];
  for (const m of text.matchAll(/from\s+"([^"]+)"/g)) {
    const spec = m[1];
    const base = spec.startsWith("@/")
      ? join(SRC, spec.slice(2))
      : spec.startsWith(".")
        ? resolve(dirname(file), spec)
        : null;
    if (!base) continue;
    for (const candidate of [`${base}.ts`, `${base}.tsx`, join(base, "index.ts")]) {
      try {
        readFileSync(candidate);
        out.push(candidate);
        break;
      } catch {
        // Not this extension.
      }
    }
  }
  return out;
}

describe("the minimap stays free of Leaflet", () => {
  it("nothing reachable from RoutePreview and TripCard imports leaflet", () => {
    const seen = new Set<string>();
    const queue = [
      join(SRC, "components/trips/RoutePreview.tsx"),
      join(SRC, "components/trips/TripCard.tsx"),
    ];
    while (queue.length) {
      const file = queue.pop()!;
      if (seen.has(file)) continue;
      seen.add(file);
      const text = readFileSync(file, "utf8");
      expect(text, file).not.toMatch(/"leaflet"|LeafletRouteMap|\/RouteMap"/);
      queue.push(...localImports(file));
    }
    // It did walk through the modules that matter.
    const names = [...seen].map((f) => f.replace(SRC, ""));
    expect(names.some((n) => n.endsWith("tileClasses.ts"))).toBe(true);
    expect(names.some((n) => n.endsWith("preview.ts"))).toBe(true);
  });
});
