import "server-only";
import type { Analise, CriarViagemResposta, Resposta, StatusResposta } from "@/contrato/tipos";
import { ehProblema } from "@/contrato/tipos";
import { problema } from "@/contrato/problema";
import type { ViagensCliente } from "./tipos";
import type { GerenciadorToken } from "./token";

export type OpcoesApi = {
  /** Base da API, sem barra final. */
  baseUrl: string;
  timeoutMs: number;
  /** Token de client credentials; toda chamada leva `Authorization: Bearer`. */
  tokens: GerenciadorToken;
  /** Troca o `fetch` (testes). */
  fetchFn?: typeof fetch;
  /** Registro de detalhes técnicos; nunca vai ao navegador. */
  log?: (mensagem: string) => void;
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function lerRetryAfter(cabecalho: string | null): number | undefined {
  if (!cabecalho) return undefined;
  const s = Number(cabecalho.trim());
  return Number.isFinite(s) && s >= 0 ? Math.ceil(s) : undefined;
}

type Bruta = { resp: Response; texto: string } | null;

/**
 * Implementação real de `ViagensCliente`: fala com `/v1/viagens` da API, com
 * Bearer, e devolve o JSON sem reescrever campos. Falha de rede ou timeout vira
 * 503 `api_indisponivel`; resposta fora do contrato vira 502 `resposta_invalida`;
 * `401`/`403` da API viram 502 `credencial_invalida` (o `401` da tela é a sessão
 * do operador, e repassá-lo levaria a um laço de login).
 */
export function criarApiCliente(op: OpcoesApi): ViagensCliente {
  const fetchFn = op.fetchFn ?? fetch;
  const log = op.log ?? ((m: string) => console.error(`[viagens] ${m}`));

  async function pedir(
    metodo: "GET" | "POST",
    caminho: string,
    token: string,
    corpo?: unknown,
  ): Promise<Bruta> {
    try {
      const resp = await fetchFn(`${op.baseUrl}${caminho}`, {
        method: metodo,
        cache: "no-store",
        redirect: "error",
        headers: {
          Accept: "application/json, application/problem+json",
          Authorization: `Bearer ${token}`,
          ...(corpo === undefined ? {} : { "Content-Type": "application/json" }),
        },
        body: corpo === undefined ? undefined : JSON.stringify(corpo),
        signal: AbortSignal.timeout(op.timeoutMs),
      });
      return { resp, texto: await resp.text() };
    } catch (erro) {
      log(`${metodo} ${caminho}: ${erro instanceof Error ? erro.name : "falha de rede"}`);
      return null;
    }
  }

  const recusada = (): Resposta<never> => ({
    status: 502,
    corpo: problema(502, "credencial_invalida", "o servidor não aceitou a credencial do sistema"),
  });

  async function chamar<T>(
    metodo: "GET" | "POST",
    caminho: string,
    corpo?: unknown,
  ): Promise<Resposta<T>> {
    let bruta: Bruta = null;
    // No máximo duas tentativas: a segunda só após um 401 (token vencido ou
    // trocado), com token renovado. A API barra o Bearer antes de tratar a rota,
    // então repetir o pedido recusado é seguro. Timeout e 5xx nunca repetem.
    for (let tentativa = 0; tentativa < 2; tentativa++) {
      const t = await op.tokens.getToken();
      if (!t.ok) {
        return { status: t.falha.status, corpo: t.falha.corpo, retryAfter: t.falha.retryAfter };
      }
      bruta = await pedir(metodo, caminho, t.token, corpo);
      if (bruta?.resp.status !== 401) break;
      op.tokens.invalidate(t.token);
      if (tentativa === 1) {
        log(`${metodo} ${caminho}: 401 mesmo com token novo`);
        return recusada();
      }
    }
    if (bruta === null) {
      return {
        status: 503,
        corpo: problema(503, "api_indisponivel", "não foi possível falar com o servidor"),
      };
    }
    const { resp, texto } = bruta;
    if (resp.status === 403) {
      log(`${metodo} ${caminho}: 403`);
      return recusada();
    }

    const retryAfter = lerRetryAfter(resp.headers.get("Retry-After"));
    let json: unknown;
    try {
      json = JSON.parse(texto);
    } catch {
      log(`${metodo} ${caminho}: corpo não é JSON (status ${resp.status})`);
      return { status: 502, corpo: problema(502, "resposta_invalida"), retryAfter };
    }

    if (resp.status >= 200 && resp.status < 300) {
      return { status: resp.status, corpo: json as T };
    }
    if (ehProblema(json)) {
      return { status: resp.status, corpo: json, retryAfter };
    }
    log(`${metodo} ${caminho}: erro ${resp.status} sem codigo`);
    return { status: 502, corpo: problema(502, "resposta_invalida"), retryAfter };
  }

  const desconhecida = <T>(): Resposta<T> => ({
    status: 404,
    corpo: problema(404, "viagem_desconhecida", "viagem desconhecida"),
  });

  return {
    criar: (corpo: unknown) => chamar<CriarViagemResposta>("POST", "/v1/viagens", corpo),
    async status(id: string) {
      if (!UUID.test(id)) return desconhecida<StatusResposta>();
      return chamar<StatusResposta>("GET", `/v1/viagens/${id.toLowerCase()}`);
    },
    async analise(id: string) {
      if (!UUID.test(id)) return desconhecida<Analise>();
      return chamar<Analise>("GET", `/v1/viagens/${id.toLowerCase()}/analise`);
    },
  };
}
