export type PerfilGoogle = { email?: string | null; email_verified?: boolean | null };

/** Lê a lista de operadores: separada por vírgula, sem espaços nem maiúsculas. */
export function listaOperadores(bruto: string | undefined | null): string[] {
  return (bruto ?? "")
    .split(",")
    .map((e) => e.trim().toLowerCase())
    .filter((e) => e.length > 0);
}

/**
 * Decide se a conta Google entra: `email_verified`, domínio `usp.br` e e-mail
 * na lista de operadores. Lista vazia ou ausente não libera ninguém.
 */
export function contaLiberada(
  perfil: PerfilGoogle | null | undefined,
  bruto: string | undefined | null,
): boolean {
  if (!perfil || perfil.email_verified !== true || typeof perfil.email !== "string") return false;
  const email = perfil.email.trim().toLowerCase();
  if (!email.endsWith("@usp.br") || email.indexOf("@") !== email.lastIndexOf("@")) return false;
  return listaOperadores(bruto).includes(email);
}

/** Aceita só caminhos internos, para o retorno depois do login. */
export function retornoSeguro(valor: string | undefined | null): string {
  if (!valor || !valor.startsWith("/") || valor.startsWith("//") || valor.includes("\\"))
    return "/processar";
  return valor;
}
