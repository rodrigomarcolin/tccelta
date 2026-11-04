import Link from "next/link";
import type { Analise } from "@/contrato/tipos";
import { descricaoCabecalho } from "@/dominio/analise";
import { IconeVoltar } from "@/componentes/icones/Icones";
import { AlternarTema } from "@/componentes/ui/Tema";

export function Cabecalho({ analise }: { analise: Analise }) {
  const { identificacao, quando } = descricaoCabecalho(analise);
  return (
    <header className="cabecalho">
      <div className="cabecalho-titulo">
        <Link
          href="/processar"
          className="botao botao-link nao-imprimir"
          aria-label="Voltar para processar viagem"
        >
          <IconeVoltar />
          Nova viagem
        </Link>
        <h1>Análise de viagem</h1>
        <p className="cabecalho-linha">
          <span className="dado">{identificacao}</span>
          {quando ? <span className="dado">{quando}</span> : null}
        </p>
        <p className="cabecalho-linha cabecalho-motorista">
          <span>{analise.viagem.motorista.nome}</span>
          <span className="texto-secundario">categoria {analise.viagem.categoria_viagem}</span>
        </p>
      </div>
      <div className="nao-imprimir">
        <AlternarTema />
      </div>
    </header>
  );
}
