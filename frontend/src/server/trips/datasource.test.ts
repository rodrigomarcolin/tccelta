import { afterEach, describe, expect, it, vi } from "vitest";
import { initialDongle, chooseSource, readConfig, getDatasource } from "./datasource";

const API = {
  ANALISE_FONTE_DADOS: "api",
  ANALISE_DONGLE_ID: "4",
  ANALISE_CLIENT_ID: "next-test",
  ANALISE_CLIENT_SECRET: "test-secret",
};

afterEach(() => {
  vi.unstubAllGlobals();
  delete (globalThis as Record<string, unknown>).__tccelta_datasource__;
});

describe("chooseSource", () => {
  it("uses mock in development when the variable is missing", () => {
    expect(chooseSource({ NODE_ENV: "development" })).toBe("mock");
    expect(chooseSource({ NODE_ENV: "test", ANALISE_FONTE_DADOS: "  " })).toBe("mock");
  });
  it("in production the variable is required", () => {
    expect(() => chooseSource({ NODE_ENV: "production" })).toThrow(/ANALISE_FONTE_DADOS/);
    expect(() => chooseSource({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "" })).toThrow();
  });
  it("accepts explicit mock in production", () => {
    expect(chooseSource({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "mock" })).toBe("mock");
  });
  it("api requires the dongle", () => {
    expect(() => chooseSource({ ANALISE_FONTE_DADOS: "api" })).toThrow(/ANALISE_DONGLE_ID/);
    expect(chooseSource(API)).toBe("api");
  });
  it("rejects an unknown value", () => {
    expect(() => chooseSource({ ANALISE_FONTE_DADOS: "banco" })).toThrow(/unknown/);
  });
});

describe("getDatasource", () => {
  it("always returns the same instance", () => {
    const a = getDatasource({ NODE_ENV: "test" });
    expect(getDatasource({ NODE_ENV: "test" })).toBe(a);
  });
  it("does not start in production without the variable", () => {
    expect(() => getDatasource({ NODE_ENV: "production" })).toThrow();
  });
});

describe("readConfig with the api source", () => {
  it("uses the local URL and the default timeout in development", () => {
    expect(readConfig({ ...API, NODE_ENV: "development" })).toEqual({
      source: "api",
      url: "http://127.0.0.1:8080",
      timeoutMs: 10_000,
      dongleId: 4,
      clientId: "next-test",
      clientSecret: "test-secret",
    });
  });
  it("in production the URL is required", () => {
    expect(() => readConfig({ ...API, NODE_ENV: "production" })).toThrow(/ANALISE_API_URL/);
  });
  it("strips the trailing slash and reads the timeout", () => {
    expect(
      readConfig({
        ...API,
        ANALISE_API_URL: "http://api:8080//",
        ANALISE_TIMEOUT_MS: "2500",
      }),
    ).toMatchObject({ url: "http://api:8080", timeoutMs: 2500 });
  });
  it("rejects invalid URL, protocol, dongle and timeout", () => {
    expect(() => readConfig({ ...API, ANALISE_API_URL: "api" })).toThrow(/valid URL/);
    expect(() => readConfig({ ...API, ANALISE_API_URL: "ftp://x" })).toThrow(/http/);
    expect(() => readConfig({ ...API, ANALISE_DONGLE_ID: "0" })).toThrow(/ANALISE_DONGLE_ID/);
    expect(() => readConfig({ ...API, ANALISE_DONGLE_ID: "1.5" })).toThrow(/integer/);
    expect(() => readConfig({ ...API, ANALISE_TIMEOUT_MS: "-3" })).toThrow(/ANALISE_TIMEOUT_MS/);
  });
});

describe("initialDongle", () => {
  it("comes from ANALISE_DONGLE_ID with the api and is 1 with the mock", () => {
    expect(initialDongle(API)).toBe(4);
    expect(initialDongle({ NODE_ENV: "test" })).toBe(1);
  });
});

describe("getDatasource with the api", () => {
  it("switches datasource when ANALISE_API_URL changes, without rebuilding", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? new Response(JSON.stringify({ access_token: "t", expires_in: 1800 }), { status: 200 })
        : new Response("{}", { status: 200 }),
    );
    vi.stubGlobal("fetch", f);
    const id = "7d1f3a10-0001-4c5e-9b1a-000000000008";
    const a = getDatasource({ ...API, ANALISE_API_URL: "http://a:1" });
    expect(getDatasource({ ...API, ANALISE_API_URL: "http://a:1" })).toBe(a);
    await a.status(id);
    const b = getDatasource({ ...API, ANALISE_API_URL: "http://b:2" });
    expect(b).not.toBe(a);
    await b.status(id);
    expect(f.mock.calls.map((c) => (c as unknown as [string])[0])).toEqual([
      "http://a:1/v1/auth/token",
      `http://a:1/v1/viagens/${id}`,
      "http://b:2/v1/auth/token",
      `http://b:2/v1/viagens/${id}`,
    ]);
  });
  it("with mock does not call the network", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    await getDatasource({ NODE_ENV: "test" }).status("7d1f3a10-0001-4c5e-9b1a-000000000008");
    expect(f).not.toHaveBeenCalled();
  });
});

describe("API client credentials", () => {
  it("ANALISE_CLIENT_ID and ANALISE_CLIENT_SECRET are required, without calling the network", () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    const withoutId = { ...API, ANALISE_CLIENT_ID: undefined };
    const withoutSecret = { ...API, ANALISE_CLIENT_SECRET: undefined };
    expect(() => readConfig(withoutId)).toThrow(/ANALISE_CLIENT_ID/);
    expect(() => readConfig(withoutSecret)).toThrow(/ANALISE_CLIENT_SECRET/);
    expect(() => readConfig({ ...API, ANALISE_CLIENT_SECRET: "  " })).toThrow(
      /ANALISE_CLIENT_SECRET/,
    );
    expect(() => getDatasource(withoutSecret)).toThrow();
    expect(f).not.toHaveBeenCalled();
  });
  it("the client gets the token and calls the API with Bearer", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? new Response(JSON.stringify({ access_token: "jwt-x", expires_in: 1800 }), {
            status: 200,
          })
        : new Response("{}", { status: 200 }),
    );
    vi.stubGlobal("fetch", f);
    await getDatasource({ ...API, ANALISE_API_URL: "http://c:3" }).status(
      "7d1f3a10-0001-4c5e-9b1a-000000000008",
    );
    const calls = f.mock.calls as unknown as [string, RequestInit][];
    expect(calls[0][0]).toBe("http://c:3/v1/auth/token");
    expect((calls[1][1].headers as Record<string, string>).Authorization).toBe("Bearer jwt-x");
  });
});
