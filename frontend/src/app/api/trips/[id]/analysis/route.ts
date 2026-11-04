import { withSession, validId, respond, unknownTrip } from "@/server/api";
import { getDatasource } from "@/server/trips/datasource";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(_req: Request, ctx: { params: Promise<{ id: string }> }) {
  return withSession(async () => {
    const { id } = await ctx.params;
    if (!validId(id)) return unknownTrip();
    return respond(await getDatasource().analysis(id));
  });
}
