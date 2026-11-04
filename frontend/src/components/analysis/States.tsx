import type { ReactNode } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { IconReload, IconBack } from "@/components/icons/Icons";

function State({ children, role = "status" }: { children: ReactNode; role?: "status" | "alert" }) {
  return (
    <div className="state" role={role}>
      {children}
    </div>
  );
}

export function WaitingState({ driverName }: { driverName: string | null }) {
  return (
    <State>
      <Badge tone="live" pulse>
        Ao vivo
      </Badge>
      <p className="state-title">
        {driverName ? `Processando a viagem de «${driverName}»` : "Processando a viagem"}
      </p>
      <p className="note">A análise aparece aqui quando os dados chegarem.</p>
    </State>
  );
}

export function LoadingState() {
  return (
    <div className="state skeleton" aria-busy="true" aria-label="Carregando a análise">
      <div className="skeleton-block" />
      <div className="skeleton-block" />
      <div className="skeleton-block" />
    </div>
  );
}

export function FailedState() {
  return (
    <State role="alert">
      <p className="state-title">
        Não foi possível calcular esta viagem. O nome do motorista foi liberado.
      </p>
      <Link href="/processar" className="button button-primary">
        <IconBack />
        Processar outra viagem
      </Link>
    </State>
  );
}

export function NotFoundState() {
  return (
    <State role="alert">
      <p className="state-title">Viagem não encontrada</p>
      <Link href="/processar" className="button button-primary">
        <IconBack />
        Voltar para processar
      </Link>
    </State>
  );
}

export function ErrorState({ onRetry }: { onRetry: () => void }) {
  return (
    <State role="alert">
      <p className="state-title">Não foi possível carregar a viagem.</p>
      <Button icon={<IconReload />} onClick={onRetry}>
        Tentar de novo
      </Button>
    </State>
  );
}

export function NoDataState() {
  return (
    <State>
      <p className="state-title">Sem dados para esta viagem</p>
    </State>
  );
}

export function AwaitingDataState() {
  return (
    <State>
      <Badge tone="warning" pulse>
        Provisório
      </Badge>
      <p className="state-title">Aguardando dados</p>
      <p className="note">A tela atualiza sozinha quando os dados chegarem.</p>
    </State>
  );
}

export function ProvisionalBanner({ expired = false }: { expired?: boolean }) {
  return (
    <p className="provisional-banner" role="status">
      {expired
        ? "Resultado provisório. Recarregue para verificar"
        : "Resultado provisório: ainda podem chegar dados"}
    </p>
  );
}

/** Discreet notice for connection or busy-server problems; the screen is not cleared. */
export function ConnectionBanner({ text }: { text: string }) {
  return (
    <p className="provisional-banner" role="status">
      {text}
    </p>
  );
}

export function WaitLimitState({ onContinue }: { onContinue: () => void }) {
  return (
    <State role="alert">
      <p className="state-title">
        A viagem ainda não foi processada. O processamento no servidor pode estar parado.
      </p>
      <div className="actions">
        <Button icon={<IconReload />} onClick={onContinue}>
          Continuar esperando
        </Button>
        <Link href="/processar" className="button button-secondary">
          <IconBack />
          Voltar
        </Link>
      </div>
    </State>
  );
}
