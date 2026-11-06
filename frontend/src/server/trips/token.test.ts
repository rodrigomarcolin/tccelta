import { beforeEach, describe, expect, it, vi } from "vitest";
import { createTokenManager, slackFor, type TokenManager } from "./token";

const SECRET = "segredo-sentinela-xyz";
const ID = "next-test";
const BASIC = `Basic ${Buffer.from(`${ID}:${SECRET}`).toString("base64")}`;

let now: number;
let fetchFn: ReturnType<typeof vi.fn<typeof fetch>>;
let log: ReturnType<typeof vi.fn<(n: "error" | "warn", m: string) => void>>;
let g: TokenManager;

const ok = (token: string, expiresIn = 1800) =>
  new Response(
    JSON.stringify({ access_token: token, token_type: "Bearer", expires_in: expiresIn }),
    {
      status: 200,
      headers: { "Content-Type": "application/json" },
    },
  );
const errorResponse = (status: number, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify({ error: "x", error_description: SECRET, codigo: "x" }), {
    status,
    headers: headers,
  });

function create() {
  g = createTokenManager({
    baseUrl: "http://api:8080",
    clientId: ID,
    clientSecret: SECRET,
    fetchFn,
    now: () => now,
    log,
  });
}

beforeEach(() => {
  now = 1_000_000;
  fetchFn = vi.fn<typeof fetch>(async () => ok("t1"));
  log = vi.fn();
  create();
});

describe("token request", () => {
  it("sends Basic, form urlencoded and grant_type without scope", async () => {
    const r = await g.getToken();
    expect(r).toEqual({ ok: true, token: "t1" });
    const [url, init] = fetchFn.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://api:8080/v1/auth/token");
    expect(init.method).toBe("POST");
    const h = init.headers as Record<string, string>;
    expect(h.Authorization).toBe(BASIC);
    expect(h["Content-Type"]).toBe("application/x-www-form-urlencoded");
    expect(init.body).toBe("grant_type=client_credentials");
  });

  it("does not read JWKS or kid: only the token endpoint is called", async () => {
    await g.getToken();
    g.invalidate();
    await g.getToken();
    expect(fetchFn.mock.calls.every((c) => String(c[0]).endsWith("/v1/auth/token"))).toBe(true);
  });
});

describe("cache and slack", () => {
  it("reuses the token within the slack", async () => {
    await g.getToken();
    now += 1_700_000; // validity 1800 s, slack 60 s: still valid
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it("renews at the 60 s slack (long validity)", async () => {
    await g.getToken();
    fetchFn.mockResolvedValueOnce(ok("t2"));
    now += 1_740_000; // 60 s remaining
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("short validity uses 20%", async () => {
    expect(slackFor(60_000)).toBe(12_000);
    expect(slackFor(300_000)).toBe(60_000);
    fetchFn.mockResolvedValueOnce(ok("curto", 100));
    await g.getToken();
    now += 79_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(1);
    fetchFn.mockResolvedValueOnce(ok("novo", 100));
    now += 1_000; // 80 s = 80% of the validity
    expect(await g.getToken()).toEqual({ ok: true, token: "novo" });
  });

  it("50 simultaneous calls produce a single request", async () => {
    const todas = await Promise.all(Array.from({ length: 50 }, () => g.getToken()));
    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(todas.every((r) => r.ok && r.token === "t1")).toBe(true);
  });

  it("invalidate discards the token; with the old rejected token, it does not discard the new one", async () => {
    await g.getToken();
    g.invalidate("t1");
    fetchFn.mockResolvedValueOnce(ok("t2"));
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    g.invalidate("t1"); // late 401 from another request
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });
});

describe("failures", () => {
  it("401 invalid_client: does not retry for 60 s and then tries once", async () => {
    fetchFn.mockImplementation(async () => errorResponse(401, { "WWW-Authenticate": "Basic" }));
    const r = await g.getToken();
    expect(r).toMatchObject({
      ok: false,
      failure: { status: 502, body: { codigo: "credencial_invalida" }, retryAfter: 60 },
    });
    for (let i = 0; i < 20; i++) {
      now += 2_900;
      const x = await g.getToken();
      expect(x.ok).toBe(false);
    }
    expect(fetchFn).toHaveBeenCalledTimes(1); // 58 s
    now += 5_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(2);
    expect(log).toHaveBeenCalledWith("error", expect.stringContaining(ID));
  });

  it("429 with Retry-After: no attempt until the deadline", async () => {
    fetchFn.mockImplementation(async () => errorResponse(429, { "Retry-After": "120" }));
    expect(await g.getToken()).toMatchObject({
      ok: false,
      failure: { status: 429, body: { codigo: "bloqueado" }, retryAfter: 120 },
    });
    now += 30_000;
    expect(await g.getToken()).toMatchObject({ failure: { retryAfter: 90 } });
    now += 89_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(1);
    now += 1_000;
    fetchFn.mockResolvedValueOnce(ok("depois"));
    expect(await g.getToken()).toEqual({ ok: true, token: "depois" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("429 without a valid Retry-After assumes 60 s", async () => {
    fetchFn.mockImplementation(async () => errorResponse(429, { "Retry-After": "soon" }));
    expect(await g.getToken()).toMatchObject({ failure: { retryAfter: 60 } });
    expect(log).toHaveBeenCalledWith("warn", expect.stringContaining("assumed"));
  });

  it("network error releases the Promise; new attempt only after 1 s", async () => {
    fetchFn.mockRejectedValueOnce(new TypeError("fetch failed"));
    const a = await g.getToken();
    expect(a).toMatchObject({
      ok: false,
      failure: { status: 503, body: { codigo: "api_indisponivel" } },
    });
    await g.getToken(); // within 1 s: does not even try
    expect(fetchFn).toHaveBeenCalledTimes(1);
    now += 1_000;
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("503 and 5xx from the token endpoint become api_indisponivel without a long quarantine", async () => {
    fetchFn.mockResolvedValueOnce(errorResponse(503, { "Retry-After": "5" }));
    expect(await g.getToken()).toMatchObject({ failure: { status: 503 } });
    now += 1_000;
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
  });

  it("a 200 response outside the contract is rejected", async () => {
    fetchFn.mockResolvedValueOnce(
      new Response(JSON.stringify({ access_token: "" }), { status: 200 }),
    );
    expect(await g.getToken()).toMatchObject({ ok: false, failure: { status: 502 } });
    fetchFn.mockResolvedValueOnce(new Response("not json", { status: 200 }));
    now += 1_000;
    expect(await g.getToken()).toMatchObject({ ok: false });
  });

  it("no log or failure carries the secret, the Basic header or the token", async () => {
    fetchFn.mockImplementationOnce(async () => errorResponse(401));
    const a = await g.getToken();
    fetchFn.mockImplementationOnce(async () => errorResponse(429));
    create();
    const b = await g.getToken();
    const everything = JSON.stringify([a, b, log.mock.calls]);
    expect(everything).not.toContain(SECRET);
    expect(everything).not.toContain(BASIC.slice(6));
  });
});
