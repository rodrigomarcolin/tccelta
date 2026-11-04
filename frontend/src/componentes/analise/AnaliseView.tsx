"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import type { Analise, Evento } from "@/contrato/tipos";
import {
  eixoDaAnalise,
  formatadorDeTempo,
  formatadorDeTempoCompleto,
  instanteMaisProximo,
  temLeituras,
  temposDaSerie,
} from "@/dominio/analise";
import { useTema } from "@/componentes/ui/Tema";
import { Cabecalho } from "./Cabecalho";
import { CombustivelMotor } from "./CombustivelMotor";
import { EstadoAguardandoDados, EstadoSemDados, FaixaConexao, FaixaProvisoria } from "./Estados";
import { FaixaAvisos } from "./FaixaAvisos";
import { TooltipInstante } from "./Instante";
import { LinhaDoTempo, type Cursor } from "./LinhaDoTempo";
import { Mapa } from "./Mapa";
import { MarchasRotacao } from "./MarchasRotacao";
import { Qualidade } from "./Qualidade";
import { Legenda, Resumo } from "./Resumo";
import { TabelaEventos } from "./TabelaEventos";

type Props = {
  analise: Analise;
  provisoria: boolean;
  /** Passou o tempo de acompanhar a viagem provisória. */
  provisoriaExpirada?: boolean;
  /** Aviso de conexão ou de servidor ocupado. */
  aviso?: string | null;
  /** Hora (HH:MM) da última versão que substituiu a tela. */
  atualizadoEm?: string | null;
};

export function AnaliseView({
  analise,
  provisoria,
  provisoriaExpirada = false,
  aviso = null,
  atualizadoEm = null,
}: Props) {
  const tema = useTema();
  const [cursor, setCursorEstado] = useState<Cursor>({ t: null, origem: "grafico" });
  const [foco, setFoco] = useState<{ lat: number; lon: number; nonce: number } | null>(null);
  const mapaRef = useRef<HTMLDivElement>(null);

  const eixo = useMemo(() => eixoDaAnalise(analise), [analise]);
  const fmtEixo = useMemo(() => formatadorDeTempo(analise), [analise]);
  const fmtT = useMemo(() => formatadorDeTempoCompleto(analise), [analise]);
  const formatarX = useCallback((x: number) => fmtEixo(eixo.de(x)), [fmtEixo, eixo]);
  const tempos = useMemo(() => temposDaSerie(analise), [analise]);

  const setCursor = useCallback((t: number | null, origem: Cursor["origem"]) => {
    setCursorEstado((prev) => {
      if (prev.t === t && prev.origem === origem) return prev;
      // Sair do gráfico ou do mapa não apaga um instante marcado de propósito.
      if (t === null && prev.origem === "fixo") return prev;
      return { t, origem };
    });
  }, []);

  const verNoMapa = useCallback((e: Evento) => {
    if (e.lat === null || e.lon === null) return;
    setFoco({ lat: e.lat, lon: e.lon, nonce: Date.now() });
    setCursorEstado({ t: e.t_inicio_ms, origem: "fixo" });
    mapaRef.current?.scrollIntoView?.({ block: "center", behavior: "smooth" });
  }, []);

  const marcarInstante = useCallback((t: number) => setCursorEstado({ t, origem: "fixo" }), []);

  const vazia = !temLeituras(analise);
  const instante = cursor.t === null ? null : instanteMaisProximo(analise, tempos, cursor.t);
  const rota = analise.rota;
  const comMapa = rota !== null && rota.lat.some((v) => v !== null);

  return (
    <div className="analise">
      <Cabecalho analise={analise} />
      {aviso ? <FaixaConexao texto={aviso} /> : null}
      {provisoria ? <FaixaProvisoria expirada={provisoriaExpirada} /> : null}
      {atualizadoEm ? (
        <p className="nota" role="status">
          Resultado atualizado às {atualizadoEm}
        </p>
      ) : null}
      <FaixaAvisos avisos={analise.avisos} />

      {vazia ? (
        provisoria ? (
          <EstadoAguardandoDados />
        ) : (
          <EstadoSemDados />
        )
      ) : (
        <>
          <section className={`resumo${comMapa ? "" : " resumo-total"}`} aria-labelledby="t-resumo">
            <h2 id="t-resumo" className="so-leitor">
              Resumo
            </h2>
            {comMapa && rota ? (
              <div className="resumo-mapa" ref={mapaRef}>
                <div className="mapa-moldura">
                  <Mapa
                    rota={rota}
                    eventos={analise.eventos}
                    paradas={analise.paradas}
                    tema={tema}
                    cursorT={cursor.t}
                    onHoverT={(t) => setCursor(t, "mapa")}
                    foco={foco}
                    formatarT={fmtT}
                  />
                  <TooltipInstante
                    className="tooltip-mapa"
                    ocultarVazio
                    instante={cursor.origem === "mapa" ? instante : null}
                    formatarT={fmtT}
                  />
                </div>
                <Legenda analise={analise} />
              </div>
            ) : null}
            <div className="resumo-indicadores">
              <Resumo analise={analise} />
              {!comMapa ? <Legenda analise={analise} /> : null}
            </div>
          </section>

          <section className="secao" aria-labelledby="t-linha">
            <h2 id="t-linha">Linha do tempo</h2>
            <LinhaDoTempo
              analise={analise}
              eixo={eixo}
              formatarX={formatarX}
              cursor={cursor}
              onCursor={setCursor}
            />
          </section>

          <section className="secao" aria-labelledby="t-marchas">
            <h2 id="t-marchas">Marchas e rotação</h2>
            <MarchasRotacao analise={analise} formatarT={fmtT} onMarcarInstante={marcarInstante} />
          </section>

          <section className="secao" aria-labelledby="t-comb">
            <h2 id="t-comb">Combustível e motor</h2>
            <CombustivelMotor analise={analise} eixo={eixo} formatarX={formatarX} />
          </section>

          <section className="secao" aria-labelledby="t-eventos">
            <h2 id="t-eventos">Eventos</h2>
            <TabelaEventos eventos={analise.eventos} formatarT={fmtT} onVerNoMapa={verNoMapa} />
          </section>
        </>
      )}

      <section className="secao" aria-label="Qualidade dos dados e metodologia">
        <Qualidade analise={analise} />
      </section>
    </div>
  );
}
