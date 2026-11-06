// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.fn();
vi.mock("@/auth", () => ({ auth: () => auth(), handlers: { GET: vi.fn(), POST: vi.fn() } }));

import { resetMock } from "@/server/trips/mock/mockDatasource";
import { GET, POST } from "./route";
import { GET as GET_STATUS } from "./[id]/route";
import { GET as GET_ANALYSIS } from "./[id]/analysis/route";
import { EXAMPLES } from "@/server/trips/mock/scenarios";

const post = (body: string) => POST(new Request("http://x/api/trips", { method: "POST", body }));
const get = (qs = "") => GET(new Request(`http://x/api/trips${qs}`));
const ctx = (id: string) => ({ params: Promise.resolve({ id }) });
const validBody = JSON.stringify({
  motorista: { nome: "Rota" },
  categoria_viagem: "teste",
  combustivel: "diesel",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 20 },
});

const UUID = "7d1f3a10-0001-4c5e-9b1a-000000000008";

beforeEach(() => {
  resetMock();
  auth.mockReset();
  vi.stubEnv("OPERATOR_EMAILS", "ana@usp.br, outra@usp.br");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  delete (globalThis as Record<string, unknown>).__tccelta_datasource__;
});

describe("without a session", () => {
  beforeEach(() => auth.mockResolvedValue(null));

  it("POST responds 401 with a problem", async () => {
    const r = await post(validBody);
    expect(r.status).toBe(401);
    expect(r.headers.get("Content-Type")).toBe("application/problem+json");
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(await r.json()).toMatchObject({ codigo: "nao_autenticado", status: 401 });
  });

  it("GET list responds 401 and the data layer is not asked", async () => {
    const r = await get();
    expect(r.status).toBe(401);
    expect(await r.json()).toMatchObject({ codigo: "nao_autenticado" });
  });

  it("GET status and analysis respond 401", async () => {
    const id = EXAMPLES[0].id;
    expect((await GET_STATUS(new Request("http://x"), ctx(id))).status).toBe(401);
    expect((await GET_ANALYSIS(new Request("http://x"), ctx(id))).status).toBe(401);
  });
});

describe("session outside OPERATOR_EMAILS", () => {
  beforeEach(() => {
    auth.mockResolvedValue({ user: { email: "intrusa@usp.br" } });
    vi.stubEnv("ANALISE_FONTE_DADOS", "api");
    vi.stubEnv("ANALISE_DONGLE_ID", "1");
  });

  it("the three routes respond 403 and the API receives no call", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    const readBody = vi.fn(async () => "{}");
    const req = { text: readBody } as unknown as Request;
    const r = await POST(req);
    expect(r.status).toBe(403);
    expect(r.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await r.json()).toMatchObject({ codigo: "nao_autorizado" });
    // The body is not read before the check.
    expect(readBody).not.toHaveBeenCalled();
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(403);
    expect((await GET_ANALYSIS(new Request("http://x"), ctx(UUID))).status).toBe(403);
    expect((await get("?limite=5")).status).toBe(403);
    expect(f).not.toHaveBeenCalled();
  });

  it("no e-mail in the session is also 403", async () => {
    auth.mockResolvedValue({ user: {} });
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(403);
  });

  it("without a session the API receives no call", async () => {
    auth.mockResolvedValue(null);
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    expect((await post(validBody)).status).toBe(401);
    expect((await GET_STATUS(new Request("http://x"), ctx(UUID))).status).toBe(401);
    expect((await GET_ANALYSIS(new Request("http://x"), ctx(UUID))).status).toBe(401);
    expect(f).not.toHaveBeenCalled();
  });
});

const tokenOk = () =>
  new Response(JSON.stringify({ access_token: "jwt-x", token_type: "Bearer", expires_in: 1800 }), {
    status: 200,
  });

describe("with the api source", () => {
  beforeEach(() => {
    auth.mockResolvedValue({ user: { email: "ANA@usp.br" } });
    vi.stubEnv("ANALISE_FONTE_DADOS", "api");
    vi.stubEnv("ANALISE_API_URL", "http://api.teste:8080");
    vi.stubEnv("ANALISE_DONGLE_ID", "3");
    vi.stubEnv("ANALISE_CLIENT_ID", "next-test");
    vi.stubEnv("ANALISE_CLIENT_SECRET", "segredo-de-teste");
  });

  it("forwards Retry-After and the API problem", async () => {
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

  it("GET list forwards only limite, cursor and busca, encoded, and never reaches the API with a bad value", async () => {
    const f = vi.fn(async (url: string | URL | Request) =>
      String(url).endsWith("/v1/auth/token")
        ? tokenOk()
        : new Response(JSON.stringify({ viagens: [], total: 0, proximo_cursor: null }), {
            status: 200,
          }),
    );
    vi.stubGlobal("fetch", f);
    const r = await get("?limite=24&cursor=a%2Bb%3D&busca=Jo%C3%A3o%20%26&x=1");
    expect(r.status).toBe(200);
    const calls = f.mock.calls as unknown as [string][];
    const url = new URL(calls[calls.length - 1][0]);
    expect(url.pathname).toBe("/v1/viagens");
    expect(url.searchParams.get("cursor")).toBe("a+b=");
    expect(url.searchParams.get("busca")).toBe("João &");
    expect([...url.searchParams.keys()]).toEqual(["limite", "cursor", "busca"]);

    const before = f.mock.calls.length;
    for (const qs of ["?limite=0", "?limite=51", "?limite=abc", "?busca=" + "a".repeat(61)]) {
      const bad = await get(qs);
      expect(bad.status, qs).toBe(400);
      expect(await bad.json()).toMatchObject({ codigo: "entrada_invalida" });
    }
    expect(f.mock.calls.length).toBe(before);
  });

  it("an id that is not a UUID never reaches the API", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    for (const id of ["zzz", "../health", `${UUID}/analysis`]) {
      expect((await GET_STATUS(new Request("http://x"), ctx(id))).status).toBe(404);
      expect((await GET_ANALYSIS(new Request("http://x"), ctx(id))).status).toBe(404);
    }
    expect(f).not.toHaveBeenCalled();
  });

  it("API down becomes 503 api_indisponivel", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new TypeError("fetch failed"))),
    );
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const r = await post(validBody);
    expect(r.status).toBe(503);
    expect(await r.json()).toMatchObject({ codigo: "api_indisponivel" });
  });

  it("secret rejected by the token API becomes 502 credencial_invalida, with no 401 to the browser", async () => {
    delete (globalThis as Record<string, unknown>).__tccelta_datasource__;
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

describe("with a session", () => {
  beforeEach(() => auth.mockResolvedValue({ user: { email: "ana@usp.br" } }));

  it("POST creates, forwards status and body and does not cache", async () => {
    const r = await post(validBody);
    expect(r.status).toBe(202);
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(r.headers.get("Content-Type")).toBe("application/json");
    const created = (await r.json()) as { viagem_id: string };
    const s = await GET_STATUS(new Request("http://x"), ctx(created.viagem_id));
    expect(s.status).toBe(200);
    const a = await GET_ANALYSIS(new Request("http://x"), ctx(created.viagem_id));
    expect(a.status).toBe(409);
    expect(a.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await a.json()).toMatchObject({ viagem_status: "pendente" });
  });

  it("forwards 409 for a repeated name", async () => {
    await post(validBody);
    // Another boot: the same one would be viagem_repetida.
    const r = await post(
      JSON.stringify({
        ...JSON.parse(validBody),
        entrada: { tipo: "boot", dongle_id: 1, boot_id: 21 },
      }),
    );
    expect(r.status).toBe(409);
    expect(await r.json()).toMatchObject({ codigo: "motorista_repetido" });
  });

  it("GET list returns the first page of the mock, without cache", async () => {
    const r = await get();
    expect(r.status).toBe(200);
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    const page = await r.json();
    expect(page.viagens).toHaveLength(24);
    expect(page.total).toBeGreaterThan(24);
    expect(page.proximo_cursor).toEqual(expect.any(String));
    const next = await (await get(`?cursor=${encodeURIComponent(page.proximo_cursor)}`)).json();
    expect(next.viagens[0].viagem_id).not.toBe(page.viagens[0].viagem_id);
  });

  it("GET list with a bad cursor gives 400", async () => {
    const r = await get("?cursor=!!!");
    expect(r.status).toBe(400);
    expect(await r.json()).toMatchObject({ codigo: "entrada_invalida" });
  });

  it("forwards 409 viagem_repetida with the existing trip", async () => {
    const r = await post(
      JSON.stringify({
        ...JSON.parse(validBody),
        entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
      }),
    );
    expect(r.status).toBe(409);
    expect(await r.json()).toMatchObject({
      codigo: "viagem_repetida",
      viagem_id: EXAMPLES[0].id,
      motorista: { nome: "Exemplo — Boot normal" },
    });
  });

  it("invalid JSON gives 400 corpo_malformado", async () => {
    const r = await post("{nao é json");
    expect(r.status).toBe(400);
    expect(await r.json()).toMatchObject({ codigo: "corpo_malformado" });
  });

  it("body above 4 KiB gives 413", async () => {
    const r = await post(JSON.stringify({ x: "a".repeat(5000) }));
    expect(r.status).toBe(413);
    expect(await r.json()).toMatchObject({ codigo: "corpo_grande" });
  });

  it("a ready example returns the analysis", async () => {
    const r = await GET_ANALYSIS(new Request("http://x"), ctx(EXAMPLES[0].id));
    expect(r.status).toBe(200);
    expect(await r.json()).toMatchObject({ versao_calculo: "1" });
  });

  it("unknown id gives 404", async () => {
    const r = await GET_STATUS(new Request("http://x"), ctx("zzz"));
    expect(r.status).toBe(404);
  });
});
