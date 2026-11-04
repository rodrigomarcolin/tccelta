import Link from "next/link";
import type { TripAnalysis } from "@/domain/trip/types";
import { headerDescription } from "@/domain/trip/analysis";
import { IconBack } from "@/components/icons/Icons";
import { ThemeToggle } from "@/components/ui/Theme";

export function Header({ analysis }: { analysis: TripAnalysis }) {
  const { identification, when } = headerDescription(analysis);
  return (
    <header className="header">
      <div className="header-title">
        <Link
          href="/processar"
          className="button button-link no-print"
          aria-label="Voltar para processar viagem"
        >
          <IconBack />
          Nova viagem
        </Link>
        <h1>Análise de viagem</h1>
        <p className="header-row">
          <span className="data-value">{identification}</span>
          {when ? <span className="data-value">{when}</span> : null}
        </p>
        <p className="header-row header-driver">
          <span>{analysis.viagem.motorista.nome}</span>
          <span className="text-secondary">categoria {analysis.viagem.categoria_viagem}</span>
        </p>
      </div>
      <div className="no-print">
        <ThemeToggle />
      </div>
    </header>
  );
}
