import type { Ref } from "react";
import Link from "next/link";
import type { TripAnalysis } from "@/domain/trip/types";
import { headerDescription } from "@/domain/trip/analysis";
import { IconBack } from "@/components/icons/Icons";
import { HeaderControls } from "@/components/ui/AppNav";
import { buttonClass } from "@/components/ui/Button";
import { DataValue } from "@/components/ui/DataValue";
import { PageHeader, PageHeaderTitle } from "@/components/ui/PageHeader";

export function Header({
  analysis,
  headerRef,
}: {
  analysis: TripAnalysis;
  headerRef?: Ref<HTMLElement>;
}) {
  const { identification, when } = headerDescription(analysis);
  return (
    <PageHeader sticky ref={headerRef}>
      <PageHeaderTitle>
        <Link
          href="/viagens"
          className={buttonClass({ variant: "link", className: "justify-self-start print:hidden" })}
          aria-label="Voltar para a lista de viagens"
        >
          <IconBack />
          Viagens
        </Link>
        <h1>Análise de viagem</h1>
        <p className="m-0 flex flex-wrap gap-4 text-secondary">
          <DataValue>{identification}</DataValue>
          {when ? <DataValue>{when}</DataValue> : null}
        </p>
        <p className="m-0 flex flex-wrap gap-4 text-primary">
          <span>{analysis.viagem.motorista.nome}</span>
          <span className="text-secondary">categoria {analysis.viagem.categoria_viagem}</span>
        </p>
      </PageHeaderTitle>
      <HeaderControls navHiddenOnPhone />
    </PageHeader>
  );
}
