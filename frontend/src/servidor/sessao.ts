import "server-only";
import { redirect } from "next/navigation";
import { auth } from "@/auth";

/** Confere a sessão num server component; sem sessão, vai a /entrar e volta depois. */
export async function exigirSessao(caminho: string): Promise<void> {
  const sessao = await auth();
  if (!sessao?.user) redirect(`/entrar?retorno=${encodeURIComponent(caminho)}`);
}
