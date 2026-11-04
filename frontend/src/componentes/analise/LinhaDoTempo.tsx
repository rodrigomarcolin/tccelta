"use client";

import { useMemo } from "react";
import type { Analise } from "@/contrato/tipos";
import type { Eixo } from "@/dominio/eixo";
import {
  algumValor,
  formatadorDeTempoCompleto,
  instanteMaisProximo,
  temposDaSerie,
  textoPausa,
} from "@/dominio/analise";
import { numero } from "@/formato/numeros";
import { Grafico } from "./Grafico";
import type { Marcador, Pausa } from "./GraficoUPlot";
import { TooltipInstante } from "./Instante";

export type Cursor = { t: number | null; origem: "grafico" | "mapa" | "fixo" };

type Props = {
  analise: Analise;
  eixo: Eixo;
  formatarX: (x: number) => string;
  cursor: Cursor;
  onCursor: (t: number | null, origem: Cursor["origem"]) => void;
};

const SYNC = "linha-do-tempo";

function rotuloY(v: number): string {
  return numero(v, 0);
}

export function LinhaDoTempo({ analise, eixo, formatarX, cursor, onCursor }: Props) {
  const s = analise.series;
  const tempos = useMemo(() => temposDaSerie(analise), [analise]);
  const x = useMemo(() => tempos.map((t) => eixo.para(t)), [tempos, eixo]);
  const pausas: Pausa[] = useMemo(
    () => eixo.pausas.map((p) => ({ x0: p.x0, x1: p.x1, rotulo: textoPausa(p.duracaoMs) })),
    [eixo],
  );
  const formatarT = useMemo(() => formatadorDeTempoCompleto(analise), [analise]);

  const marcadoresVel: Marcador[] = useMemo(
    () =>
      analise.eventos
        .filter((e) => e.tipo !== "marcha_lenta_longa")
        .map((e) => ({
          x: eixo.para(e.t_inicio_ms),
          y: e.velocidade_inicial_kmh ?? 0,
          forma: e.tipo === "frenagem_brusca" ? ("tri-baixo" as const) : ("tri-cima" as const),
        })),
    [analise.eventos, eixo],
  );
  const marcadoresRpm: Marcador[] = useMemo(
    () =>
      analise.marchas.trocas.map((t) => ({
        x: eixo.para(t.t_ms),
        y: t.rpm_antes,
        forma: t.direcao === "subida" ? ("circulo" as const) : ("circulo-vazio" as const),
      })),
    [analise.marchas.trocas, eixo],
  );

  const maxMarcha = Math.max(0, ...s.marcha.map((m) => m ?? 0));
  const ticksMarcha = Array.from({ length: maxMarcha + 1 }, (_, i) => i);

  const instante = cursor.t === null ? null : instanteMaisProximo(analise, tempos, cursor.t);
  const cursorX = cursor.t === null ? null : eixo.para(cursor.t);
  const aplicar = cursor.origem !== "grafico";
  const aoMover = (xv: number | null) => onCursor(xv === null ? null : eixo.de(xv), "grafico");

  const comum = {
    x,
    formatarX,
    pausas,
    sync: SYNC,
    cursorX,
    aplicarCursor: aplicar,
    onCursorX: aoMover,
  };

  return (
    <div className="linha-do-tempo">
      <TooltipInstante instante={instante} formatarT={formatarT} />
      <div className="painel-grafico">
        <h3 className="titulo-grafico">
          Velocidade <span className="unidade">km/h</span>
        </h3>
        <Grafico
          {...comum}
          titulo="Velocidade ao longo da viagem"
          series={[{ rotulo: "Velocidade", valores: s.velocidade_kmh, cor: "--text-primary" }]}
          marcadores={marcadoresVel}
          formatarY={rotuloY}
          semLeituras={!algumValor(s.velocidade_kmh)}
        />
        <p className="legenda-marcadores">
          <span aria-hidden="true">▼</span> Frenagem brusca · <span aria-hidden="true">▲</span>{" "}
          Aceleração brusca
        </p>
      </div>
      <div className="painel-grafico">
        <h3 className="titulo-grafico">
          Rotação <span className="unidade">rpm</span>
        </h3>
        <Grafico
          {...comum}
          titulo="Rotação do motor ao longo da viagem"
          series={[{ rotulo: "Rotação", valores: s.rpm, cor: "--accent" }]}
          marcadores={marcadoresRpm}
          formatarY={rotuloY}
          semLeituras={!algumValor(s.rpm)}
        />
        <p className="legenda-marcadores">
          <span aria-hidden="true">●</span> Subida de marcha · <span aria-hidden="true">○</span>{" "}
          Redução de marcha
        </p>
      </div>
      <div className="painel-grafico">
        <h3 className="titulo-grafico">Marcha estimada</h3>
        <Grafico
          {...comum}
          titulo="Marcha estimada ao longo da viagem"
          series={[{ rotulo: "Marcha", valores: s.marcha, cor: "--text-secondary", passo: true }]}
          altura={120}
          ticksY={ticksMarcha}
          faixaY={[-0.5, maxMarcha + 0.5]}
          formatarY={(v) => (v === 0 ? "N/E" : `${v}ª`)}
          semLeituras={!algumValor(s.marcha)}
        />
        <p className="legenda-marcadores">
          N/E: ponto morto ou embreagem. Marcha indefinida fica vazia.
        </p>
      </div>
      <div className="painel-grafico">
        <h3 className="titulo-grafico">
          Pedal <span className="unidade">%</span>
        </h3>
        <Grafico
          {...comum}
          titulo="Posição do pedal ao longo da viagem"
          series={[{ rotulo: "Pedal", valores: s.pedal_pct, cor: "--text-secondary" }]}
          altura={120}
          faixaY={[0, 100]}
          formatarY={rotuloY}
          semLeituras={!algumValor(s.pedal_pct)}
        />
      </div>
    </div>
  );
}
