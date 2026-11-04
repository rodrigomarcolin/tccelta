import type { Problema } from "./tipos";

/** Monta um `application/problem+json` no formato de `backend/internal/http/problem.go`. */
export function problema(
  status: number,
  codigo: string,
  detail?: string,
  extra?: Partial<Problema>,
): Problema {
  return {
    type: `https://tccelta.local/erros/${codigo}`,
    title: codigo,
    status,
    codigo,
    ...(detail ? { detail } : {}),
    ...extra,
  };
}
