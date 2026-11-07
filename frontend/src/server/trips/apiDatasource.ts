import "server-only";
import type {
  TripAnalysis,
  CreateTripResponse,
  ApiResponse,
  TripStatusResponse,
  ListQuery,
  TripListResponse,
} from "@/domain/trip/types";
import { isProblem } from "@/domain/trip/types";
import { buildProblem } from "@/domain/trip/problem";
import type { TripsDatasource } from "./datasource";
import type { TokenManager } from "./token";

export type ApiOptions = {
  /** API base, without trailing slash. */
  baseUrl: string;
  timeoutMs: number;
  /** Client credentials token; every call carries `Authorization: Bearer`. */
  tokens: TokenManager;
  /** Replaces `fetch` (tests). */
  fetchFn?: typeof fetch;
  /** Technical-detail log; never goes to the browser. */
  log?: (message: string) => void;
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function parseRetryAfter(header: string | null): number | undefined {
  if (!header) return undefined;
  const s = Number(header.trim());
  return Number.isFinite(s) && s >= 0 ? Math.ceil(s) : undefined;
}

type RawResponse = { resp: Response; text: string } | null;

/**
 * Real implementation of `TripsDatasource`: talks to the API `/v1/viagens`, with
 * Bearer, and returns the JSON without rewriting fields. A network failure or timeout becomes
 * 503 `api_indisponivel`; a response outside the contract becomes 502 `resposta_invalida`;
 * the API `401`/`403` become 502 `credencial_invalida` (the screen `401` is the operator
 * session, and forwarding it would cause a login loop).
 */
export function createApiDatasource(opts: ApiOptions): TripsDatasource {
  const fetchFn = opts.fetchFn ?? fetch;
  const log = opts.log ?? ((m: string) => console.error(`[trips] ${m}`));

  async function request(
    method: "GET" | "POST" | "DELETE",
    path: string,
    token: string,
    body?: unknown,
  ): Promise<RawResponse> {
    try {
      const resp = await fetchFn(`${opts.baseUrl}${path}`, {
        method: method,
        cache: "no-store",
        redirect: "error",
        headers: {
          Accept: "application/json, application/problem+json",
          Authorization: `Bearer ${token}`,
          ...(body === undefined ? {} : { "Content-Type": "application/json" }),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: AbortSignal.timeout(opts.timeoutMs),
      });
      return { resp, text: await resp.text() };
    } catch (error) {
      log(`${method} ${path}: ${error instanceof Error ? error.name : "network failure"}`);
      return null;
    }
  }

  const rejected = (): ApiResponse<never> => ({
    status: 502,
    body: buildProblem(
      502,
      "credencial_invalida",
      "o servidor não aceitou a credencial do sistema",
    ),
  });

  async function call<T>(
    method: "GET" | "POST" | "DELETE",
    path: string,
    body?: unknown,
  ): Promise<ApiResponse<T>> {
    let raw: RawResponse = null;
    // At most two attempts: the second only after a 401 (expired or replaced
    // token), with a renewed token. The API rejects the Bearer before handling the route,
    // so repeating the rejected request is safe. Timeouts and 5xx never retry.
    for (let attempt = 0; attempt < 2; attempt++) {
      const t = await opts.tokens.getToken();
      if (!t.ok) {
        return { status: t.failure.status, body: t.failure.body, retryAfter: t.failure.retryAfter };
      }
      raw = await request(method, path, t.token, body);
      if (raw?.resp.status !== 401) break;
      opts.tokens.invalidate(t.token);
      if (attempt === 1) {
        log(`${method} ${path}: 401 even with a new token`);
        return rejected();
      }
    }
    if (raw === null) {
      return {
        status: 503,
        body: buildProblem(503, "api_indisponivel", "não foi possível falar com o servidor"),
      };
    }
    const { resp, text } = raw;
    if (resp.status === 403) {
      log(`${method} ${path}: 403`);
      return rejected();
    }

    const retryAfter = parseRetryAfter(resp.headers.get("Retry-After"));
    // 204 No Content (DELETE) has no body to parse.
    if (resp.status === 204) return { status: 204, body: null as T };
    let json: unknown;
    try {
      json = JSON.parse(text);
    } catch {
      log(`${method} ${path}: body is not JSON (status ${resp.status})`);
      return { status: 502, body: buildProblem(502, "resposta_invalida"), retryAfter };
    }

    if (resp.status >= 200 && resp.status < 300) {
      return { status: resp.status, body: json as T };
    }
    if (isProblem(json)) {
      return { status: resp.status, body: json, retryAfter };
    }
    log(`${method} ${path}: error ${resp.status} without codigo`);
    return { status: 502, body: buildProblem(502, "resposta_invalida"), retryAfter };
  }

  const unknown = <T>(): ApiResponse<T> => ({
    status: 404,
    body: buildProblem(404, "viagem_desconhecida", "viagem unknown"),
  });

  return {
    create: (body: unknown) => call<CreateTripResponse>("POST", "/v1/viagens", body),
    async status(id: string) {
      if (!UUID.test(id)) return unknown<TripStatusResponse>();
      return call<TripStatusResponse>("GET", `/v1/viagens/${id.toLowerCase()}`);
    },
    async analysis(id: string) {
      if (!UUID.test(id)) return unknown<TripAnalysis>();
      return call<TripAnalysis>("GET", `/v1/viagens/${id.toLowerCase()}/analise`);
    },
    async remove(id: string) {
      if (!UUID.test(id)) return unknown<null>();
      return call<null>("DELETE", `/v1/viagens/${id.toLowerCase()}`);
    },
    async list(query: ListQuery) {
      const params = new URLSearchParams();
      if (query.limite !== undefined) params.set("limite", String(query.limite));
      if (query.cursor) params.set("cursor", query.cursor);
      if (query.busca) params.set("busca", query.busca);
      const qs = params.toString();
      return call<TripListResponse>("GET", `/v1/viagens${qs ? `?${qs}` : ""}`);
    },
  };
}
