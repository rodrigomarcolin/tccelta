// @vitest-environment node
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Browser bundle (INT2). After `next build` with a sentinel secret
 * (`ANALISE_CLIENT_SECRET=$SENTINEL npm run build`), nothing in `.next/static`
 * may carry the variable names, the secret value or a JWT. Without a build,
 * the test is skipped.
 */
const STATIC_DIR = join(process.cwd(), ".next", "static");
const SENTINEL = process.env.ANALISE_CLIENT_SECRET ?? "sentinel-secret-int2-do-not-leak";
const SENTINEL_ID = process.env.ANALISE_CLIENT_ID ?? "sentinel-id-int2";

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? files(path) : [path];
  });
}

describe.skipIf(!existsSync(STATIC_DIR))("bundle delivered to the browser", () => {
  const text = existsSync(STATIC_DIR)
    ? files(STATIC_DIR)
        .filter((a) => /\.(js|css|html|json|txt|map)$/.test(a))
        .map((a) => readFileSync(a, "utf8"))
        .join("\n")
    : "";

  it("has content to scan", () => {
    expect(text.length).toBeGreaterThan(1000);
  });

  it.each(["ANALISE_CLIENT_SECRET", "ANALISE_CLIENT_ID", "NEXT_PUBLIC_ANALISE", "grant_type"])(
    "does not contain %s",
    (name) => {
      expect(text).not.toContain(name);
    },
  );

  it("does not contain the secret value or the client_id", () => {
    expect(text).not.toContain(SENTINEL);
    expect(text).not.toContain(SENTINEL_ID);
  });

  it("does not contain a JWT", () => {
    expect(text).not.toMatch(/eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}/);
  });
});
