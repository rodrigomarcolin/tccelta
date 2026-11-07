import { NextResponse } from "next/server";
import { withSession, validId, respond, unknownTrip, NO_CACHE } from "@/server/api";
import { getDatasource } from "@/server/trips/datasource";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(_req: Request, ctx: { params: Promise<{ id: string }> }) {
  return withSession(async () => {
    const { id } = await ctx.params;
    if (!validId(id)) return unknownTrip();
    return respond(await getDatasource().status(id));
  });
}

export async function DELETE(_req: Request, ctx: { params: Promise<{ id: string }> }) {
  return withSession(async () => {
    const { id } = await ctx.params;
    if (!validId(id)) return unknownTrip();
    const result = await getDatasource().remove(id);
    // 204 carries no body, so it cannot go through `respond`.
    if (result.status === 204) return new NextResponse(null, { status: 204, headers: NO_CACHE });
    return respond(result);
  });
}
