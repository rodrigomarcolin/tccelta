import type { ReactNode } from "react";
import Link from "next/link";
import { Botao } from "@/componentes/ui/Botao";
import { Distintivo } from "@/componentes/ui/Distintivo";
import { IconeRecarregar, IconeVoltar } from "@/componentes/icones/Icones";

function Estado({
  children,
  papel = "status",
}: {
  children: ReactNode;
  papel?: "status" | "alert";
}) {
  return (
    <div className="estado" role={papel}>
      {children}
    </div>
  );
}

export function EstadoEspera({ motorista }: { motorista: string | null }) {
  return (
    <Estado>
      <Distintivo tom="ao-vivo" pulsar>
        Ao vivo
      </Distintivo>
      <p className="estado-titulo">
        {motorista ? `Processando a viagem de «${motorista}»` : "Processando a viagem"}
      </p>
      <p className="nota">A análise aparece aqui quando os dados chegarem.</p>
    </Estado>
  );
}

export function EstadoCarregando() {
  return (
    <div className="estado esqueleto" aria-busy="true" aria-label="Carregando a análise">
      <div className="esqueleto-bloco" />
      <div className="esqueleto-bloco" />
      <div className="esqueleto-bloco" />
    </div>
  );
}

export function EstadoFalhou() {
  return (
    <Estado papel="alert">
      <p className="estado-titulo">
        Não foi possível calcular esta viagem. O nome do motorista foi liberado.
      </p>
      <Link href="/processar" className="botao botao-primario">
        <IconeVoltar />
        Processar outra viagem
      </Link>
    </Estado>
  );
}

export function EstadoNaoEncontrada() {
  return (
    <Estado papel="alert">
      <p className="estado-titulo">Viagem não encontrada</p>
      <Link href="/processar" className="botao botao-primario">
        <IconeVoltar />
        Voltar para processar
      </Link>
    </Estado>
  );
}

export function EstadoErro({ onTentar }: { onTentar: () => void }) {
  return (
    <Estado papel="alert">
      <p className="estado-titulo">Não foi possível carregar a viagem.</p>
      <Botao icone={<IconeRecarregar />} onClick={onTentar}>
        Tentar de novo
      </Botao>
    </Estado>
  );
}

export function EstadoSemDados() {
  return (
    <Estado>
      <p className="estado-titulo">Sem dados para esta viagem</p>
    </Estado>
  );
}

export function EstadoAguardandoDados() {
  return (
    <Estado>
      <Distintivo tom="aviso" pulsar>
        Provisório
      </Distintivo>
      <p className="estado-titulo">Aguardando dados</p>
      <p className="nota">A tela atualiza sozinha quando os dados chegarem.</p>
    </Estado>
  );
}

export function FaixaProvisoria({ expirada = false }: { expirada?: boolean }) {
  return (
    <p className="faixa-provisoria" role="status">
      {expirada
        ? "Resultado provisório. Recarregue para verificar"
        : "Resultado provisório: ainda podem chegar dados"}
    </p>
  );
}

/** Aviso discreto de conexão ou de servidor ocupado; a tela não é esvaziada. */
export function FaixaConexao({ texto }: { texto: string }) {
  return (
    <p className="faixa-provisoria" role="status">
      {texto}
    </p>
  );
}

export function EstadoLimiteEspera({ onContinuar }: { onContinuar: () => void }) {
  return (
    <Estado papel="alert">
      <p className="estado-titulo">
        A viagem ainda não foi processada. O processamento no servidor pode estar parado.
      </p>
      <div className="acoes">
        <Botao icone={<IconeRecarregar />} onClick={onContinuar}>
          Continuar esperando
        </Botao>
        <Link href="/processar" className="botao botao-secundario">
          <IconeVoltar />
          Voltar
        </Link>
      </div>
    </Estado>
  );
}
