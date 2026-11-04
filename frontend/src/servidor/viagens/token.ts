import "server-only";
import type { Problema } from "@/contrato/tipos";
import { problema } from "@/contrato/problema";

/**
 * Gerenciador do token de client credentials (INT2). Guarda o token em memória,
 * renova antes do vencimento, junta renovações simultâneas numa só e respeita
 * a quarentena depois de `401 invalid_client` ou `429`: nesses casos não vai à
 * API de novo até o prazo. Segredo e token nunca saem daqui, nem em log.
 */

export type FalhaToken = { status: number; corpo: Problema; retryAfter?: number };
export type ResultadoToken = { ok: true; token: string } | { ok: false; falha: FalhaToken };

export interface GerenciadorToken {
  /** Token válido (do cache ou recém-obtido), ou a falha já traduzida em problema. */
  getToken(): Promise<ResultadoToken>;
  /**
   * Descarta o token que a API recusou. Com `recusado`, só descarta se ele ainda
   * for o do cache, para que vários `401` simultâneos gerem uma só renovação.
   */
  invalidate(recusado?: string): void;
}

export type OpcoesToken = {
  /** Base da API, sem barra final. */
  baseUrl: string;
  clientId: string;
  clientSecret: string;
  /** Padrão: 5 s. */
  timeoutMs?: number;
  fetchFn?: typeof fetch;
  /** Relógio em ms (testes). */
  agora?: () => number;
  log?: (nivel: "error" | "warn", mensagem: string) => void;
};

const TIMEOUT_PADRAO_MS = 5_000;
const QUARENTENA_MS = 60_000;
const INTERVALO_MIN_TENTATIVAS_MS = 1_000;
const FOLGA_MS = 60_000;
const VALIDADE_CURTA_MS = 5 * 60_000;

function logPadrao(nivel: "error" | "warn", mensagem: string) {
  (nivel === "error" ? console.error : console.warn)(`[token] ${mensagem}`);
}

/** Renova quando faltam menos de 60 s, ou 20% da validade se esta for menor que 5 min. */
export function folgaDe(validadeMs: number): number {
  return validadeMs < VALIDADE_CURTA_MS ? validadeMs * 0.2 : FOLGA_MS;
}

function segundosRestantes(ate: number, agora: number): number {
  return Math.max(1, Math.ceil((ate - agora) / 1000));
}

function lerRetryAfter(cabecalho: string | null): number | undefined {
  if (!cabecalho) return undefined;
  const s = Number(cabecalho.trim());
  return Number.isFinite(s) && s > 0 ? Math.ceil(s) : undefined;
}

export function criarGerenciadorToken(op: OpcoesToken): GerenciadorToken {
  const fetchFn = op.fetchFn ?? fetch;
  const agora = op.agora ?? Date.now;
  const log = op.log ?? logPadrao;
  const timeoutMs = op.timeoutMs ?? TIMEOUT_PADRAO_MS;
  const basic = Buffer.from(`${op.clientId}:${op.clientSecret}`, "utf8").toString("base64");

  let token: string | null = null;
  let renovarEm = 0;
  let pendente: Promise<ResultadoToken> | null = null;
  let credencialInvalidaAte = 0;
  let bloqueadoAte = 0;
  let ultimaFalhaRede = -Infinity;

  const falha = (
    status: number,
    codigo: string,
    detail: string,
    retryAfter?: number,
  ): ResultadoToken => ({
    ok: false,
    falha: { status, corpo: problema(status, codigo, detail), retryAfter },
  });

  const credencialInvalida = (retryAfter?: number) =>
    falha(502, "credencial_invalida", "o servidor não aceitou a credencial do sistema", retryAfter);
  const bloqueado = (retryAfter: number) =>
    falha(429, "bloqueado", "acesso temporariamente bloqueado", retryAfter);
  const indisponivel = () =>
    falha(503, "api_indisponivel", "não foi possível falar com o servidor");

  async function renovar(): Promise<ResultadoToken> {
    let resp: Response;
    let texto: string;
    try {
      resp = await fetchFn(`${op.baseUrl}/v1/auth/token`, {
        method: "POST",
        cache: "no-store",
        redirect: "error",
        headers: {
          Authorization: `Basic ${basic}`,
          "Content-Type": "application/x-www-form-urlencoded",
          Accept: "application/json",
        },
        body: "grant_type=client_credentials",
        signal: AbortSignal.timeout(timeoutMs),
      });
      texto = await resp.text();
    } catch (erro) {
      ultimaFalhaRede = agora();
      log("error", `POST /v1/auth/token: ${erro instanceof Error ? erro.name : "falha de rede"}`);
      return indisponivel();
    }

    const t = agora();
    if (resp.status === 200) {
      let json: unknown;
      try {
        json = JSON.parse(texto);
      } catch {
        json = null;
      }
      const o = (json ?? {}) as { access_token?: unknown; expires_in?: unknown };
      if (
        typeof o.access_token !== "string" ||
        o.access_token === "" ||
        typeof o.expires_in !== "number" ||
        !(o.expires_in > 0)
      ) {
        ultimaFalhaRede = t;
        log("error", "POST /v1/auth/token: resposta 200 fora do contrato");
        return falha(502, "resposta_invalida", "resposta de autenticação inválida");
      }
      const validade = o.expires_in * 1000;
      token = o.access_token;
      renovarEm = t + validade - folgaDe(validade);
      return { ok: true, token };
    }

    if (resp.status === 401) {
      credencialInvalidaAte = t + QUARENTENA_MS;
      log(
        "error",
        `client_id=${op.clientId} recusado (status 401); verifique ANALISE_CLIENT_SECRET. Sem nova tentativa por ${QUARENTENA_MS / 1000} s`,
      );
      return credencialInvalida(QUARENTENA_MS / 1000);
    }
    if (resp.status === 429) {
      const ra = lerRetryAfter(resp.headers.get("Retry-After"));
      const s = ra ?? QUARENTENA_MS / 1000;
      bloqueadoAte = t + s * 1000;
      log(
        "warn",
        `client_id=${op.clientId} bloqueado (429), Retry-After=${s} s${ra === undefined ? " (assumido, o cabeçalho faltou)" : ""}; causa provável: segredo errado`,
      );
      return bloqueado(s);
    }

    ultimaFalhaRede = t;
    log("error", `POST /v1/auth/token: status ${resp.status}`);
    return indisponivel();
  }

  return {
    async getToken() {
      const t = agora();
      if (token !== null && t < renovarEm) return { ok: true, token };
      if (pendente) return pendente;
      if (t < credencialInvalidaAte)
        return credencialInvalida(segundosRestantes(credencialInvalidaAte, t));
      if (t < bloqueadoAte) return bloqueado(segundosRestantes(bloqueadoAte, t));
      if (t - ultimaFalhaRede < INTERVALO_MIN_TENTATIVAS_MS) return indisponivel();
      const p = renovar().finally(() => {
        if (pendente === p) pendente = null;
      });
      pendente = p;
      return p;
    },
    invalidate(recusado) {
      if (recusado === undefined || recusado === token) {
        token = null;
        renovarEm = 0;
      }
    },
  };
}
