import { NextResponse } from "next/server";
import { comSessao } from "@/servidor/api";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const ESTILOS: Record<string, string> = { claro: "light_all", escuro: "dark_all" };
const INTEIRO = /^\d{1,7}$/;

type Ctx = { params: Promise<{ estilo: string; z: string; x: string; y: string }> };

/**
 * Proxy dos tiles do CARTO: a chave (`ANALISE_MAPA_KEY`, só servidor, lida em runtime)
 * nunca vai ao navegador. Exige a mesma sessão de operador das demais rotas.
 */
export async function GET(_req: Request, ctx: Ctx) {
  return comSessao(async () => {
    const { estilo, z, x, y } = await ctx.params;
    const nome = ESTILOS[estilo];
    if (!nome || ![z, x, y].every((v) => INTEIRO.test(v))) {
      return new NextResponse(null, { status: 404 });
    }
    const chave = process.env.ANALISE_MAPA_KEY?.trim();
    const url = new URL(`https://basemaps.cartocdn.com/rastertiles/${nome}/${z}/${x}/${y}.png`);
    if (chave) url.searchParams.set("key", chave);
    try {
      const r = await fetch(url, { signal: AbortSignal.timeout(8000), redirect: "error" });
      if (!r.ok) return new NextResponse(null, { status: 502 });
      return new NextResponse(r.body, {
        status: 200,
        headers: {
          "Content-Type": r.headers.get("content-type") ?? "image/png",
          "Cache-Control": "private, max-age=86400",
        },
      });
    } catch {
      return new NextResponse(null, { status: 502 });
    }
  });
}
