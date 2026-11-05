import { describe, expect, it } from "vitest";
import nextConfig from "../../next.config";
import { BASE_PATH } from "@/config";

describe("basePath", () => {
  it("BASE_PATH matches the basePath in next.config.ts", () => {
    expect(nextConfig.basePath).toBe(BASE_PATH);
  });
});
