import type { Problem } from "./types";

/** Builds an `application/problem+json` body in the format of `backend/internal/http/problem.go`. */
export function buildProblem(
  status: number,
  codigo: string,
  detail?: string,
  extra?: Partial<Problem>,
): Problem {
  return {
    type: `https://tccelta.local/erros/${codigo}`,
    title: codigo,
    status,
    codigo,
    ...(detail ? { detail } : {}),
    ...extra,
  };
}
