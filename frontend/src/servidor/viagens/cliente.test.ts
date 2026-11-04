import { afterEach, describe, expect, it, vi } from "vitest";
import { dongleInicial, escolherFonte, lerConfig, obterCliente } from "./cliente";

const API = {
  ANALISE_FONTE_DADOS: "api",
  ANALISE_DONGLE_ID: "4",
  ANALISE_CLIENT_ID: "next-test",
  ANALISE_CLIENT_SECRET: "segredo-de-teste",
};

afterEach(() => {
  vi.unstubAllGlobals();
  delete (globalThis as Record<string, unknown>).__tccelta_cliente__;
});

describe("escolherFonte", () => {
  it("usa mock em desenvolvimento quando a variável falta", () => {
    expect(escolherFonte({ NODE_ENV: "development" })).toBe("mock");
    expect(escolherFonte({ NODE_ENV: "test", ANALISE_FONTE_DADOS: "  " })).toBe("mock");
  });
  it("em produção a variável é obrigatória", () => {
    expect(() => escolherFonte({ NODE_ENV: "production" })).toThrow(/ANALISE_FONTE_DADOS/);
    expect(() => escolherFonte({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "" })).toThrow();
  });
  it("aceita mock explícito em produção", () => {
    expect(escolherFonte({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "mock" })).toBe("mock");
  });
  it("api exige o dongle", () => {
    expect(() => escolherFonte({ ANALISE_FONTE_DADOS: "api" })).toThrow(/ANALISE_DONGLE_ID/);
    expect(escolherFonte(API)).toBe("api");
  });
  it("recusa valor desconhecido", () => {
    expect(() => escolherFonte({ ANALISE_FONTE_DADOS: "banco" })).toThrow(/desconhecida/);
  });
});

describe("obterCliente", () => {
  it("devolve sempre a mesma instância", () => {
    const a = obterCliente({ NODE_ENV: "test" });
    expect(obterCliente({ NODE_ENV: "test" })).toBe(a);
  });
  it("não sobe em produção sem a variável", () => {
    expect(() => obterCliente({ NODE_ENV: "production" })).toThrow();
  });
});

describe("lerConfig com a fonte api", () => {
  it("usa a URL local e o timeout padrão em desenvolvimento", () => {
    expect(lerConfig({ ...API, NODE_ENV: "development" })).toEqual({
      fonte: "api",
      url: "http://127.0.0.1:8080",
      timeoutMs: 10_000,
      dongleId: 4,
      clientId: "next-test",
      clientSecret: "segredo-de-teste",
    });
  });
  it("em produção a URL é obrigatória", () => {
    expect(() => lerConfig({ ...API, NODE_ENV: "production" })).toThrow(/ANALISE_API_URL/);
  });
  it("tira a barra final e lê o timeout", () => {
    expect(
      lerConfig({
        ...API,
        ANALISE_API_URL: "http://api:8080//",
        ANALISE_TIMEOUT_MS: "2500",
      }),
    ).toMatchObject({ url: "http://api:8080", timeoutMs: 2500 });
  });
  it("recusa URL, protocolo, dongle e timeout inválidos", () => {
    expect(() => lerConfig({ ...API, ANALISE_API_URL: "api" })).toThrow(/URL válida/);
    expect(() => lerConfig({ ...API, ANALISE_API_URL: "ftp://x" })).toThrow(/http/);
    expect(() => lerConfig({ ...API, ANALISE_DONGLE_ID: "0" })).toThrow(/ANALISE_DONGLE_ID/);
    expect(() => lerConfig({ ...API, ANALISE_DONGLE_ID: "1.5" })).toThrow(/inteiro/);
    expect(() => lerConfig({ ...API, ANALISE_TIMEOUT_MS: "-3" })).toThrow(/ANALISE_TIMEOUT_MS/);
  });
});

describe("dongleInicial", () => {
  it("vem de ANALISE_DONGLE_ID com a api e é 1 com o mock", () => {
    expect(dongleInicial(API)).toBe(4);
    expect(dongleInicial({ NODE_ENV: "test" })).toBe(1);
  });
});

describe("obterCliente com a api", () => {
  it("troca de cliente quando ANALISE_API_URL muda, sem recompilar", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? new Response(JSON.stringify({ access_token: "t", expires_in: 1800 }), { status: 200 })
        : new Response("{}", { status: 200 }),
    );
    vi.stubGlobal("fetch", f);
    const id = "7d1f3a10-0001-4c5e-9b1a-000000000008";
    const a = obterCliente({ ...API, ANALISE_API_URL: "http://a:1" });
    expect(obterCliente({ ...API, ANALISE_API_URL: "http://a:1" })).toBe(a);
    await a.status(id);
    const b = obterCliente({ ...API, ANALISE_API_URL: "http://b:2" });
    expect(b).not.toBe(a);
    await b.status(id);
    expect(f.mock.calls.map((c) => (c as unknown as [string])[0])).toEqual([
      "http://a:1/v1/auth/token",
      `http://a:1/v1/viagens/${id}`,
      "http://b:2/v1/auth/token",
      `http://b:2/v1/viagens/${id}`,
    ]);
  });
  it("com mock não chama a rede", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    await obterCliente({ NODE_ENV: "test" }).status("7d1f3a10-0001-4c5e-9b1a-000000000008");
    expect(f).not.toHaveBeenCalled();
  });
});

describe("credenciais do cliente da API", () => {
  it("ANALISE_CLIENT_ID e ANALISE_CLIENT_SECRET são obrigatórias, sem chamar a rede", () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    const semId = { ...API, ANALISE_CLIENT_ID: undefined };
    const semSegredo = { ...API, ANALISE_CLIENT_SECRET: undefined };
    expect(() => lerConfig(semId)).toThrow(/ANALISE_CLIENT_ID/);
    expect(() => lerConfig(semSegredo)).toThrow(/ANALISE_CLIENT_SECRET/);
    expect(() => lerConfig({ ...API, ANALISE_CLIENT_SECRET: "  " })).toThrow(
      /ANALISE_CLIENT_SECRET/,
    );
    expect(() => obterCliente(semSegredo)).toThrow();
    expect(f).not.toHaveBeenCalled();
  });
  it("o cliente obtém o token e chama a API com Bearer", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? new Response(JSON.stringify({ access_token: "jwt-x", expires_in: 1800 }), {
            status: 200,
          })
        : new Response("{}", { status: 200 }),
    );
    vi.stubGlobal("fetch", f);
    await obterCliente({ ...API, ANALISE_API_URL: "http://c:3" }).status(
      "7d1f3a10-0001-4c5e-9b1a-000000000008",
    );
    const chamadas = f.mock.calls as unknown as [string, RequestInit][];
    expect(chamadas[0][0]).toBe("http://c:3/v1/auth/token");
    expect((chamadas[1][1].headers as Record<string, string>).Authorization).toBe("Bearer jwt-x");
  });
});
