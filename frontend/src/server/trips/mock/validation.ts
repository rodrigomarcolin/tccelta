import type { FuelId, CreateTripRequest, TripInput, Problem } from "@/domain/trip/types";
import { FUELS } from "@/config";
import { buildProblem } from "@/domain/trip/problem";

type Result =
  | { ok: true; value: CreateTripRequest & { combustivel: FuelId } }
  | { ok: false; problem: Problem };

const RFC3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;
const SEVEN_DAYS_MS = 7 * 24 * 3600 * 1000;

const runes = (s: string) => [...s].length;
const isInteger = (v: unknown): v is number => typeof v === "number" && Number.isInteger(v);
const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

function unknownKey(obj: Record<string, unknown>, allowed: string[]): string | null {
  return Object.keys(obj).find((k) => !allowed.includes(k)) ?? null;
}

const malformed = (detail: string): Result => ({
  ok: false,
  problem: buildProblem(400, "corpo_malformado", detail),
});
const invalid = (detail: string): Result => ({
  ok: false,
  problem: buildProblem(400, "entrada_invalida", detail),
});

/** Validates the POST body with the same rules as API2. */
export function validateCreation(body: unknown): Result {
  if (!isRecord(body)) return malformed("o corpo deve ser um objeto");
  const extra = unknownKey(body, ["motorista", "categoria_viagem", "combustivel", "entrada"]);
  if (extra) return malformed(`campo desconhecido: ${extra}`);

  const { motorista, categoria_viagem: category, combustivel, entrada } = body;
  if (!isRecord(motorista) || typeof motorista.nome !== "string") {
    return invalid("motorista.nome é obrigatório");
  }
  const extraDriver = unknownKey(motorista, ["nome"]);
  if (extraDriver) return malformed(`campo desconhecido: motorista.${extraDriver}`);
  const name = motorista.nome;
  if (name.length === 0 || name !== name.trim()) {
    return invalid("motorista.nome: obrigatório e sem espaços nas pontas");
  }
  if (runes(name) > 60) return invalid("motorista.nome: no máximo 60 caracteres");
  if (/\p{Cc}/u.test(name)) return invalid("motorista.nome: sem caracteres de controle");

  if (typeof category !== "string" || category.length === 0 || category !== category.trim()) {
    return invalid("categoria_viagem: obrigatória e sem espaços nas pontas");
  }
  if (runes(category) > 40) return invalid("categoria_viagem: no máximo 40 caracteres");
  if (!/^[\p{L}\p{N} _-]+$/u.test(category)) {
    return invalid("categoria_viagem: só letras, dígitos, espaço, _ e -");
  }

  if (typeof combustivel !== "string" || !FUELS.some((c) => c.id === combustivel)) {
    return invalid("combustivel: use um dos presets");
  }

  const e = validateInput(entrada);
  if ("problem" in e) return { ok: false, problem: e.problem };
  return {
    ok: true,
    value: {
      motorista: { nome: name },
      categoria_viagem: category.toLowerCase(),
      combustivel: combustivel as FuelId,
      entrada: e.entrada,
    },
  };
}

function validateInput(entrada: unknown): { entrada: TripInput } | { problem: Problem } {
  const fail = (detail: string) => ({ problem: buildProblem(400, "entrada_invalida", detail) });
  if (!isRecord(entrada)) return fail("entrada é obrigatória");
  if (entrada.tipo === "boot") {
    const extra = unknownKey(entrada, ["tipo", "dongle_id", "boot_id", "boot_generation"]);
    if (extra) return fail(`entrada.${extra} não vale para boot`);
    if (!isInteger(entrada.dongle_id) || entrada.dongle_id < 1) {
      return fail("entrada.dongle_id: inteiro maior que zero");
    }
    if (!isInteger(entrada.boot_id) || entrada.boot_id < 1 || entrada.boot_id > 65535) {
      return fail("entrada.boot_id: de 1 a 65535");
    }
    const g = entrada.boot_generation;
    if (g !== undefined && g !== null && (!isInteger(g) || g < 0 || g > 65535)) {
      return fail("entrada.boot_generation: de 0 a 65535");
    }
    return {
      entrada: {
        tipo: "boot",
        dongle_id: entrada.dongle_id,
        boot_id: entrada.boot_id,
        boot_generation: g === undefined ? null : (g as number | null),
      },
    };
  }
  if (entrada.tipo === "intervalo") {
    const extra = unknownKey(entrada, ["tipo", "dongle_id", "t0", "t1"]);
    if (extra) return fail(`entrada.${extra} não vale para intervalo`);
    if (!isInteger(entrada.dongle_id) || entrada.dongle_id < 1) {
      return fail("entrada.dongle_id: inteiro maior que zero");
    }
    const { t0, t1 } = entrada;
    if (typeof t0 !== "string" || !RFC3339.test(t0)) return fail("entrada.t0: RFC 3339 com fuso");
    if (typeof t1 !== "string" || !RFC3339.test(t1)) return fail("entrada.t1: RFC 3339 com fuso");
    const a = Date.parse(t0);
    const b = Date.parse(t1);
    if (Number.isNaN(a) || Number.isNaN(b)) return fail("entrada: data inválida");
    if (b <= a) return fail("entrada.t1 deve ser maior que t0");
    if (b - a > SEVEN_DAYS_MS) return fail("entrada: intervalo de no máximo 7 dias");
    return {
      entrada: {
        tipo: "intervalo",
        dongle_id: entrada.dongle_id,
        t0: new Date(a).toISOString(),
        t1: new Date(b).toISOString(),
      },
    };
  }
  return fail("entrada.tipo: boot ou intervalo");
}

export function normalizeName(name: string): string {
  return name.trim().toLowerCase();
}
