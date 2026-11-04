import { comSessao, idValido, responder, viagemDesconhecida } from "@/servidor/api";
import { obterCliente } from "@/servidor/viagens/cliente";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(_req: Request, ctx: { params: Promise<{ id: string }> }) {
  return comSessao(async () => {
    const { id } = await ctx.params;
    if (!idValido(id)) return viagemDesconhecida();
    return responder(await obterCliente().status(id));
  });
}
