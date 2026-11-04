import type { ReactNode } from "react";
import { duracao, numero } from "@/formato/numeros";

/* Gráficos próprios em HTML e SVG: poucos elementos, sem biblioteca. */

export type ItemBarra = {
  rotulo: string;
  pct: number | null;
  tempoS: number | null;
  /** Variável CSS ou cor; padrão neutro. */
  cor?: string;
  nota?: string;
};

/** Barras horizontais com % e tempo. */
export function BarrasHorizontais({ itens, titulo }: { itens: ItemBarra[]; titulo: string }) {
  const max = Math.max(1, ...itens.map((i) => i.pct ?? 0));
  return (
    <ul className="barras" aria-label={titulo}>
      {itens.map((i) => {
        const cor = i.cor
          ? i.cor.startsWith("--")
            ? `var(${i.cor})`
            : i.cor
          : "var(--text-secondary)";
        return (
          <li key={i.rotulo} className="barra-linha">
            <span className="barra-rotulo">{i.rotulo}</span>
            <span className="barra-trilha" aria-hidden="true">
              <span
                className="barra-preenchida"
                style={{ width: `${((i.pct ?? 0) / max) * 100}%`, background: cor }}
              />
            </span>
            <span className="dado barra-valor">
              {i.pct === null ? "—" : `${numero(i.pct, 0)}%`} · {duracao(i.tempoS)}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

export type PontoDispersao = {
  x: number;
  y: number;
  cor: string;
  rotulo?: string;
  onClick?: () => void;
};

export type LinhaDispersao = {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  cor: string;
  rotulo?: string;
};

type PropsDispersao = {
  titulo: string;
  pontos: PontoDispersao[];
  linhas?: LinhaDispersao[];
  /** Traços horizontais (por exemplo, a mediana de cada tipo). */
  tracos?: { x1: number; x2: number; y: number; rotulo?: string }[];
  dominioX: [number, number];
  dominioY: [number, number];
  ticksX?: { v: number; rotulo: string }[];
  ticksY?: { v: number; rotulo: string }[];
  rotuloX: string;
  rotuloY: string;
  largura?: number;
  altura?: number;
  raio?: number;
};

const MARGEM = { e: 54, d: 16, t: 10, b: 38 };

function cssCor(c: string): string {
  return c.startsWith("--") ? `var(${c})` : c;
}

/** Dispersão em SVG, com linhas e traços opcionais. */
export function Dispersao(p: PropsDispersao) {
  const L = p.largura ?? 520;
  const A = p.altura ?? 300;
  const w = L - MARGEM.e - MARGEM.d;
  const h = A - MARGEM.t - MARGEM.b;
  const [x0, x1] = p.dominioX;
  const [y0, y1] = p.dominioY;
  const px = (v: number) => MARGEM.e + ((v - x0) / (x1 - x0 || 1)) * w;
  const py = (v: number) => MARGEM.t + h - ((v - y0) / (y1 - y0 || 1)) * h;
  const ticksX = p.ticksX ?? [];
  const ticksY = p.ticksY ?? [];
  return (
    <svg className="dispersao" viewBox={`0 0 ${L} ${A}`} role="group" aria-label={p.titulo}>
      <title>{p.titulo}</title>
      {ticksY.map((t) => (
        <g key={`y${t.v}`}>
          <line x1={MARGEM.e} x2={MARGEM.e + w} y1={py(t.v)} y2={py(t.v)} className="grade" />
          <text x={MARGEM.e - 6} y={py(t.v) + 4} textAnchor="end" className="eixo-texto">
            {t.rotulo}
          </text>
        </g>
      ))}
      {ticksX.map((t) => (
        <g key={`x${t.v}`}>
          <line x1={px(t.v)} x2={px(t.v)} y1={MARGEM.t} y2={MARGEM.t + h} className="grade" />
          <text x={px(t.v)} y={MARGEM.t + h + 14} textAnchor="middle" className="eixo-texto">
            {t.rotulo}
          </text>
        </g>
      ))}
      <text x={MARGEM.e + w / 2} y={A - 4} textAnchor="middle" className="eixo-titulo">
        {p.rotuloX}
      </text>
      <text
        x={12}
        y={MARGEM.t + h / 2}
        textAnchor="middle"
        className="eixo-titulo"
        transform={`rotate(-90 12 ${MARGEM.t + h / 2})`}
      >
        {p.rotuloY}
      </text>
      {(p.linhas ?? []).map((l, i) => (
        <g key={`l${i}`}>
          <line
            x1={px(l.x1)}
            y1={py(l.y1)}
            x2={px(l.x2)}
            y2={py(l.y2)}
            stroke={cssCor(l.cor)}
            strokeWidth={1.2}
            strokeDasharray="4 3"
          />
          {l.rotulo ? (
            <text
              x={px(l.x2) - 4}
              y={py(l.y2) - 4}
              textAnchor="end"
              className="eixo-texto"
              fill={cssCor(l.cor)}
              style={{ fill: cssCor(l.cor) }}
            >
              {l.rotulo}
            </text>
          ) : null}
        </g>
      ))}
      {(p.tracos ?? []).map((t, i) => (
        <g key={`t${i}`}>
          <line x1={px(t.x1)} x2={px(t.x2)} y1={py(t.y)} y2={py(t.y)} className="traco-mediana" />
          {t.rotulo ? (
            <text x={px(t.x2) + 3} y={py(t.y) + 4} className="eixo-texto">
              {t.rotulo}
            </text>
          ) : null}
        </g>
      ))}
      {p.pontos.map((pt, i) => {
        const comum = {
          cx: px(pt.x),
          cy: py(pt.y),
          r: p.raio ?? 2.6,
          fill: cssCor(pt.cor),
          opacity: 0.85,
        };
        if (!pt.onClick) return <circle key={i} {...comum} />;
        return (
          <circle
            key={i}
            {...comum}
            r={5}
            className="ponto-clicavel"
            role="button"
            tabIndex={0}
            aria-label={pt.rotulo}
            onClick={pt.onClick}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                pt.onClick?.();
              }
            }}
          >
            <title>{pt.rotulo}</title>
          </circle>
        );
      })}
    </svg>
  );
}

type PropsCalor = {
  /** Linhas = faixas de rotação; colunas = faixas de pedal. */
  tempoS: number[][];
  rpmBin: number;
  pedalBin: number;
};

/** Mapa de calor rotação × pedal, em um só matiz. */
export function MapaDeCalor({ tempoS, rpmBin, pedalBin }: PropsCalor): ReactNode {
  const linhas = tempoS.length;
  const colunas = Math.max(...tempoS.map((l) => l.length), 0);
  const max = Math.max(0, ...tempoS.flat());
  if (linhas === 0 || max === 0) return <p className="nota">Sem leituras</p>;
  // Só mostra do primeiro ao último intervalo de rotação com tempo.
  const comTempo = tempoS.map((l) => l.some((v) => v > 0));
  const primeira = comTempo.indexOf(true);
  const ultima = comTempo.lastIndexOf(true);
  const mostradas = ultima - primeira + 1;
  const C = 34;
  const L = 24;
  const ml = 56;
  const mb = 34;
  const largura = ml + colunas * C + 8;
  const altura = mostradas * L + mb + 6;
  return (
    <figure className="calor">
      <svg
        viewBox={`0 0 ${largura} ${altura}`}
        role="img"
        aria-label="Mapa de calor do tempo por rotação e posição do pedal"
      >
        {Array.from({ length: mostradas }, (_, k) => {
          const i = ultima - k; // maior rotação em cima
          const linha = tempoS[i] ?? [];
          return (
            <g key={i}>
              <text x={ml - 6} y={k * L + L / 2 + 4} textAnchor="end" className="eixo-texto">
                {i * rpmBin}
              </text>
              {Array.from({ length: colunas }, (_, j) => {
                const v = linha[j] ?? 0;
                const alfa = v === 0 ? 0 : 0.12 + 0.88 * (v / max);
                return (
                  <rect
                    key={j}
                    x={ml + j * C}
                    y={k * L}
                    width={C - 2}
                    height={L - 2}
                    rx={3}
                    style={{ fill: `rgba(var(--calor), ${alfa.toFixed(3)})` }}
                    className="celula-calor"
                  >
                    <title>{`${i * rpmBin}–${(i + 1) * rpmBin} rpm, pedal ${j * pedalBin}–${(j + 1) * pedalBin}%: ${duracao(v)}`}</title>
                  </rect>
                );
              })}
            </g>
          );
        })}
        {Array.from({ length: colunas }, (_, j) => (
          <text
            key={j}
            x={ml + j * C + (C - 2) / 2}
            y={mostradas * L + 14}
            textAnchor="middle"
            className="eixo-texto"
          >
            {j * pedalBin}
          </text>
        ))}
        <text
          x={ml + (colunas * C) / 2}
          y={mostradas * L + 30}
          textAnchor="middle"
          className="eixo-titulo"
        >
          Pedal (%)
        </text>
      </svg>
      <figcaption className="nota">
        Linhas: rotação (rpm, início de cada faixa). Cor: tempo, de 0 s a{" "}
        <span className="dado">{duracao(max)}</span>.
      </figcaption>
      <div className="escala-calor" aria-hidden="true">
        <span>0 s</span>
        <span className="escala-calor-barra" />
        <span>{duracao(max)}</span>
      </div>
    </figure>
  );
}
