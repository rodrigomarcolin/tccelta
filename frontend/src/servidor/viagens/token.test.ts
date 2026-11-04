import { beforeEach, describe, expect, it, vi } from "vitest";
import { criarGerenciadorToken, folgaDe, type GerenciadorToken } from "./token";

const SEGREDO = "segredo-sentinela-xyz";
const ID = "next-test";
const BASIC = `Basic ${Buffer.from(`${ID}:${SEGREDO}`).toString("base64")}`;

let agora: number;
let fetchFn: ReturnType<typeof vi.fn<typeof fetch>>;
let log: ReturnType<typeof vi.fn<(n: "error" | "warn", m: string) => void>>;
let g: GerenciadorToken;

const ok = (token: string, expiresIn = 1800) =>
  new Response(
    JSON.stringify({ access_token: token, token_type: "Bearer", expires_in: expiresIn }),
    {
      status: 200,
      headers: { "Content-Type": "application/json" },
    },
  );
const erro = (status: number, cabecalhos: Record<string, string> = {}) =>
  new Response(JSON.stringify({ error: "x", error_description: SEGREDO, codigo: "x" }), {
    status,
    headers: cabecalhos,
  });

function criar() {
  g = criarGerenciadorToken({
    baseUrl: "http://api:8080",
    clientId: ID,
    clientSecret: SEGREDO,
    fetchFn,
    agora: () => agora,
    log,
  });
}

beforeEach(() => {
  agora = 1_000_000;
  fetchFn = vi.fn<typeof fetch>(async () => ok("t1"));
  log = vi.fn();
  criar();
});

describe("pedido de token", () => {
  it("envia Basic, form urlencoded e grant_type sem scope", async () => {
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

  it("não lê JWKS nem kid: só o endpoint de token é chamado", async () => {
    await g.getToken();
    g.invalidate();
    await g.getToken();
    expect(fetchFn.mock.calls.every((c) => String(c[0]).endsWith("/v1/auth/token"))).toBe(true);
  });
});

describe("cache e folga", () => {
  it("reutiliza o token dentro da folga", async () => {
    await g.getToken();
    agora += 1_700_000; // validade 1800 s, folga 60 s: ainda vale
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it("renova na folga de 60 s (validade longa)", async () => {
    await g.getToken();
    fetchFn.mockResolvedValueOnce(ok("t2"));
    agora += 1_740_000; // faltam 60 s
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("validade curta usa 20%", async () => {
    expect(folgaDe(60_000)).toBe(12_000);
    expect(folgaDe(300_000)).toBe(60_000);
    fetchFn.mockResolvedValueOnce(ok("curto", 100));
    await g.getToken();
    agora += 79_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(1);
    fetchFn.mockResolvedValueOnce(ok("novo", 100));
    agora += 1_000; // 80 s = 80% da validade
    expect(await g.getToken()).toEqual({ ok: true, token: "novo" });
  });

  it("50 chamadas simultâneas geram um só pedido", async () => {
    const todas = await Promise.all(Array.from({ length: 50 }, () => g.getToken()));
    expect(fetchFn).toHaveBeenCalledTimes(1);
    expect(todas.every((r) => r.ok && r.token === "t1")).toBe(true);
  });

  it("invalidate descarta o token; com o token recusado antigo, não descarta o novo", async () => {
    await g.getToken();
    g.invalidate("t1");
    fetchFn.mockResolvedValueOnce(ok("t2"));
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    g.invalidate("t1"); // 401 atrasado de outro pedido
    expect(await g.getToken()).toEqual({ ok: true, token: "t2" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });
});

describe("falhas", () => {
  it("401 invalid_client: não repete por 60 s e depois tenta uma vez", async () => {
    fetchFn.mockImplementation(async () => erro(401, { "WWW-Authenticate": "Basic" }));
    const r = await g.getToken();
    expect(r).toMatchObject({
      ok: false,
      falha: { status: 502, corpo: { codigo: "credencial_invalida" }, retryAfter: 60 },
    });
    for (let i = 0; i < 20; i++) {
      agora += 2_900;
      const x = await g.getToken();
      expect(x.ok).toBe(false);
    }
    expect(fetchFn).toHaveBeenCalledTimes(1); // 58 s
    agora += 5_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(2);
    expect(log).toHaveBeenCalledWith("error", expect.stringContaining(ID));
  });

  it("429 com Retry-After: nenhuma tentativa até o prazo", async () => {
    fetchFn.mockImplementation(async () => erro(429, { "Retry-After": "120" }));
    expect(await g.getToken()).toMatchObject({
      ok: false,
      falha: { status: 429, corpo: { codigo: "bloqueado" }, retryAfter: 120 },
    });
    agora += 30_000;
    expect(await g.getToken()).toMatchObject({ falha: { retryAfter: 90 } });
    agora += 89_000;
    await g.getToken();
    expect(fetchFn).toHaveBeenCalledTimes(1);
    agora += 1_000;
    fetchFn.mockResolvedValueOnce(ok("depois"));
    expect(await g.getToken()).toEqual({ ok: true, token: "depois" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("429 sem Retry-After válido assume 60 s", async () => {
    fetchFn.mockImplementation(async () => erro(429, { "Retry-After": "logo" }));
    expect(await g.getToken()).toMatchObject({ falha: { retryAfter: 60 } });
    expect(log).toHaveBeenCalledWith("warn", expect.stringContaining("assumido"));
  });

  it("erro de rede libera a Promise; nova tentativa só após 1 s", async () => {
    fetchFn.mockRejectedValueOnce(new TypeError("fetch failed"));
    const a = await g.getToken();
    expect(a).toMatchObject({
      ok: false,
      falha: { status: 503, corpo: { codigo: "api_indisponivel" } },
    });
    await g.getToken(); // dentro de 1 s: nem tenta
    expect(fetchFn).toHaveBeenCalledTimes(1);
    agora += 1_000;
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("503 e 5xx do token viram api_indisponivel sem quarentena longa", async () => {
    fetchFn.mockResolvedValueOnce(erro(503, { "Retry-After": "5" }));
    expect(await g.getToken()).toMatchObject({ falha: { status: 503 } });
    agora += 1_000;
    expect(await g.getToken()).toEqual({ ok: true, token: "t1" });
  });

  it("resposta 200 fora do contrato é recusada", async () => {
    fetchFn.mockResolvedValueOnce(
      new Response(JSON.stringify({ access_token: "" }), { status: 200 }),
    );
    expect(await g.getToken()).toMatchObject({ ok: false, falha: { status: 502 } });
    fetchFn.mockResolvedValueOnce(new Response("não é json", { status: 200 }));
    agora += 1_000;
    expect(await g.getToken()).toMatchObject({ ok: false });
  });

  it("nenhum log nem falha traz o segredo, o Basic ou o token", async () => {
    fetchFn.mockImplementationOnce(async () => erro(401));
    const a = await g.getToken();
    fetchFn.mockImplementationOnce(async () => erro(429));
    criar();
    const b = await g.getToken();
    const todo = JSON.stringify([a, b, log.mock.calls]);
    expect(todo).not.toContain(SEGREDO);
    expect(todo).not.toContain(BASIC.slice(6));
  });
});
