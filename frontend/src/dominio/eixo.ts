import { FRACAO_PAUSA_EIXO } from "@/config";

export type Intervalo = { inicio: number; fim: number };

type Trecho = { t0: number; t1: number; x0: number; x1: number; pausa: boolean };

export type Eixo = {
  /** Tempo real (ms) para coordenada do eixo. Linear por trechos. */
  para(t: number): number;
  /** Inversa de `para`. */
  de(x: number): number;
  xMin: number;
  xMax: number;
  trechos: Trecho[];
  /** Pausas comprimidas, com duração real. */
  pausas: { x0: number; x1: number; duracaoMs: number }[];
};

/**
 * Eixo X com cada pausa entre partes comprimida a `fracao` da soma das
 * durações das partes. A transformação é linear por trechos e invertível.
 */
export function criarEixo(intervalos: Intervalo[], fracao = FRACAO_PAUSA_EIXO): Eixo {
  const ordenados = [...intervalos].sort((a, b) => a.inicio - b.inicio);
  const soma = ordenados.reduce((s, i) => s + Math.max(0, i.fim - i.inicio), 0);
  const trechos: Trecho[] = [];
  const pausas: Eixo["pausas"] = [];
  let x = 0;
  ordenados.forEach((iv, i) => {
    const anterior = ordenados[i - 1];
    if (anterior) {
      const real = iv.inicio - anterior.fim;
      if (real > 0) {
        const mostrado = Math.min(real, fracao * soma);
        trechos.push({ t0: anterior.fim, t1: iv.inicio, x0: x, x1: x + mostrado, pausa: true });
        pausas.push({ x0: x, x1: x + mostrado, duracaoMs: real });
        x += mostrado;
      }
    }
    const len = Math.max(0, iv.fim - iv.inicio);
    trechos.push({ t0: iv.inicio, t1: iv.fim, x0: x, x1: x + len, pausa: false });
    x += len;
  });
  if (trechos.length === 0) trechos.push({ t0: 0, t1: 1, x0: 0, x1: 1, pausa: false });

  const primeiro = trechos[0]!;
  const ultimo = trechos[trechos.length - 1]!;

  const para = (t: number): number => {
    if (t <= primeiro.t0) return primeiro.x0 + (t - primeiro.t0);
    if (t >= ultimo.t1) return ultimo.x1 + (t - ultimo.t1);
    for (const tr of trechos) {
      if (t <= tr.t1) {
        const dt = tr.t1 - tr.t0;
        return dt === 0 ? tr.x0 : tr.x0 + ((t - tr.t0) / dt) * (tr.x1 - tr.x0);
      }
    }
    return ultimo.x1;
  };

  const de = (xv: number): number => {
    if (xv <= primeiro.x0) return primeiro.t0 + (xv - primeiro.x0);
    if (xv >= ultimo.x1) return ultimo.t1 + (xv - ultimo.x1);
    for (const tr of trechos) {
      if (xv <= tr.x1) {
        const dx = tr.x1 - tr.x0;
        return dx === 0 ? tr.t0 : tr.t0 + ((xv - tr.x0) / dx) * (tr.t1 - tr.t0);
      }
    }
    return ultimo.t1;
  };

  return { para, de, xMin: primeiro.x0, xMax: ultimo.x1, trechos, pausas };
}
