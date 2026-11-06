import { buildProblem } from "@/domain/trip/problem";
import { withSession, respond, respondProblem } from "@/server/api";
import { getDatasource } from "@/server/trips/datasource";
import { parseListQuery } from "@/server/trips/listQuery";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const BODY_LIMIT = 4096;

export async function GET(req: Request) {
  return withSession(async () => {
    const parsed = parseListQuery(new URL(req.url).searchParams);
    if (!parsed.ok) return respondProblem(parsed.problem);
    return respond(await getDatasource().list(parsed.query));
  });
}

export async function POST(req: Request) {
  return withSession(async () => {
    const text = await req.text();
    if (new TextEncoder().encode(text).length > BODY_LIMIT) {
      return respondProblem(buildProblem(413, "corpo_grande", "corpo acima de 4 KiB"));
    }
    let body: unknown;
    try {
      body = JSON.parse(text);
    } catch {
      return respondProblem(buildProblem(400, "corpo_malformado", "JSON inválido"));
    }
    return respond(await getDatasource().create(body));
  });
}
