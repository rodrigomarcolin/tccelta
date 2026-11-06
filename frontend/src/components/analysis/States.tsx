import type { ReactNode } from "react";
import Link from "next/link";
import { Button, buttonClass } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { Note } from "@/components/ui/Note";
import { IconReload, IconBack } from "@/components/icons/Icons";

function State({ children, role = "status" }: { children: ReactNode; role?: "status" | "alert" }) {
  return (
    <Card roomy className="grid justify-items-start gap-4" role={role}>
      {children}
    </Card>
  );
}

function StateTitle({ children }: { children: ReactNode }) {
  return <p className="m-0 type-title">{children}</p>;
}

const BANNER = "m-0 mb-5 rounded-md border border-hairline px-5 py-3";

export function WaitingState({ driverName }: { driverName: string | null }) {
  return (
    <State>
      <Badge tone="live" pulse>
        Ao vivo
      </Badge>
      <StateTitle>
        {driverName ? `Processando a viagem de «${driverName}»` : "Processando a viagem"}
      </StateTitle>
      <Note>A análise aparece aqui quando os dados chegarem.</Note>
    </State>
  );
}

export function LoadingState() {
  return (
    <Card
      roomy
      className="grid justify-items-start gap-4"
      aria-busy="true"
      aria-label="Carregando a análise"
    >
      <div className="h-[120px] w-full rounded-md bg-track" />
      <div className="h-[120px] w-full rounded-md bg-track" />
      <div className="h-[120px] w-full rounded-md bg-track" />
    </Card>
  );
}

export function FailedState() {
  return (
    <State role="alert">
      <StateTitle>
        Não foi possível calcular esta viagem. O nome do motorista foi liberado.
      </StateTitle>
      <Link href="/processar" className={buttonClass()}>
        <IconBack />
        Processar outra viagem
      </Link>
    </State>
  );
}

export function NotFoundState() {
  return (
    <State role="alert">
      <StateTitle>Viagem não encontrada</StateTitle>
      <Link href="/viagens" className={buttonClass()}>
        <IconBack />
        Ver todas as viagens
      </Link>
    </State>
  );
}

export function ErrorState({
  onRetry,
  text = "Não foi possível carregar a viagem.",
}: {
  onRetry: () => void;
  text?: string;
}) {
  return (
    <State role="alert">
      <StateTitle>{text}</StateTitle>
      <Button icon={<IconReload />} onClick={onRetry}>
        Tentar de novo
      </Button>
    </State>
  );
}

export function NoDataState() {
  return (
    <State>
      <StateTitle>Sem dados para esta viagem</StateTitle>
    </State>
  );
}

export function AwaitingDataState() {
  return (
    <State>
      <Badge tone="warning" pulse>
        Provisório
      </Badge>
      <StateTitle>Aguardando dados</StateTitle>
      <Note>A tela atualiza sozinha quando os dados chegarem.</Note>
    </State>
  );
}

export function ProvisionalBanner({ expired = false }: { expired?: boolean }) {
  return (
    <p className={`${BANNER} bg-accent-wash`} role="status">
      {expired
        ? "Resultado provisório. Recarregue para verificar"
        : "Resultado provisório: ainda podem chegar dados"}
    </p>
  );
}

/** Discreet notice for connection or busy-server problems; the screen is not cleared. */
export function ConnectionBanner({ text }: { text: string }) {
  return (
    <p className={`${BANNER} bg-accent-wash`} role="status">
      {text}
    </p>
  );
}

export function WaitLimitState({ onContinue }: { onContinue: () => void }) {
  return (
    <State role="alert">
      <StateTitle>
        A viagem ainda não foi processada. O processamento no servidor pode estar parado.
      </StateTitle>
      <div className="flex flex-wrap gap-3">
        <Button icon={<IconReload />} onClick={onContinue}>
          Continuar esperando
        </Button>
        <Link href="/processar" className={buttonClass({ variant: "secondary" })}>
          <IconBack />
          Voltar
        </Link>
      </div>
    </State>
  );
}
