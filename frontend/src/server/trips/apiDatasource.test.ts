// @vitest-environment node
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { isProblem, type TripAnalysis } from "@/domain/trip/types";
import { createApiDatasource } from "./apiDatasource";
import type { TokenManager } from "./token";
import { buildAnalysis } from "./mock/analyses";
import type { ScenarioId } from "./mock/scenarios";
import { contractViolations } from "@/tests/contract";

const SCENARIOS: ScenarioId[] = [
  "normal",
  "sem_gps",
  "gps_falhas",
  "sem_hora",
  "sem_leituras",
  "provisoria_aguardando",
  "pid_ausente",
  "dados_parciais",
  "intervalo",
];

type Call = {
  method: string;
  url: string;
  contentType: string | undefined;
  auth: string | undefined;
  body: string;
};
type FakeResponse = {
  status: number;
  contentType?: string;
  body?: string;
  headers?: Record<string, string>;
  /** Delay before responding (for the timeout). */
  delayMs?: number;
};

const ID = "7d1f3a10-0001-4c5e-9b1a-000000000008";
const problemBody = (status: number, codigo: string, extra: object = {}) =>
  JSON.stringify({
    type: `https://tccelta.local/erros/${codigo}`,
    title: codigo,
    status,
    codigo,
    ...extra,
  });

let httpServer: Server;
let base: string;
let calls: Call[];
let next: FakeResponse;
const TOKEN = "tk-sentinel";
let tokens: TokenManager;
let log: ReturnType<typeof vi.fn<(message: string) => void>>;

beforeAll(async () => {
  httpServer = createServer((req: IncomingMessage, res: ServerResponse) => {
    const chunks: Buffer[] = [];
    req.on("data", (p: Buffer) => chunks.push(p));
    req.on("end", () => {
      calls.push({
        method: req.method ?? "",
        url: req.url ?? "",
        contentType: req.headers["content-type"],
        auth: req.headers.authorization,
        body: Buffer.concat(chunks).toString("utf8"),
      });
      const r = next;
      const send = () => {
        res.writeHead(r.status, {
          "Content-Type": r.contentType ?? "application/json",
          ...r.headers,
        });
        res.end(r.body ?? "");
      };
      if (r.delayMs) setTimeout(send, r.delayMs);
      else send();
    });
  });
  await new Promise<void>((ok) => httpServer.listen(0, "127.0.0.1", ok));
  base = `http://127.0.0.1:${(httpServer.address() as AddressInfo).port}`;
});

afterAll(async () => {
  httpServer.closeAllConnections();
  await new Promise((ok) => httpServer.close(ok));
});

beforeEach(() => {
  calls = [];
  next = { status: 200, body: "{}" };
  log = vi.fn<(message: string) => void>();
  tokens = {
    getToken: vi.fn(async () => ({ ok: true as const, token: TOKEN })),
    invalidate: vi.fn(),
  };
});

const client = (timeoutMs = 2000) => createApiDatasource({ baseUrl: base, timeoutMs, tokens, log });

describe("create (POST /v1/viagens)", () => {
  const requestBody = {
    motorista: { nome: "Ana" },
    categoria_viagem: "feira",
    combustivel: "gasolina_e30",
    entrada: { tipo: "boot", dongle_id: 3, boot_id: 8 },
  };

  it("sends the JSON unchanged and returns 202", async () => {
    next = {
      status: 202,
      body: JSON.stringify({ viagem_id: ID, status: "pendente" }),
      headers: { Location: `/v1/viagens/${ID}` },
    };
    const r = await client().create(requestBody);
    expect(r).toEqual({ status: 202, body: { viagem_id: ID, status: "pendente" } });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      method: "POST",
      url: "/v1/viagens",
      contentType: "application/json",
    });
    expect(JSON.parse(calls[0].body)).toEqual(requestBody);
  });

  it.each([
    [400, "entrada_invalida"],
    [400, "corpo_malformado"],
    [404, "dongle_desconhecido"],
    [409, "motorista_repetido"],
    [413, "corpo_grande"],
    [415, "tipo_conteudo"],
    [503, "banco_ocupado"],
  ])("repassa o problema %i %s com o codigo", async (status, codigo) => {
    next = { status, contentType: "application/problem+json", body: problemBody(status, codigo) };
    const r = await client().create(requestBody);
    expect(r.status).toBe(status);
    expect(r.body).toMatchObject({ codigo, status });
  });

  it("429 forwards Retry-After in seconds", async () => {
    next = {
      status: 429,
      contentType: "application/problem+json",
      body: problemBody(429, "limite_taxa"),
      headers: { "Retry-After": "3" },
    };
    const r = await client().create(requestBody);
    expect(r).toMatchObject({ status: 429, retryAfter: 3, body: { codigo: "limite_taxa" } });
  });

  it("503 forwards Retry-After and an invalid Retry-After is ignored", async () => {
    next = {
      status: 503,
      contentType: "application/problem+json",
      body: problemBody(503, "banco_ocupado"),
      headers: { "Retry-After": "11" },
    };
    expect((await client().create(requestBody)).retryAfter).toBe(11);
    next = { ...next, headers: { "Retry-After": "tomorrow" } };
    expect((await client().create(requestBody)).retryAfter).toBeUndefined();
  });

  it("5xx without codigo becomes 502 resposta_invalida and the detail goes only to the log", async () => {
    next = { status: 500, contentType: "application/json", body: '{"erro":"panic: segredo"}' };
    const r = await client().create(requestBody);
    expect(r.status).toBe(502);
    expect(r.body).toMatchObject({ codigo: "resposta_invalida" });
    expect(JSON.stringify(r.body)).not.toContain("segredo");
    expect(log).toHaveBeenCalledWith(expect.stringContaining("500"));
  });

  it("a body that is not JSON becomes 502 resposta_invalida", async () => {
    next = { status: 502, contentType: "text/html", body: "<html>Bad Gateway</html>" };
    const r = await client().create(requestBody);
    expect(r.status).toBe(502);
    expect(r.body).toMatchObject({ codigo: "resposta_invalida" });
    next = { status: 202, body: "not json" };
    expect((await client().create(requestBody)).status).toBe(502);
  });

  it("does not repeat the POST by itself, even on error", async () => {
    next = {
      status: 503,
      contentType: "application/problem+json",
      body: problemBody(503, "banco_ocupado"),
    };
    await client().create(requestBody);
    expect(calls).toHaveLength(1);
  });
});

describe("status and analysis", () => {
  it("GET /v1/viagens/{id} returns the JSON without rewriting fields", async () => {
    const st = {
      viagem_id: ID,
      status: "pronta",
      provisoria: true,
      motorista: { nome: "Ana" },
      categoria_viagem: "feira",
      combustivel: "gasolina_e30",
      entrada: { tipo: "boot", dongle_id: 3, boot_id: 8, boot_generation: null },
      criada_em: "2026-10-28T17:00:00.000Z",
      calculada_em: "2026-10-28T17:00:20.000Z",
    };
    next = { status: 200, body: JSON.stringify(st) };
    const r = await client().status(ID.toUpperCase());
    expect(r).toEqual({ status: 200, body: st });
    expect(calls[0]).toMatchObject({ method: "GET", url: `/v1/viagens/${ID}` });
  });

  it.each(SCENARIOS)(
    "analysis of scenario %s arrives identical to the mock example",
    async (scenario) => {
      const expected: TripAnalysis = buildAnalysis(
        scenario,
        {
          input: { tipo: "boot", dongle_id: 1, boot_id: 8 },
          driver: { id: ID, nome: "Ana" },
          category: "feira",
          fuel: "gasolina_e30",
        },
        true,
      );
      next = { status: 200, body: JSON.stringify(expected) };
      const r = await client().analysis(ID);
      expect(r.status).toBe(200);
      expect(r.body).toEqual(expected);
      expect(contractViolations(r.body as TripAnalysis)).toEqual([]);
      expect(calls[0].url).toBe(`/v1/viagens/${ID}/analise`);
    },
  );

  it("409 analise_indisponivel carries viagem_status", async () => {
    next = {
      status: 409,
      contentType: "application/problem+json",
      body: problemBody(409, "analise_indisponivel", { viagem_status: "pendente" }),
    };
    const r = await client().analysis(ID);
    expect(r.status).toBe(409);
    expect(r.body).toMatchObject({ codigo: "analise_indisponivel", viagem_status: "pendente" });
  });

  it("404 viagem_desconhecida is forwarded", async () => {
    next = {
      status: 404,
      contentType: "application/problem+json",
      body: problemBody(404, "viagem_desconhecida"),
    };
    expect((await client().status(ID)).body).toMatchObject({ codigo: "viagem_desconhecida" });
  });

  it("an id outside the canonical format never reaches the API", async () => {
    for (const id of ["abc", "../x", `${ID}/analise`, ""]) {
      const s = await client().status(id);
      const a = await client().analysis(id);
      expect(s.status).toBe(404);
      expect(a.status).toBe(404);
      expect(isProblem(s.body) && s.body.codigo).toBe("viagem_desconhecida");
    }
    expect(calls).toHaveLength(0);
  });
});

describe("transport failures", () => {
  it("timeout becomes 503 api_indisponivel", async () => {
    next = { status: 200, body: "{}", delayMs: 500 };
    const r = await client(50).status(ID);
    expect(r.status).toBe(503);
    expect(r.body).toMatchObject({ codigo: "api_indisponivel" });
    expect(log).toHaveBeenCalled();
  });

  it("connection refused becomes 503 api_indisponivel", async () => {
    const dead = createApiDatasource({
      baseUrl: "http://127.0.0.1:1",
      timeoutMs: 1000,
      tokens,
      log,
    });
    const r = await dead.analysis(ID);
    expect(r.status).toBe(503);
    expect(r.body).toMatchObject({ codigo: "api_indisponivel" });
  });

  it("redirects are not followed", async () => {
    next = { status: 302, headers: { Location: "http://other.example/" }, body: "" };
    const r = await client().status(ID);
    expect(r.status).toBe(503);
  });
});

describe("fetch options", () => {
  it("uses cache no-store, timeout and the given base", async () => {
    const fetchFn = vi.fn(async () => new Response("{}", { status: 200 }));
    const c = createApiDatasource({ baseUrl: "http://api:8080", timeoutMs: 1234, tokens, fetchFn });
    await c.status(ID);
    const [url, init] = fetchFn.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe(`http://api:8080/v1/viagens/${ID}`);
    expect(init.cache).toBe("no-store");
    expect(init.signal).toBeInstanceOf(AbortSignal);
    expect(init.headers).toHaveProperty("Authorization", `Bearer ${TOKEN}`);
  });
});

describe("Bearer and credential errors", () => {
  it("every call carries the manager Bearer, on GET and POST", async () => {
    next = { status: 202, body: JSON.stringify({ viagem_id: ID, status: "pendente" }) };
    await client().create({});
    await client().status(ID);
    await client().analysis(ID);
    expect(calls.map((c) => c.auth)).toEqual([
      `Bearer ${TOKEN}`,
      `Bearer ${TOKEN}`,
      `Bearer ${TOKEN}`,
    ]);
  });

  it("the token and the secret reach neither the log nor the error response", async () => {
    next = { status: 500, body: "{}" };
    const r = await client().status(ID);
    expect(JSON.stringify(r)).not.toContain(TOKEN);
    expect(JSON.stringify(log.mock.calls)).not.toContain(TOKEN);
  });

  it("401: invalidates, renews and repeats once with the new token", async () => {
    let n = 0;
    tokens = {
      getToken: vi.fn(async () => ({ ok: true as const, token: `t${++n}` })),
      invalidate: vi.fn(),
    };
    let respostas = 0;
    const fetchFn = vi.fn(async (_u: unknown, init?: RequestInit) => {
      respostas++;
      const auth = (init?.headers as Record<string, string>).Authorization;
      return respostas === 1
        ? new Response(problemBody(401, "token_expirado"), {
            status: 401,
            headers: { "Content-Type": "application/problem+json" },
          })
        : new Response(JSON.stringify({ ok: auth }), { status: 200 });
    });
    const c = createApiDatasource({ baseUrl: base, timeoutMs: 1000, tokens, fetchFn, log });
    const r = await c.status(ID);
    expect(r).toEqual({ status: 200, body: { ok: "Bearer t2" } });
    expect(tokens.invalidate).toHaveBeenCalledExactlyOnceWith("t1");
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("a second 401 becomes 502 credencial_invalida, with no third attempt", async () => {
    next = {
      status: 401,
      contentType: "application/problem+json",
      body: problemBody(401, "token_invalido"),
    };
    const r = await client().create({ a: 1 });
    expect(r.status).toBe(502);
    expect(r.body).toMatchObject({ codigo: "credencial_invalida" });
    expect(calls).toHaveLength(2);
    expect(tokens.getToken).toHaveBeenCalledTimes(2);
  });

  it("403 becomes 502 credencial_invalida, without repeating", async () => {
    next = {
      status: 403,
      contentType: "application/problem+json",
      body: problemBody(403, "proibido"),
    };
    const r = await client().status(ID);
    expect(r.status).toBe(502);
    expect(r.body).toMatchObject({ codigo: "credencial_invalida" });
    expect(calls).toHaveLength(1);
  });

  it("a manager failure is returned without calling the trips API", async () => {
    tokens = {
      getToken: vi.fn(async () => ({
        ok: false as const,
        failure: {
          status: 429,
          body: { type: "x", title: "bloqueado", status: 429, codigo: "bloqueado" },
          retryAfter: 42,
        },
      })),
      invalidate: vi.fn(),
    };
    const r = await client().status(ID);
    expect(r).toMatchObject({ status: 429, retryAfter: 42, body: { codigo: "bloqueado" } });
    expect(calls).toHaveLength(0);
  });

  it("a timeout after the token does not repeat the request", async () => {
    next = { status: 200, body: "{}", delayMs: 300 };
    const r = await client(40).create({});
    expect(r.status).toBe(503);
    expect(calls).toHaveLength(1);
  });
});

describe("list (GET /v1/viagens)", () => {
  const page = { viagens: [], total: 0, proximo_cursor: null };

  it("forwards limite, cursor and busca encoded, with the Bearer", async () => {
    next = { status: 200, body: JSON.stringify(page) };
    const r = await client().list({ limite: 24, cursor: "a+b/c==", busca: "João & Ana" });
    expect(r).toEqual({ status: 200, body: page });
    expect(calls[0]).toMatchObject({ method: "GET", auth: `Bearer ${TOKEN}` });
    const url = new URL(calls[0].url, "http://x");
    expect(url.pathname).toBe("/v1/viagens");
    expect(url.searchParams.get("limite")).toBe("24");
    expect(url.searchParams.get("cursor")).toBe("a+b/c==");
    expect(url.searchParams.get("busca")).toBe("João & Ana");
    expect([...url.searchParams.keys()]).toEqual(["limite", "cursor", "busca"]);
  });

  it("sends no query string without parameters", async () => {
    next = { status: 200, body: JSON.stringify(page) };
    await client().list({});
    expect(calls[0].url).toBe("/v1/viagens");
  });

  it("forwards the API problem", async () => {
    next = {
      status: 400,
      contentType: "application/problem+json",
      body: problemBody(400, "entrada_invalida"),
    };
    const r = await client().list({ limite: 24 });
    expect(r).toMatchObject({ status: 400, body: { codigo: "entrada_invalida" } });
  });
});
