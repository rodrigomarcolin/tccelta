import type { ReactNode, Ref } from "react";
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
  actions,
}: {
  analysis: TripAnalysis;
  headerRef?: Ref<HTMLElement>;
  /** Page actions (delete), on the right of the back link. */
  actions?: ReactNode;
}) {
  const { identification, when } = headerDescription(analysis);
  return (
    <PageHeader sticky ref={headerRef}>
      <PageHeaderTitle>
        <div className="flex items-center justify-between gap-3 print:hidden">
          <Link
            href="/viagens"
            className={buttonClass({ variant: "link" })}
            aria-label="Voltar para a lista de viagens"
          >
            <IconBack />
            Viagens
          </Link>
          {actions}
        </div>
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
