import { comSessao, responder } from "@/servidor/api";
import { obterCliente } from "@/servidor/viagens/cliente";

export const dynamic = "force-dynamic";

export async function GET(_req: Request, ctx: { params: Promise<{ id: string }> }) {
  return comSessao(async () => {
    const { id } = await ctx.params;
    return responder(await obterCliente().analise(id));
  });
}
