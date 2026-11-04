import "server-only";
import { NextResponse } from "next/server";
import { auth } from "@/auth";
import { listaOperadores } from "@/auth/operadores";
import type { Problema, Resposta } from "@/contrato/tipos";
import { problema } from "@/contrato/problema";

const SEM_CACHE = { "Cache-Control": "no-store" };

/** Repassa status, corpo e `Retry-After` da camada de dados, sem cache. */
export function responder(resp: Resposta<unknown>): NextResponse {
  const erro = resp.status >= 400;
  return new NextResponse(JSON.stringify(resp.corpo), {
    status: resp.status,
    headers: {
      ...SEM_CACHE,
      "Content-Type": erro ? "application/problem+json" : "application/json",
      ...(resp.retryAfter === undefined ? {} : { "Retry-After": String(resp.retryAfter) }),
    },
  });
}

export function responderProblema(p: Problema): NextResponse {
  return responder({ status: p.status, corpo: p });
}

/**
 * Executa o handler só com sessão de operador. Sem sessão: 401. Com sessão fora
 * de `OPERATOR_EMAILS`: 403. Nada é lido da requisição nem chamado antes disso.
 */
export async function comSessao(executar: () => Promise<NextResponse>): Promise<NextResponse> {
  const sessao = await auth();
  if (!sessao?.user) {
    return responderProblema(
      problema(401, "nao_autenticado", "entre com uma conta USP autorizada"),
    );
  }
  const email = sessao.user.email?.trim().toLowerCase();
  if (!email || !listaOperadores(process.env.OPERATOR_EMAILS).includes(email)) {
    return responderProblema(problema(403, "nao_autorizado", "conta sem permissão de operador"));
  }
  return executar();
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Só um UUID em formato canônico passa à camada de dados; o resto é "desconhecida". */
export function idValido(id: string): boolean {
  return UUID.test(id);
}

export function viagemDesconhecida(): NextResponse {
  return responderProblema(problema(404, "viagem_desconhecida", "viagem desconhecida"));
}
