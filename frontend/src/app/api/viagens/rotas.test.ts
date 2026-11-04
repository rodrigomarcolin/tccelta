// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.fn();
vi.mock("@/auth", () => ({ auth: () => auth(), handlers: { GET: vi.fn(), POST: vi.fn() } }));

import { reiniciarMock } from "@/servidor/viagens/mock/mock-cliente";
import { POST } from "./route";
import { GET as GET_STATUS } from "./[id]/route";
import { GET as GET_ANALISE } from "./[id]/analise/route";
import { EXEMPLOS } from "@/servidor/viagens/mock/cenarios";

const post = (corpo: string) =>
  POST(new Request("http://x/api/viagens", { method: "POST", body: corpo }));
const ctx = (id: string) => ({ params: Promise.resolve({ id }) });
const valido = JSON.stringify({
  motorista: { nome: "Rota" },
  categoria_viagem: "teste",
  combustivel: "diesel",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
});

const UUID = "7d1f3a10-0001-4c5e-9b1a-000000000008";

beforeEach(() => {
  reiniciarMock();
  auth.mockReset();
  vi.stubEnv("OPERATOR_EMAILS", "ana@usp.br, outra@usp.br");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  delete (globalThis as Record<string, unknown>).__tccelta_cliente__;
});

describe("sem sessão", () => {
  beforeEach(() => auth.mockResolvedValue(null));

  it("POST responde 401 com problema", async () => {
    const r = await post(valido);
    expect(r.status).toBe(401);
    expect(r.headers.get("Content-Type")).toBe("application/problem+json");
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(await r.json()).toMatchObject({ codigo: "nao_autenticado", status: 401 });
  });

  it("GET status e análise respondem 401", async () => {
    const id = EXEMPLOS[0].id;
    expect((await GET_STATUS(new Request("http://x"), ctx(id))).status).toBe(401);
    expect((await GET_ANALISE(new Request("http://x"), ctx(id))).status).toBe(401);
  });
});

describe("sessão fora de OPERATOR_EMAILS", () => {
  beforeEach(() => {
    auth.mockResolvedValue({ user: { email: "intrusa@usp.br" } });
    vi.stubEnv("ANALISE_FONTE_DADOS", "api");
    vi.stubEnv("ANALISE_DONGLE_ID", "1");
  });

  it("as três rotas respondem 403 e a API não recebe chamada", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    const leitura = vi.fn(async () => "{}");
    const req = { text: leitura } as unknown as Request;
    const r = await POST(req);
    expect(r.status).toBe(403);
    expect(r.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await r.json()).toMatchObject({ codigo: "nao_autorizado" });
    // O corpo não é lido antes da conferência.
    expect(leitura).not.toHaveBeenCalled();
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(403);
    expect((await GET_ANALISE(new Request("http://x"), ctx(UUID))).status).toBe(403);
    expect(f).not.toHaveBeenCalled();
  });

  it("sem e-mail na sessão também é 403", async () => {
    auth.mockResolvedValue({ user: {} });
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(403);
  });

  it("sem sessão a API não recebe chamada", async () => {
    auth.mockResolvedValue(null);
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    expect((await post(valido)).status).toBe(401);
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(401);
    expect((await GET_ANALISE(new Request("http://x"), ctx(UUID))).status).toBe(401);
    expect(f).not.toHaveBeenCalled();
  });
});

const tokenOk = () =>
  new Response(JSON.stringify({ access_token: "jwt-x", token_type: "Bearer", expires_in: 1800 }), {
    status: 200,
  });

describe("com a fonte api", () => {
  beforeEach(() => {
    auth.mockResolvedValue({ user: { email: "ANA@usp.br" } });
    vi.stubEnv("ANALISE_FONTE_DADOS", "api");
    vi.stubEnv("ANALISE_API_URL", "http://api.teste:8080");
    vi.stubEnv("ANALISE_DONGLE_ID", "3");
    vi.stubEnv("ANALISE_CLIENT_ID", "next-test");
    vi.stubEnv("ANALISE_CLIENT_SECRET", "segredo-de-teste");
  });

  it("repassa Retry-After e o problema da API", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? tokenOk()
        : new Response(
            JSON.stringify({ type: "t", title: "limite_taxa", status: 429, codigo: "limite_taxa" }),
            {
              status: 429,
              headers: { "Content-Type": "application/problem+json", "Retry-After": "7" },
            },
          ),
    );
    vi.stubGlobal("fetch", f);
    const r = await GET_STATUS(new Request("http://x"), ctx(UUID));
    expect(r.status).toBe(429);
    expect(r.headers.get("Retry-After")).toBe("7");
    expect(await r.json()).toMatchObject({ codigo: "limite_taxa" });
    expect((f.mock.calls as unknown as [string][])[0][0]).toBe(
      `http://api.teste:8080/v1/auth/token`,
    );
    expect((f.mock.calls as unknown as [string][])[1][0]).toBe(
      `http://api.teste:8080/v1/viagens/${UUID}`,
    );
  });

  it("id que não é UUID nem chega à API", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    for (const id of ["zzz", "../health", `${UUID}/analise`]) {
      expect((await GET_STATUS(new Request("http://x"), ctx(id))).status).toBe(404);
      expect((await GET_ANALISE(new Request("http://x"), ctx(id))).status).toBe(404);
    }
    expect(f).not.toHaveBeenCalled();
  });

  it("API fora do ar vira 503 api_indisponivel", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new TypeError("fetch failed"))),
    );
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const r = await post(valido);
    expect(r.status).toBe(503);
    expect(await r.json()).toMatchObject({ codigo: "api_indisponivel" });
  });

  it("segredo recusado pela API de token vira 502 credencial_invalida, sem 401 ao navegador", async () => {
    delete (globalThis as Record<string, unknown>).__tccelta_cliente__;
    vi.stubEnv("ANALISE_CLIENT_SECRET", "outro-segredo");
    const f = vi.fn(async () => new Response('{"error":"invalid_client"}', { status: 401 }));
    vi.stubGlobal("fetch", f);
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const r = await GET_STATUS(new Request("http://x"), ctx(UUID));
    expect(r.status).toBe(502);
    expect(await r.json()).toMatchObject({ codigo: "credencial_invalida" });
    const r2 = await GET_STATUS(new Request("http://x"), ctx(UUID));
    expect(r2.status).toBe(502);
    expect(f).toHaveBeenCalledTimes(1);
  });
});

describe("com sessão", () => {
  beforeEach(() => auth.mockResolvedValue({ user: { email: "ana@usp.br" } }));

  it("POST cria, repassa status e corpo e não usa cache", async () => {
    const r = await post(valido);
    expect(r.status).toBe(202);
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(r.headers.get("Content-Type")).toBe("application/json");
    const corpo = (await r.json()) as { viagem_id: string };
    const s = await GET_STATUS(new Request("http://x"), ctx(corpo.viagem_id));
    expect(s.status).toBe(200);
    const a = await GET_ANALISE(new Request("http://x"), ctx(corpo.viagem_id));
    expect(a.status).toBe(409);
    expect(a.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await a.json()).toMatchObject({ viagem_status: "pendente" });
  });

  it("repassa 409 de nome repetido", async () => {
    await post(valido);
    const r = await post(valido);
    expect(r.status).toBe(409);
    expect(await r.json()).toMatchObject({ codigo: "motorista_repetido" });
  });

  it("JSON inválido dá 400 corpo_malformado", async () => {
    const r = await post("{nao é json");
    expect(r.status).toBe(400);
    expect(await r.json()).toMatchObject({ codigo: "corpo_malformado" });
  });

  it("corpo acima de 4 KiB dá 413", async () => {
    const r = await post(JSON.stringify({ x: "a".repeat(5000) }));
    expect(r.status).toBe(413);
    expect(await r.json()).toMatchObject({ codigo: "corpo_grande" });
  });

  it("exemplo pronto devolve a análise", async () => {
    const r = await GET_ANALISE(new Request("http://x"), ctx(EXEMPLOS[0].id));
    expect(r.status).toBe(200);
    expect(await r.json()).toMatchObject({ versao_calculo: "1" });
  });

  it("id desconhecido dá 404", async () => {
    const r = await GET_STATUS(new Request("http://x"), ctx("zzz"));
    expect(r.status).toBe(404);
  });
});
