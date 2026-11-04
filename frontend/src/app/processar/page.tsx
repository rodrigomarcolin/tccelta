import { FormularioProcessar } from "@/componentes/processar/FormularioProcessar";
import { AlternarTema } from "@/componentes/ui/Tema";
import { exigirSessao } from "@/servidor/sessao";

export const dynamic = "force-dynamic";

export default async function Processar() {
  await exigirSessao("/processar");
  return (
    <main className="pagina-estreita">
      <header className="cabecalho">
        <div className="cabecalho-titulo">
          <h1>Processar viagem</h1>
          <p className="texto-secundario">Informe o motorista, o combustível e a viagem.</p>
        </div>
        <AlternarTema />
      </header>
      <FormularioProcessar />
    </main>
  );
}
