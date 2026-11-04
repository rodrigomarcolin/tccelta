import type { CombustivelId, CriarViagemEntrada, Entrada, Problema } from "@/contrato/tipos";
import { COMBUSTIVEIS } from "@/config";
import { problema } from "@/contrato/problema";

type Resultado =
  | { ok: true; valor: CriarViagemEntrada & { combustivel: CombustivelId } }
  | { ok: false; problema: Problema };

const RFC3339 = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;
const SETE_DIAS_MS = 7 * 24 * 3600 * 1000;

const runes = (s: string) => [...s].length;
const inteiro = (v: unknown): v is number => typeof v === "number" && Number.isInteger(v);
const registro = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

function desconhecido(obj: Record<string, unknown>, permitidos: string[]): string | null {
  return Object.keys(obj).find((k) => !permitidos.includes(k)) ?? null;
}

const malformado = (detail: string): Resultado => ({
  ok: false,
  problema: problema(400, "corpo_malformado", detail),
});
const invalida = (detail: string): Resultado => ({
  ok: false,
  problema: problema(400, "entrada_invalida", detail),
});

/** Valida o corpo do POST com as mesmas regras da API2. */
export function validarCriacao(corpo: unknown): Resultado {
  if (!registro(corpo)) return malformado("o corpo deve ser um objeto");
  const extra = desconhecido(corpo, ["motorista", "categoria_viagem", "combustivel", "entrada"]);
  if (extra) return malformado(`campo desconhecido: ${extra}`);

  const { motorista, categoria_viagem: categoria, combustivel, entrada } = corpo;
  if (!registro(motorista) || typeof motorista.nome !== "string") {
    return invalida("motorista.nome é obrigatório");
  }
  const extraMot = desconhecido(motorista, ["nome"]);
  if (extraMot) return malformado(`campo desconhecido: motorista.${extraMot}`);
  const nome = motorista.nome;
  if (nome.length === 0 || nome !== nome.trim()) {
    return invalida("motorista.nome: obrigatório e sem espaços nas pontas");
  }
  if (runes(nome) > 60) return invalida("motorista.nome: no máximo 60 caracteres");
  if (/\p{Cc}/u.test(nome)) return invalida("motorista.nome: sem caracteres de controle");

  if (typeof categoria !== "string" || categoria.length === 0 || categoria !== categoria.trim()) {
    return invalida("categoria_viagem: obrigatória e sem espaços nas pontas");
  }
  if (runes(categoria) > 40) return invalida("categoria_viagem: no máximo 40 caracteres");
  if (!/^[\p{L}\p{N} _-]+$/u.test(categoria)) {
    return invalida("categoria_viagem: só letras, dígitos, espaço, _ e -");
  }

  if (typeof combustivel !== "string" || !COMBUSTIVEIS.some((c) => c.id === combustivel)) {
    return invalida("combustivel: use um dos presets");
  }

  const e = validarEntrada(entrada);
  if ("problema" in e) return { ok: false, problema: e.problema };
  return {
    ok: true,
    valor: {
      motorista: { nome },
      categoria_viagem: categoria.toLowerCase(),
      combustivel: combustivel as CombustivelId,
      entrada: e.entrada,
    },
  };
}

function validarEntrada(entrada: unknown): { entrada: Entrada } | { problema: Problema } {
  const falha = (detail: string) => ({ problema: problema(400, "entrada_invalida", detail) });
  if (!registro(entrada)) return falha("entrada é obrigatória");
  if (entrada.tipo === "boot") {
    const extra = desconhecido(entrada, ["tipo", "dongle_id", "boot_id", "boot_generation"]);
    if (extra) return falha(`entrada.${extra} não vale para boot`);
    if (!inteiro(entrada.dongle_id) || entrada.dongle_id < 1) {
      return falha("entrada.dongle_id: inteiro maior que zero");
    }
    if (!inteiro(entrada.boot_id) || entrada.boot_id < 1 || entrada.boot_id > 65535) {
      return falha("entrada.boot_id: de 1 a 65535");
    }
    const g = entrada.boot_generation;
    if (g !== undefined && g !== null && (!inteiro(g) || g < 0 || g > 65535)) {
      return falha("entrada.boot_generation: de 0 a 65535");
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
    const extra = desconhecido(entrada, ["tipo", "dongle_id", "t0", "t1"]);
    if (extra) return falha(`entrada.${extra} não vale para intervalo`);
    if (!inteiro(entrada.dongle_id) || entrada.dongle_id < 1) {
      return falha("entrada.dongle_id: inteiro maior que zero");
    }
    const { t0, t1 } = entrada;
    if (typeof t0 !== "string" || !RFC3339.test(t0)) return falha("entrada.t0: RFC 3339 com fuso");
    if (typeof t1 !== "string" || !RFC3339.test(t1)) return falha("entrada.t1: RFC 3339 com fuso");
    const a = Date.parse(t0);
    const b = Date.parse(t1);
    if (Number.isNaN(a) || Number.isNaN(b)) return falha("entrada: data inválida");
    if (b <= a) return falha("entrada.t1 deve ser maior que t0");
    if (b - a > SETE_DIAS_MS) return falha("entrada: intervalo de no máximo 7 dias");
    return {
      entrada: {
        tipo: "intervalo",
        dongle_id: entrada.dongle_id,
        t0: new Date(a).toISOString(),
        t1: new Date(b).toISOString(),
      },
    };
  }
  return falha("entrada.tipo: boot ou intervalo");
}

export function normalizarNome(nome: string): string {
  return nome.trim().toLowerCase();
}
