import { buildProblem } from "@/domain/trip/problem";
import type { ListQuery, Problem } from "@/domain/trip/types";

export const LIST_LIMIT_MAX = 50;
export const SEARCH_MAX_RUNES = 60;
const CURSOR_MAX_LENGTH = 512;

type Result = { ok: true; query: ListQuery } | { ok: false; problem: Problem };

const invalid = (detail: string): Result => ({
  ok: false,
  problem: buildProblem(400, "entrada_invalida", detail),
});

/**
 * Validates the query string of `GET /api/trips` with the rules of `GET /v1/viagens`, so that a
 * bad value never leaves the BFF. Only `limite`, `cursor` and `busca` are forwarded; anything
 * else in the URL is dropped.
 */
export function parseListQuery(params: URLSearchParams): Result {
  const query: ListQuery = {};
  for (const name of ["limite", "cursor", "busca"]) {
    if (params.getAll(name).length > 1) return invalid(`${name}: informe uma vez só`);
  }

  const limite = params.get("limite");
  if (limite !== null) {
    if (!/^\d{1,3}$/.test(limite)) return invalid(`limite: inteiro de 1 a ${LIST_LIMIT_MAX}`);
    const n = Number(limite);
    if (n < 1 || n > LIST_LIMIT_MAX) return invalid(`limite: inteiro de 1 a ${LIST_LIMIT_MAX}`);
    query.limite = n;
  }

  const cursor = params.get("cursor");
  if (cursor) {
    if (cursor.length > CURSOR_MAX_LENGTH || /[\p{Cc}\s]/u.test(cursor)) {
      return invalid("cursor inválido");
    }
    query.cursor = cursor;
  }

  const busca = params.get("busca");
  if (busca) {
    if ([...busca].length > SEARCH_MAX_RUNES) {
      return invalid(`busca: no máximo ${SEARCH_MAX_RUNES} caracteres`);
    }
    if (/\p{Cc}/u.test(busca)) return invalid("busca: sem caracteres de controle");
    query.busca = busca;
  }
  return { ok: true, query };
}
