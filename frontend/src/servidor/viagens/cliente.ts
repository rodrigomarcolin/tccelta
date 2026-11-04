import "server-only";
import { DONGLE_INICIAL } from "@/config";
import { criarApiCliente } from "./api-cliente";
import { criarMockCliente } from "./mock/mock-cliente";
import type { ViagensCliente } from "./tipos";

export type { ViagensCliente } from "./tipos";

export type FonteDados = "mock" | "api";

type Ambiente = Record<string, string | undefined>;

export type ConfigDados =
  { fonte: "mock" } | { fonte: "api"; url: string; timeoutMs: number; dongleId: number };

const TIMEOUT_PADRAO_MS = 10_000;
const URL_PADRAO_DEV = "http://127.0.0.1:8080";

function inteiroPositivo(bruto: string | undefined, nome: string): number | undefined {
  const v = bruto?.trim();
  if (!v) return undefined;
  const n = Number(v);
  if (!Number.isInteger(n) || n < 1) throw new Error(`${nome} deve ser um inteiro positivo`);
  return n;
}

/**
 * Lê a configuração de dados, em runtime. Em produção `ANALISE_FONTE_DADOS` é
 * obrigatória, para o estande nunca mostrar dados de exemplo por falta de
 * configuração. Com `api`, a URL (padrão só em desenvolvimento) e o dongle são
 * obrigatórios. Não existe `ANALISE_SEM_AUTH` aqui: quem a liga é o backend.
 */
export function lerConfig(env: Ambiente = process.env): ConfigDados {
  const valor = env.ANALISE_FONTE_DADOS?.trim();
  if (!valor) {
    if (env.NODE_ENV === "production") {
      throw new Error("ANALISE_FONTE_DADOS é obrigatória em produção (use mock ou api)");
    }
    return { fonte: "mock" };
  }
  if (valor === "mock") return { fonte: "mock" };
  if (valor !== "api") throw new Error(`ANALISE_FONTE_DADOS desconhecida: ${valor}`);

  let bruta = env.ANALISE_API_URL?.trim();
  if (!bruta) {
    if (env.NODE_ENV === "production") {
      throw new Error("ANALISE_API_URL é obrigatória com ANALISE_FONTE_DADOS=api");
    }
    bruta = URL_PADRAO_DEV;
  }
  let url: URL;
  try {
    url = new URL(bruta);
  } catch {
    throw new Error("ANALISE_API_URL não é uma URL válida");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("ANALISE_API_URL deve usar http ou https");
  }
  const dongleId = inteiroPositivo(env.ANALISE_DONGLE_ID, "ANALISE_DONGLE_ID");
  if (dongleId === undefined) {
    throw new Error("ANALISE_DONGLE_ID é obrigatória com ANALISE_FONTE_DADOS=api");
  }
  const timeoutMs = inteiroPositivo(env.ANALISE_TIMEOUT_MS, "ANALISE_TIMEOUT_MS");
  return {
    fonte: "api",
    url: url.toString().replace(/\/+$/, ""),
    timeoutMs: timeoutMs ?? TIMEOUT_PADRAO_MS,
    dongleId,
  };
}

/** Decide a fonte de dados e falha se a configuração estiver incompleta. */
export function escolherFonte(env: Ambiente = process.env): FonteDados {
  return lerConfig(env).fonte;
}

/** Valor inicial do campo dongle da tela de processar. */
export function dongleInicial(env: Ambiente = process.env): number {
  const c = lerConfig(env);
  return c.fonte === "api" ? c.dongleId : DONGLE_INICIAL;
}

const CHAVE = "__tccelta_cliente__";
type Guardado = { chave: string; cliente: ViagensCliente };

export function obterCliente(env: Ambiente = process.env): ViagensCliente {
  const c = lerConfig(env);
  const chave = c.fonte === "mock" ? "mock" : `api|${c.url}|${c.timeoutMs}`;
  const g = globalThis as unknown as Record<string, Guardado | undefined>;
  const atual = g[CHAVE];
  if (atual?.chave === chave) return atual.cliente;
  const cliente =
    c.fonte === "mock"
      ? criarMockCliente()
      : criarApiCliente({ baseUrl: c.url, timeoutMs: c.timeoutMs });
  g[CHAVE] = { chave, cliente };
  return cliente;
}
