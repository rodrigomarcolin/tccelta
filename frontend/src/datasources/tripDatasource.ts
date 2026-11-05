import { BASE_PATH } from "@/config";
import { ApiError } from "@/domain/trip/apiError";
import type {
  CreateTripRequest,
  CreateTripResponse,
  Problem,
  TripAnalysis,
  TripStatusResponse,
} from "@/domain/trip/types";

/**
 * The only browser code that calls `fetch`: the BFF routes under `/api/trips`.
 * Success returns the domain types; anything else throws `ApiError`. Network failures
 * (and aborts) propagate as they are.
 */

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/** The body as a problem, when it carries a `codigo`. */
function asProblem(body: unknown): Problem | null {
  return isObject(body) && typeof body.codigo === "string" ? (body as Problem) : null;
}

function parseRetryAfter(header: string | null | undefined): number | undefined {
  const seconds = Number(header);
  return Number.isFinite(seconds) && seconds > 0 ? seconds : undefined;
}

async function request(
  url: string,
  init: RequestInit,
  expected: number,
  valid: (body: unknown) => boolean,
): Promise<unknown> {
  const response = await fetch(`${BASE_PATH}${url}`, { cache: "no-store", ...init });
  const body: unknown = await response.json().catch(() => null);
  const retryAfter = parseRetryAfter(response.headers?.get("Retry-After"));
  if (response.status !== expected || !valid(body)) {
    throw new ApiError(response.status, asProblem(body), retryAfter);
  }
  return body;
}

export async function createTrip(body: CreateTripRequest): Promise<CreateTripResponse> {
  return (await request(
    "/api/trips",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
    202,
    (b) => isObject(b) && typeof b.viagem_id === "string",
  )) as CreateTripResponse;
}

export async function getTripStatus(id: string, signal?: AbortSignal): Promise<TripStatusResponse> {
  return (await request(
    `/api/trips/${encodeURIComponent(id)}`,
    { signal },
    200,
    (b) =>
      isObject(b) &&
      typeof b.status === "string" &&
      isObject(b.motorista) &&
      typeof b.motorista.nome === "string",
  )) as TripStatusResponse;
}

export async function getTripAnalysis(id: string, signal?: AbortSignal): Promise<TripAnalysis> {
  return (await request(
    `/api/trips/${encodeURIComponent(id)}/analysis`,
    { signal },
    200,
    isObject,
  )) as TripAnalysis;
}
