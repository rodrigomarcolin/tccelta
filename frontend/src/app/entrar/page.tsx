import { redirect } from "next/navigation";
import { auth, signIn } from "@/auth";
import { retornoSeguro } from "@/auth/operadores";
import { Botao } from "@/componentes/ui/Botao";
import { IconeDongle } from "@/componentes/icones/Icones";

export const dynamic = "force-dynamic";

const MENSAGENS: Record<string, string> = {
  conta_nao_liberada:
    "Esta conta não está liberada para processar viagens. Use um e-mail USP autorizado.",
};

export default async function Entrar({
  searchParams,
}: {
  searchParams: Promise<{ erro?: string; retorno?: string; error?: string }>;
}) {
  const { erro, retorno, error } = await searchParams;
  const destino = retornoSeguro(retorno);
  const sessao = await auth();
  if (sessao?.user) redirect(destino);

  const mensagem = erro
    ? (MENSAGENS[erro] ?? "Não foi possível entrar. Tente de novo.")
    : error
      ? "Não foi possível entrar. Tente de novo."
      : null;

  async function entrarComGoogle() {
    "use server";
    await signIn("google", { redirectTo: destino });
  }

  return (
    <main className="pagina-estreita">
      <div className="cartao cartao-login">
        <IconeDongle tamanho={28} />
        <h1>Análise de viagem</h1>
        <p className="texto-secundario">Acesso restrito a operadores com e-mail USP autorizado.</p>
        {mensagem ? (
          <p className="campo-erro" role="alert">
            {mensagem}
          </p>
        ) : null}
        <form action={entrarComGoogle}>
          <Botao type="submit" larguraTotal>
            Entrar com Google
          </Botao>
        </form>
      </div>
    </main>
  );
}
