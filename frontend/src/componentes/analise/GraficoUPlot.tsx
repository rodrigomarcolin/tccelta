"use client";

import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import { useTema } from "@/componentes/ui/Tema";
import { corCss } from "./cores";

export type SerieGrafico = {
  rotulo: string;
  valores: (number | null)[];
  /** Nome de variável CSS (`--accent`) ou cor hexadecimal. */
  cor: string;
  passo?: boolean;
  tracejada?: boolean;
};

export type Marcador = {
  x: number;
  y: number;
  forma: "tri-baixo" | "tri-cima" | "circulo" | "circulo-vazio";
  rotulo?: string;
};

export type Pausa = { x0: number; x1: number; rotulo: string };

export type PropsGrafico = {
  titulo: string;
  x: number[];
  series: SerieGrafico[];
  altura?: number;
  formatarX: (x: number) => string;
  formatarY?: (v: number) => string;
  ticksY?: number[];
  faixaY?: [number, number];
  /** Linha horizontal de referência (por exemplo, motor aquecido). */
  referenciaY?: { y: number; rotulo: string };
  marcadores?: Marcador[];
  pausas?: Pausa[];
  /** Chave de sincronização do cursor entre gráficos. */
  sync?: string;
  /** Posição X do cursor vinda de fora do gráfico. */
  cursorX?: number | null;
  aplicarCursor?: boolean;
  onCursorX?: (x: number | null) => void;
  semLeituras?: boolean;
};

function resolver(cor: string): string {
  return cor.startsWith("--") ? corCss(cor) : cor;
}

function desenharMarcador(
  ctx: CanvasRenderingContext2D,
  m: Marcador,
  px: number,
  py: number,
  r: number,
) {
  const contorno = corCss("--text-primary");
  const preenche = corCss("--surface-card");
  ctx.save();
  ctx.lineWidth = Math.max(1, r / 4);
  ctx.strokeStyle = contorno;
  ctx.fillStyle = m.forma === "circulo" ? contorno : preenche;
  ctx.beginPath();
  if (m.forma === "tri-baixo") {
    ctx.moveTo(px - r, py - r);
    ctx.lineTo(px + r, py - r);
    ctx.lineTo(px, py + r);
  } else if (m.forma === "tri-cima") {
    ctx.moveTo(px - r, py + r);
    ctx.lineTo(px + r, py + r);
    ctx.lineTo(px, py - r);
  } else {
    ctx.arc(px, py, r * 0.8, 0, Math.PI * 2);
  }
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
  ctx.restore();
}

/** Gráfico de linha (uPlot) com `null` como lacuna, cursor sincronizado e marcadores. */
export default function GraficoUPlot(props: PropsGrafico) {
  const { titulo, x, series, altura = 150, semLeituras } = props;
  const tema = useTema();
  const raiz = useRef<HTMLDivElement>(null);
  const plot = useRef<uPlot | null>(null);
  const aplicando = useRef(false);
  const cbCursor = useRef(props.onCursorX);
  useEffect(() => {
    cbCursor.current = props.onCursorX;
  });

  useEffect(() => {
    const el = raiz.current;
    if (!el || semLeituras || x.length === 0) return;
    const fonte = '11px "JetBrains Mono", ui-monospace, monospace';
    const secundario = corCss("--text-secondary");
    const grade = corCss("--border-hairline");
    const formatY = props.formatarY ?? ((v: number) => String(v));

    const opts: uPlot.Options = {
      width: el.clientWidth || 600,
      height: altura,
      legend: { show: false },
      padding: [10, 8, 0, 0],
      scales: {
        x: { time: false },
        y: props.faixaY ? { range: () => props.faixaY as [number, number] } : {},
      },
      axes: [
        {
          stroke: secundario,
          font: fonte,
          grid: { stroke: grade, width: 1 },
          ticks: { stroke: grade, width: 1 },
          values: (_u, splits) => splits.map((s) => props.formatarX(s)),
        },
        {
          stroke: secundario,
          font: fonte,
          size: 58,
          grid: { stroke: grade, width: 1 },
          ticks: { stroke: grade, width: 1 },
          ...(props.ticksY ? { splits: () => props.ticksY as number[] } : {}),
          values: (_u, splits) => splits.map((s) => formatY(s)),
        },
      ],
      series: [
        {},
        ...series.map((s) => ({
          label: s.rotulo,
          stroke: resolver(s.cor),
          width: 1.6,
          spanGaps: false,
          points: { show: false },
          ...(s.tracejada ? { dash: [4, 4] } : {}),
          ...(s.passo ? { paths: uPlot.paths.stepped?.({ align: 1 }) } : {}),
        })),
      ],
      cursor: {
        drag: { x: false, y: false },
        points: { size: 7, stroke: corCss("--accent"), fill: corCss("--surface-card") },
        ...(props.sync ? { sync: { key: props.sync } } : {}),
      },
      hooks: {
        setCursor: [
          (u) => {
            if (aplicando.current) return;
            const i = u.cursor.idx;
            const cb = cbCursor.current;
            if (!cb) return;
            const left = u.cursor.left ?? -1;
            cb(i === null || i === undefined || left < 0 ? null : (u.data[0][i] as number));
          },
        ],
        draw: [
          (u) => {
            const ctx = u.ctx;
            const dpr = devicePixelRatio || 1;
            const area = u.bbox;
            for (const p of props.pausas ?? []) {
              const a = u.valToPos(p.x0, "x", true);
              const b = u.valToPos(p.x1, "x", true);
              ctx.save();
              ctx.fillStyle = corCss("--surface-sunken");
              ctx.fillRect(a, area.top, Math.max(1, b - a), area.height);
              ctx.fillStyle = corCss("--text-secondary");
              ctx.font = `${10 * dpr}px "JetBrains Mono", monospace`;
              ctx.textAlign = "center";
              ctx.fillText(
                p.rotulo,
                (a + b) / 2,
                area.top + 14 * dpr,
                Math.max(60 * dpr, b - a + 40 * dpr),
              );
              ctx.restore();
            }
            if (props.referenciaY) {
              const yy = u.valToPos(props.referenciaY.y, "y", true);
              ctx.save();
              ctx.strokeStyle = corCss("--text-tertiary");
              ctx.setLineDash([6 * dpr, 4 * dpr]);
              ctx.beginPath();
              ctx.moveTo(area.left, yy);
              ctx.lineTo(area.left + area.width, yy);
              ctx.stroke();
              ctx.fillStyle = corCss("--text-secondary");
              ctx.font = `${10 * dpr}px "JetBrains Mono", monospace`;
              ctx.fillText(props.referenciaY.rotulo, area.left + 4 * dpr, yy - 4 * dpr);
              ctx.restore();
            }
            for (const m of props.marcadores ?? []) {
              desenharMarcador(
                ctx,
                m,
                u.valToPos(m.x, "x", true),
                u.valToPos(m.y, "y", true),
                5 * dpr,
              );
            }
          },
        ],
      },
    };

    const u = new uPlot(opts, [x, ...series.map((s) => s.valores)] as uPlot.AlignedData, el);
    plot.current = u;
    const ro = new ResizeObserver(() =>
      u.setSize({ width: el.clientWidth || 600, height: altura }),
    );
    ro.observe(el);
    return () => {
      ro.disconnect();
      u.destroy();
      plot.current = null;
    };
    // O gráfico é recriado quando os dados, o tema ou a altura mudam.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [x, series, tema, altura, semLeituras, props.marcadores, props.pausas, props.referenciaY]);

  useEffect(() => {
    const u = plot.current;
    if (!u || !props.aplicarCursor) return;
    aplicando.current = true;
    if (props.cursorX === null || props.cursorX === undefined) {
      u.setCursor({ left: -10, top: -10 });
    } else {
      u.setCursor({ left: Math.round(u.valToPos(props.cursorX, "x")), top: 10 });
    }
    aplicando.current = false;
  }, [props.cursorX, props.aplicarCursor]);

  if (semLeituras) {
    return (
      <div
        className="grafico grafico-vazio"
        role="img"
        aria-label={`${titulo}: sem leituras`}
        style={{ height: altura }}
      >
        Sem leituras
      </div>
    );
  }
  return <div className="grafico" ref={raiz} role="img" aria-label={titulo} />;
}
