import { POLL_BACKOFF_MAX_MS, POLL_WAITING_MS } from "@/config";
import { ApiError } from "@/domain/trip/apiError";

/** Seconds waited when the server holds attempts (bad system credential or blocked access). */
const HELD_SECONDS = 60;

export const apiCode = (error: unknown): string | undefined =>
  error instanceof ApiError ? error.problem?.codigo : undefined;

export const isHeld = (error: unknown): boolean => {
  const code = apiCode(error);
  return code === "credencial_invalida" || code === "bloqueado";
};

export const isAbort = (error: unknown): boolean =>
  error instanceof Error && error.name === "AbortError";

/** Network failure, 5xx, 429 or a held attempt: worth trying again by itself. */
export function isTransient(error: unknown): boolean {
  if (isAbort(error)) return false;
  if (!(error instanceof ApiError)) return true;
  return error.status >= 500 || error.status === 429 || isHeld(error);
}

/**
 * Retry decision of the polled queries. With a result already on screen, an unexpected
 * response is handled as a failure instead of replacing the screen with an error.
 */
export function shouldRetry(error: unknown, hasResult: boolean): boolean {
  if (isTransient(error)) return true;
  return hasResult && error instanceof ApiError && error.status !== 401 && error.status !== 404;
}

/**
 * Delay before the next attempt of a streak. `failureCount` is 0 for the first failure:
 * 1, 2, 4 and then at most 5 s, unless the server said when (`Retry-After`).
 */
export function retryDelay(failureCount: number, error: unknown): number {
  if (error instanceof ApiError) {
    if (isHeld(error)) return Math.max(1, error.retryAfter ?? HELD_SECONDS) * 1000;
    if (error.status === 429) return Math.max(1, error.retryAfter ?? 1) * 1000;
    if (error.status === 409) return POLL_WAITING_MS;
    if (error.retryAfter !== undefined) return Math.max(1, error.retryAfter) * 1000;
  }
  return Math.min(POLL_BACKOFF_MAX_MS, POLL_WAITING_MS * 2 ** failureCount);
}
