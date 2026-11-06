import "server-only";
import { NextResponse } from "next/server";
import { auth } from "@/auth";
import { operatorList } from "@/auth/operators";
import type { Problem, ApiResponse } from "@/domain/trip/types";
import { buildProblem } from "@/domain/trip/problem";

const NO_CACHE = { "Cache-Control": "no-store" };

/** Forwards status, body and `Retry-After` from the data layer, without caching. */
export function respond(resp: ApiResponse<unknown>): NextResponse {
  const isError = resp.status >= 400;
  return new NextResponse(JSON.stringify(resp.body), {
    status: resp.status,
    headers: {
      ...NO_CACHE,
      "Content-Type": isError ? "application/problem+json" : "application/json",
      ...(resp.retryAfter === undefined ? {} : { "Retry-After": String(resp.retryAfter) }),
    },
  });
}

export function respondProblem(p: Problem): NextResponse {
  return respond({ status: p.status, body: p });
}

/**
 * Runs the handler only with an operator session. No session: 401. A session outside
 * `OPERATOR_EMAILS`: 403. Nothing is read from the request or called before that.
 */
export async function withSession(run: () => Promise<NextResponse>): Promise<NextResponse> {
  const session = await auth();
  if (!session?.user) {
    return respondProblem(
      buildProblem(401, "nao_autenticado", "entre com uma conta USP autorizada"),
    );
  }
  const email = session.user.email?.trim().toLowerCase();
  if (!email || !operatorList(process.env.OPERATOR_EMAILS).includes(email)) {
    return respondProblem(buildProblem(403, "nao_autorizado", "conta sem permissão de operador"));
  }
  return run();
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Only a canonical-format UUID reaches the data layer; anything else is "unknown". */
export function validId(id: string): boolean {
  return UUID.test(id);
}

export function unknownTrip(): NextResponse {
  return respondProblem(buildProblem(404, "viagem_desconhecida", "viagem desconhecida"));
}
