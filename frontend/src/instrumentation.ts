export async function register(): Promise<void> {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    // Falha ao subir se a fonte de dados estiver mal configurada.
    const { escolherFonte } = await import("@/servidor/viagens/cliente");
    escolherFonte();
  }
}
