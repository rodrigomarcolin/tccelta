// @vitest-environment node
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import type { AddressInfo } from "node:net";
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { ehProblema, type Analise } from "@/contrato/tipos";
import { criarApiCliente } from "./api-cliente";
import type { GerenciadorToken } from "./token";
import { construirAnalise } from "./mock/analises";
import type { CenarioId } from "./mock/cenarios";
import { violacoesDoContrato } from "@/testes/contrato";

const CENARIOS: CenarioId[] = [
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

type Chamada = {
  metodo: string;
  url: string;
  tipo: string | undefined;
  auth: string | undefined;
  corpo: string;
};
type Resposta = {
  status: number;
  tipo?: string;
  corpo?: string;
  cabecalhos?: Record<string, string>;
  /** Atraso antes de responder (para o timeout). */
  atrasoMs?: number;
};

const ID = "7d1f3a10-0001-4c5e-9b1a-000000000008";
const problema = (status: number, codigo: string, extra: object = {}) =>
  JSON.stringify({
    type: `https://tccelta.local/erros/${codigo}`,
    title: codigo,
    status,
    codigo,
    ...extra,
  });

let servidor: Server;
let base: string;
let chamadas: Chamada[];
let proxima: Resposta;
const TOKEN = "tk-sentinela";
let tokens: GerenciadorToken;
let log: ReturnType<typeof vi.fn<(mensagem: string) => void>>;

beforeAll(async () => {
  servidor = createServer((req: IncomingMessage, res: ServerResponse) => {
    const partes: Buffer[] = [];
    req.on("data", (p: Buffer) => partes.push(p));
    req.on("end", () => {
      chamadas.push({
        metodo: req.method ?? "",
        url: req.url ?? "",
        tipo: req.headers["content-type"],
        auth: req.headers.authorization,
        corpo: Buffer.concat(partes).toString("utf8"),
      });
      const r = proxima;
      const enviar = () => {
        res.writeHead(r.status, {
          "Content-Type": r.tipo ?? "application/json",
          ...r.cabecalhos,
        });
        res.end(r.corpo ?? "");
      };
      if (r.atrasoMs) setTimeout(enviar, r.atrasoMs);
      else enviar();
    });
  });
  await new Promise<void>((ok) => servidor.listen(0, "127.0.0.1", ok));
  base = `http://127.0.0.1:${(servidor.address() as AddressInfo).port}`;
});

afterAll(async () => {
  servidor.closeAllConnections();
  await new Promise((ok) => servidor.close(ok));
});

beforeEach(() => {
  chamadas = [];
  proxima = { status: 200, corpo: "{}" };
  log = vi.fn<(mensagem: string) => void>();
  tokens = {
    getToken: vi.fn(async () => ({ ok: true as const, token: TOKEN })),
    invalidate: vi.fn(),
  };
});

const cliente = (timeoutMs = 2000) => criarApiCliente({ baseUrl: base, timeoutMs, tokens, log });

describe("criar (POST /v1/viagens)", () => {
  const corpo = {
    motorista: { nome: "Ana" },
    categoria_viagem: "feira",
    combustivel: "gasolina_e30",
    entrada: { tipo: "boot", dongle_id: 3, boot_id: 8 },
  };

  it("envia o JSON sem alterar e devolve 202", async () => {
    proxima = {
      status: 202,
      corpo: JSON.stringify({ viagem_id: ID, status: "pendente" }),
      cabecalhos: { Location: `/v1/viagens/${ID}` },
    };
    const r = await cliente().criar(corpo);
    expect(r).toEqual({ status: 202, corpo: { viagem_id: ID, status: "pendente" } });
    expect(chamadas).toHaveLength(1);
    expect(chamadas[0]).toMatchObject({
      metodo: "POST",
      url: "/v1/viagens",
      tipo: "application/json",
    });
    expect(JSON.parse(chamadas[0].corpo)).toEqual(corpo);
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
    proxima = { status, tipo: "application/problem+json", corpo: problema(status, codigo) };
    const r = await cliente().criar(corpo);
    expect(r.status).toBe(status);
    expect(r.corpo).toMatchObject({ codigo, status });
  });

  it("429 repassa Retry-After em segundos", async () => {
    proxima = {
      status: 429,
      tipo: "application/problem+json",
      corpo: problema(429, "limite_taxa"),
      cabecalhos: { "Retry-After": "3" },
    };
    const r = await cliente().criar(corpo);
    expect(r).toMatchObject({ status: 429, retryAfter: 3, corpo: { codigo: "limite_taxa" } });
  });

  it("503 repassa Retry-After e Retry-After inválido é ignorado", async () => {
    proxima = {
      status: 503,
      tipo: "application/problem+json",
      corpo: problema(503, "banco_ocupado"),
      cabecalhos: { "Retry-After": "11" },
    };
    expect((await cliente().criar(corpo)).retryAfter).toBe(11);
    proxima = { ...proxima, cabecalhos: { "Retry-After": "amanhã" } };
    expect((await cliente().criar(corpo)).retryAfter).toBeUndefined();
  });

  it("5xx sem codigo vira 502 resposta_invalida e o detalhe vai só ao log", async () => {
    proxima = { status: 500, tipo: "application/json", corpo: '{"erro":"panic: segredo"}' };
    const r = await cliente().criar(corpo);
    expect(r.status).toBe(502);
    expect(r.corpo).toMatchObject({ codigo: "resposta_invalida" });
    expect(JSON.stringify(r.corpo)).not.toContain("segredo");
    expect(log).toHaveBeenCalledWith(expect.stringContaining("500"));
  });

  it("corpo que não é JSON vira 502 resposta_invalida", async () => {
    proxima = { status: 502, tipo: "text/html", corpo: "<html>Bad Gateway</html>" };
    const r = await cliente().criar(corpo);
    expect(r.status).toBe(502);
    expect(r.corpo).toMatchObject({ codigo: "resposta_invalida" });
    proxima = { status: 202, corpo: "não é json" };
    expect((await cliente().criar(corpo)).status).toBe(502);
  });

  it("não repete o POST sozinho, nem em erro", async () => {
    proxima = {
      status: 503,
      tipo: "application/problem+json",
      corpo: problema(503, "banco_ocupado"),
    };
    await cliente().criar(corpo);
    expect(chamadas).toHaveLength(1);
  });
});

describe("status e análise", () => {
  it("GET /v1/viagens/{id} devolve o JSON sem reescrever campos", async () => {
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
    proxima = { status: 200, corpo: JSON.stringify(st) };
    const r = await cliente().status(ID.toUpperCase());
    expect(r).toEqual({ status: 200, corpo: st });
    expect(chamadas[0]).toMatchObject({ metodo: "GET", url: `/v1/viagens/${ID}` });
  });

  it.each(CENARIOS)("análise do cenário %s chega idêntica ao exemplo do mock", async (cenario) => {
    const esperado: Analise = construirAnalise(
      cenario,
      {
        entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
        motorista: { id: ID, nome: "Ana" },
        categoria: "feira",
        combustivel: "gasolina_e30",
      },
      true,
    );
    proxima = { status: 200, corpo: JSON.stringify(esperado) };
    const r = await cliente().analise(ID);
    expect(r.status).toBe(200);
    expect(r.corpo).toEqual(esperado);
    expect(violacoesDoContrato(r.corpo as Analise)).toEqual([]);
    expect(chamadas[0].url).toBe(`/v1/viagens/${ID}/analise`);
  });

  it("409 analise_indisponivel traz viagem_status", async () => {
    proxima = {
      status: 409,
      tipo: "application/problem+json",
      corpo: problema(409, "analise_indisponivel", { viagem_status: "pendente" }),
    };
    const r = await cliente().analise(ID);
    expect(r.status).toBe(409);
    expect(r.corpo).toMatchObject({ codigo: "analise_indisponivel", viagem_status: "pendente" });
  });

  it("404 viagem_desconhecida é repassado", async () => {
    proxima = {
      status: 404,
      tipo: "application/problem+json",
      corpo: problema(404, "viagem_desconhecida"),
    };
    expect((await cliente().status(ID)).corpo).toMatchObject({ codigo: "viagem_desconhecida" });
  });

  it("id fora do formato canônico nem chega à API", async () => {
    for (const id of ["abc", "../x", `${ID}/analise`, ""]) {
      const s = await cliente().status(id);
      const a = await cliente().analise(id);
      expect(s.status).toBe(404);
      expect(a.status).toBe(404);
      expect(ehProblema(s.corpo) && s.corpo.codigo).toBe("viagem_desconhecida");
    }
    expect(chamadas).toHaveLength(0);
  });
});

describe("falhas de transporte", () => {
  it("timeout vira 503 api_indisponivel", async () => {
    proxima = { status: 200, corpo: "{}", atrasoMs: 500 };
    const r = await cliente(50).status(ID);
    expect(r.status).toBe(503);
    expect(r.corpo).toMatchObject({ codigo: "api_indisponivel" });
    expect(log).toHaveBeenCalled();
  });

  it("conexão recusada vira 503 api_indisponivel", async () => {
    const morto = criarApiCliente({ baseUrl: "http://127.0.0.1:1", timeoutMs: 1000, tokens, log });
    const r = await morto.analise(ID);
    expect(r.status).toBe(503);
    expect(r.corpo).toMatchObject({ codigo: "api_indisponivel" });
  });

  it("redirecionamento não é seguido", async () => {
    proxima = { status: 302, cabecalhos: { Location: "http://outro.exemplo/" }, corpo: "" };
    const r = await cliente().status(ID);
    expect(r.status).toBe(503);
  });
});

describe("opções de fetch", () => {
  it("usa cache no-store, timeout e a base informada", async () => {
    const fetchFn = vi.fn(async () => new Response("{}", { status: 200 }));
    const c = criarApiCliente({ baseUrl: "http://api:8080", timeoutMs: 1234, tokens, fetchFn });
    await c.status(ID);
    const [url, init] = fetchFn.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe(`http://api:8080/v1/viagens/${ID}`);
    expect(init.cache).toBe("no-store");
    expect(init.signal).toBeInstanceOf(AbortSignal);
    expect(init.headers).toHaveProperty("Authorization", `Bearer ${TOKEN}`);
  });
});

describe("Bearer e erros de credencial", () => {
  it("toda chamada leva o Bearer do gerenciador, em GET e POST", async () => {
    proxima = { status: 202, corpo: JSON.stringify({ viagem_id: ID, status: "pendente" }) };
    await cliente().criar({});
    await cliente().status(ID);
    await cliente().analise(ID);
    expect(chamadas.map((c) => c.auth)).toEqual([
      `Bearer ${TOKEN}`,
      `Bearer ${TOKEN}`,
      `Bearer ${TOKEN}`,
    ]);
  });

  it("o token e o segredo não vão ao log nem à resposta de erro", async () => {
    proxima = { status: 500, corpo: "{}" };
    const r = await cliente().status(ID);
    expect(JSON.stringify(r)).not.toContain(TOKEN);
    expect(JSON.stringify(log.mock.calls)).not.toContain(TOKEN);
  });

  it("401: invalida, renova e repete uma vez com o token novo", async () => {
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
        ? new Response(problema(401, "token_expirado"), {
            status: 401,
            headers: { "Content-Type": "application/problem+json" },
          })
        : new Response(JSON.stringify({ ok: auth }), { status: 200 });
    });
    const c = criarApiCliente({ baseUrl: base, timeoutMs: 1000, tokens, fetchFn, log });
    const r = await c.status(ID);
    expect(r).toEqual({ status: 200, corpo: { ok: "Bearer t2" } });
    expect(tokens.invalidate).toHaveBeenCalledExactlyOnceWith("t1");
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it("segundo 401 vira 502 credencial_invalida, sem terceira tentativa", async () => {
    proxima = {
      status: 401,
      tipo: "application/problem+json",
      corpo: problema(401, "token_invalido"),
    };
    const r = await cliente().criar({ a: 1 });
    expect(r.status).toBe(502);
    expect(r.corpo).toMatchObject({ codigo: "credencial_invalida" });
    expect(chamadas).toHaveLength(2);
    expect(tokens.getToken).toHaveBeenCalledTimes(2);
  });

  it("403 vira 502 credencial_invalida, sem repetir", async () => {
    proxima = { status: 403, tipo: "application/problem+json", corpo: problema(403, "proibido") };
    const r = await cliente().status(ID);
    expect(r.status).toBe(502);
    expect(r.corpo).toMatchObject({ codigo: "credencial_invalida" });
    expect(chamadas).toHaveLength(1);
  });

  it("falha do gerenciador é devolvida sem chamar a API de viagens", async () => {
    tokens = {
      getToken: vi.fn(async () => ({
        ok: false as const,
        falha: {
          status: 429,
          corpo: { type: "x", title: "bloqueado", status: 429, codigo: "bloqueado" },
          retryAfter: 42,
        },
      })),
      invalidate: vi.fn(),
    };
    const r = await cliente().status(ID);
    expect(r).toMatchObject({ status: 429, retryAfter: 42, corpo: { codigo: "bloqueado" } });
    expect(chamadas).toHaveLength(0);
  });

  it("timeout depois do token não repete o pedido", async () => {
    proxima = { status: 200, corpo: "{}", atrasoMs: 300 };
    const r = await cliente(40).criar({});
    expect(r.status).toBe(503);
    expect(chamadas).toHaveLength(1);
  });
});
