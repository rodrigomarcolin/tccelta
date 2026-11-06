import "server-only";
import type { Problem } from "@/domain/trip/types";
import { buildProblem } from "@/domain/trip/problem";

/**
 * Token manager for client credentials (INT2). Keeps the token in memory,
 * renews it before expiry, merges simultaneous renewals into one and honors
 * the quarantine after `401 invalid_client` or `429`: in those cases it does
 * not call the API again until the deadline. The secret and the token never
 * leave this file, not even in logs.
 */

export type TokenFailure = { status: number; body: Problem; retryAfter?: number };
export type TokenResult = { ok: true; token: string } | { ok: false; failure: TokenFailure };

export interface TokenManager {
  /** Valid token (cached or freshly obtained), or the failure already translated into a problem. */
  getToken(): Promise<TokenResult>;
  /**
   * Discards the token the API rejected. With `rejected`, it only discards it if it is still
   * the cached one, so several simultaneous `401`s cause a single renewal.
   */
  invalidate(rejected?: string): void;
}

export type TokenOptions = {
  /** API base, without trailing slash. */
  baseUrl: string;
  clientId: string;
  clientSecret: string;
  /** Default: 5 s. */
  timeoutMs?: number;
  fetchFn?: typeof fetch;
  /** Clock in ms (tests). */
  now?: () => number;
  log?: (level: "error" | "warn", message: string) => void;
};

const DEFAULT_TIMEOUT_MS = 5_000;
const QUARANTINE_MS = 60_000;
const MIN_RETRY_INTERVAL_MS = 1_000;
const SLACK_MS = 60_000;
const SHORT_VALIDITY_MS = 5 * 60_000;

function defaultLog(level: "error" | "warn", message: string) {
  (level === "error" ? console.error : console.warn)(`[token] ${message}`);
}

/** Renews when less than 60 s remain, or 20% of the validity if that is under 5 min. */
export function slackFor(validityMs: number): number {
  return validityMs < SHORT_VALIDITY_MS ? validityMs * 0.2 : SLACK_MS;
}

function remainingSeconds(until: number, now: number): number {
  return Math.max(1, Math.ceil((until - now) / 1000));
}

function parseRetryAfter(header: string | null): number | undefined {
  if (!header) return undefined;
  const s = Number(header.trim());
  return Number.isFinite(s) && s > 0 ? Math.ceil(s) : undefined;
}

export function createTokenManager(opts: TokenOptions): TokenManager {
  const fetchFn = opts.fetchFn ?? fetch;
  const now = opts.now ?? Date.now;
  const log = opts.log ?? defaultLog;
  const timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const basic = Buffer.from(`${opts.clientId}:${opts.clientSecret}`, "utf8").toString("base64");

  let token: string | null = null;
  let renewAt = 0;
  let pending: Promise<TokenResult> | null = null;
  let invalidCredentialUntil = 0;
  let blockedUntil = 0;
  let lastNetworkFailure = -Infinity;

  const failed = (
    status: number,
    codigo: string,
    detail: string,
    retryAfter?: number,
  ): TokenResult => ({
    ok: false,
    failure: { status, body: buildProblem(status, codigo, detail), retryAfter },
  });

  const invalidCredential = (retryAfter?: number) =>
    failed(
      502,
      "credencial_invalida",
      "o servidor não aceitou a credencial do sistema",
      retryAfter,
    );
  const blocked = (retryAfter: number) =>
    failed(429, "bloqueado", "acesso temporariamente bloqueado", retryAfter);
  const unavailable = () =>
    failed(503, "api_indisponivel", "não foi possível falar com o servidor");

  async function renew(): Promise<TokenResult> {
    let resp: Response;
    let text: string;
    try {
      resp = await fetchFn(`${opts.baseUrl}/v1/auth/token`, {
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
      text = await resp.text();
    } catch (error) {
      lastNetworkFailure = now();
      log(
        "error",
        `POST /v1/auth/token: ${error instanceof Error ? error.name : "network failure"}`,
      );
      return unavailable();
    }

    const t = now();
    if (resp.status === 200) {
      let json: unknown;
      try {
        json = JSON.parse(text);
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
        lastNetworkFailure = t;
        log("error", "POST /v1/auth/token: 200 response outside the contract");
        return failed(502, "resposta_invalida", "resposta de autenticação inválida");
      }
      const validity = o.expires_in * 1000;
      token = o.access_token;
      renewAt = t + validity - slackFor(validity);
      return { ok: true, token };
    }

    if (resp.status === 401) {
      invalidCredentialUntil = t + QUARANTINE_MS;
      log(
        "error",
        `client_id=${opts.clientId} rejected (status 401); verifique ANALISE_CLIENT_SECRET. Sem nova tentativa por ${QUARANTINE_MS / 1000} s`,
      );
      return invalidCredential(QUARANTINE_MS / 1000);
    }
    if (resp.status === 429) {
      const ra = parseRetryAfter(resp.headers.get("Retry-After"));
      const s = ra ?? QUARANTINE_MS / 1000;
      blockedUntil = t + s * 1000;
      log(
        "warn",
        `client_id=${opts.clientId} blocked (429), Retry-After=${s} s${ra === undefined ? " (assumed, the header was missing)" : ""}; likely cause: wrong secret`,
      );
      return blocked(s);
    }

    lastNetworkFailure = t;
    log("error", `POST /v1/auth/token: status ${resp.status}`);
    return unavailable();
  }

  return {
    async getToken() {
      const t = now();
      if (token !== null && t < renewAt) return { ok: true, token };
      if (pending) return pending;
      if (t < invalidCredentialUntil)
        return invalidCredential(remainingSeconds(invalidCredentialUntil, t));
      if (t < blockedUntil) return blocked(remainingSeconds(blockedUntil, t));
      if (t - lastNetworkFailure < MIN_RETRY_INTERVAL_MS) return unavailable();
      const p = renew().finally(() => {
        if (pending === p) pending = null;
      });
      pending = p;
      return p;
    },
    invalidate(rejected) {
      if (rejected === undefined || rejected === token) {
        token = null;
        renewAt = 0;
      }
    },
  };
}
