import "server-only";
import { INITIAL_DONGLE } from "@/config";
import type {
  TripAnalysis,
  CreateTripResponse,
  ApiResponse,
  TripStatusResponse,
  ListQuery,
  TripListResponse,
} from "@/domain/trip/types";
import { createApiDatasource } from "./apiDatasource";
import { createMockDatasource } from "./mock/mockDatasource";
import { createTokenManager } from "./token";

/**
 * The single gateway to trip data. Returns the HTTP status and the body the API
 * would return (`application/problem+json` on errors).
 */
export interface TripsDatasource {
  /** `POST /v1/viagens`. `body` arrives unvalidated; the datasource validates it. */
  create(body: unknown): Promise<ApiResponse<CreateTripResponse>>;
  /** `GET /v1/viagens/{id}`. */
  status(id: string): Promise<ApiResponse<TripStatusResponse>>;
  /** `GET /v1/viagens/{id}/analise`. */
  analysis(id: string): Promise<ApiResponse<TripAnalysis>>;
  /** `GET /v1/viagens`. `query` arrives already validated by the route. */
  list(query: ListQuery): Promise<ApiResponse<TripListResponse>>;
}

export type DataSource = "mock" | "api";

type Env = Record<string, string | undefined>;

export type DataConfig =
  | { source: "mock" }
  | {
      source: "api";
      url: string;
      timeoutMs: number;
      dongleId: number;
      clientId: string;
      clientSecret: string;
    };

const DEFAULT_TIMEOUT_MS = 10_000;
const DEFAULT_DEV_URL = "http://127.0.0.1:8080";

function positiveInteger(raw: string | undefined, name: string): number | undefined {
  const v = raw?.trim();
  if (!v) return undefined;
  const n = Number(v);
  if (!Number.isInteger(n) || n < 1) throw new Error(`${name} must be a positive integer`);
  return n;
}

/**
 * Reads the data configuration at runtime. In production `ANALISE_FONTE_DADOS`
 * is required, so the booth never shows sample data because of missing
 * configuration. With `api`, the URL (default only in development) and the
 * dongle are required, as are `ANALISE_CLIENT_ID` and `ANALISE_CLIENT_SECRET`
 * (server only, never `NEXT_PUBLIC_`). The secret is not logged and does not
 * enter the cache key.
 */
export function readConfig(env: Env = process.env): DataConfig {
  const value = env.ANALISE_FONTE_DADOS?.trim();
  if (!value) {
    if (env.NODE_ENV === "production") {
      throw new Error("ANALISE_FONTE_DADOS is required in production (use mock or api)");
    }
    return { source: "mock" };
  }
  if (value === "mock") return { source: "mock" };
  if (value !== "api") throw new Error(`unknown ANALISE_FONTE_DADOS: ${value}`);

  let rawUrl = env.ANALISE_API_URL?.trim();
  if (!rawUrl) {
    if (env.NODE_ENV === "production") {
      throw new Error("ANALISE_API_URL is required with ANALISE_FONTE_DADOS=api");
    }
    rawUrl = DEFAULT_DEV_URL;
  }
  let url: URL;
  try {
    url = new URL(rawUrl);
  } catch {
    throw new Error("ANALISE_API_URL is not a valid URL");
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("ANALISE_API_URL must use http or https");
  }
  const dongleId = positiveInteger(env.ANALISE_DONGLE_ID, "ANALISE_DONGLE_ID");
  if (dongleId === undefined) {
    throw new Error("ANALISE_DONGLE_ID is required with ANALISE_FONTE_DADOS=api");
  }
  const timeoutMs = positiveInteger(env.ANALISE_TIMEOUT_MS, "ANALISE_TIMEOUT_MS");
  const clientId = env.ANALISE_CLIENT_ID?.trim();
  if (!clientId) throw new Error("ANALISE_CLIENT_ID is required with ANALISE_FONTE_DADOS=api");
  const clientSecret = env.ANALISE_CLIENT_SECRET?.trim();
  if (!clientSecret) {
    throw new Error("ANALISE_CLIENT_SECRET is required with ANALISE_FONTE_DADOS=api");
  }
  return {
    source: "api",
    url: url.toString().replace(/\/+$/, ""),
    timeoutMs: timeoutMs ?? DEFAULT_TIMEOUT_MS,
    dongleId,
    clientId,
    clientSecret,
  };
}

/** Decides the data source and fails if the configuration is incomplete. */
export function chooseSource(env: Env = process.env): DataSource {
  return readConfig(env).source;
}

/** Initial value of the dongle field on the process screen. */
export function initialDongle(env: Env = process.env): number {
  const c = readConfig(env);
  return c.source === "api" ? c.dongleId : INITIAL_DONGLE;
}

const KEY = "__tccelta_datasource__";
type Stored = { key: string; datasource: TripsDatasource };

export function getDatasource(env: Env = process.env): TripsDatasource {
  const c = readConfig(env);
  const key =
    c.source === "mock"
      ? "mock"
      : `api|${c.url}|${c.timeoutMs}|${c.clientId}|${c.clientSecret.length}`;
  const g = globalThis as unknown as Record<string, Stored | undefined>;
  const current = g[KEY];
  if (current?.key === key) return current.datasource;
  const datasource =
    c.source === "mock"
      ? createMockDatasource()
      : createApiDatasource({
          baseUrl: c.url,
          timeoutMs: c.timeoutMs,
          tokens: createTokenManager({
            baseUrl: c.url,
            clientId: c.clientId,
            clientSecret: c.clientSecret,
          }),
        });
  g[KEY] = { key, datasource };
  return datasource;
}
