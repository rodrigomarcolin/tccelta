import type { Problem } from "./types";

/**
 * Failure of a call to the BFF routes: any status other than the expected one, or a success
 * whose body is off-contract. Network failures are not wrapped and propagate as they are.
 */
export class ApiError extends Error {
  /** HTTP status of the response (200 or 202 when the body is off-contract). */
  readonly status: number;
  /** The `application/problem+json` body, when the response carried one. */
  readonly problem: Problem | null;
  /** Seconds from the `Retry-After` header, when sent and positive. */
  readonly retryAfter: number | undefined;

  constructor(status: number, problem: Problem | null, retryAfter?: number) {
    super(problem?.codigo ?? `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
    this.retryAfter = retryAfter;
  }
}
