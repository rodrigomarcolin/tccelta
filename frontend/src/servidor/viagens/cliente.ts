import "server-only";
import { criarMockCliente } from "./mock/mock-cliente";
import type { ViagensCliente } from "./tipos";

export type { ViagensCliente } from "./tipos";

export type FonteDados = "mock";

type Ambiente = Record<string, string | undefined>;

/**
 * Decide a fonte de dados. Em produção a variável é obrigatória, para o
 * estande nunca mostrar dados de exemplo por falta de configuração.
 */
export function escolherFonte(env: Ambiente = process.env): FonteDados {
  const valor = env.ANALISE_FONTE_DADOS?.trim();
  if (!valor) {
    if (env.NODE_ENV === "production") {
      throw new Error("ANALISE_FONTE_DADOS é obrigatória em produção (use mock ou api)");
    }
    return "mock";
  }
  if (valor === "mock") return "mock";
  if (valor === "api") throw new Error("Cliente da API ainda não implementado (INT1)");
  throw new Error(`ANALISE_FONTE_DADOS desconhecida: ${valor}`);
}

const CHAVE = "__tccelta_cliente__";

export function obterCliente(env: Ambiente = process.env): ViagensCliente {
  escolherFonte(env);
  const g = globalThis as unknown as Record<string, ViagensCliente | undefined>;
  g[CHAVE] ??= criarMockCliente();
  return g[CHAVE];
}
