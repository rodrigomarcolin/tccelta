import { problema } from "@/contrato/problema";
import { comSessao, responder, responderProblema } from "@/servidor/api";
import { obterCliente } from "@/servidor/viagens/cliente";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const LIMITE_CORPO = 4096;

export async function POST(req: Request) {
  return comSessao(async () => {
    const texto = await req.text();
    if (new TextEncoder().encode(texto).length > LIMITE_CORPO) {
      return responderProblema(problema(413, "corpo_grande", "corpo acima de 4 KiB"));
    }
    let corpo: unknown;
    try {
      corpo = JSON.parse(texto);
    } catch {
      return responderProblema(problema(400, "corpo_malformado", "JSON inválido"));
    }
    return responder(await obterCliente().criar(corpo));
  });
}
