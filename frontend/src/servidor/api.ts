import "server-only";
import { NextResponse } from "next/server";
import { auth } from "@/auth";
import type { Problema, Resposta } from "@/contrato/tipos";
import { problema } from "@/contrato/problema";

const SEM_CACHE = { "Cache-Control": "no-store" };

/** Repassa status e corpo da camada de dados, sem cache. */
export function responder(resp: Resposta<unknown>): NextResponse {
  const erro = resp.status >= 400;
  return new NextResponse(JSON.stringify(resp.corpo), {
    status: resp.status,
    headers: {
      ...SEM_CACHE,
      "Content-Type": erro ? "application/problem+json" : "application/json",
    },
  });
}

export function responderProblema(p: Problema): NextResponse {
  return responder({ status: p.status, corpo: p });
}

/** Executa o handler só com sessão; senão responde 401 com problema. */
export async function comSessao(executar: () => Promise<NextResponse>): Promise<NextResponse> {
  const sessao = await auth();
  if (!sessao?.user) {
    return responderProblema(
      problema(401, "nao_autenticado", "entre com uma conta USP autorizada"),
    );
  }
  return executar();
}
