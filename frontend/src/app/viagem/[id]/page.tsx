import { ViagemPagina } from "@/componentes/analise/ViagemPagina";
import { exigirSessao } from "@/servidor/sessao";

export const dynamic = "force-dynamic";

export default async function Viagem({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await exigirSessao(`/viagem/${id}`);
  return (
    <main className="pagina-larga">
      <ViagemPagina id={id} />
    </main>
  );
}
