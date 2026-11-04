/** Cálculo de contraste WCAG entre duas cores hexadecimais (#rrggbb). */

function canal(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

export function luminancia(hex: string): number {
  const n = Number.parseInt(hex.replace("#", ""), 16);
  const r = (n >> 16) & 255;
  const g = (n >> 8) & 255;
  const b = n & 255;
  return 0.2126 * canal(r) + 0.7152 * canal(g) + 0.0722 * canal(b);
}

export function contraste(a: string, b: string): number {
  const la = luminancia(a);
  const lb = luminancia(b);
  const [alto, baixo] = la >= lb ? [la, lb] : [lb, la];
  return (alto + 0.05) / (baixo + 0.05);
}

/** Mistura `frente` com opacidade `alfa` sobre `fundo`. */
export function misturar(frente: string, fundo: string, alfa: number): string {
  const f = Number.parseInt(frente.replace("#", ""), 16);
  const g = Number.parseInt(fundo.replace("#", ""), 16);
  const canalMix = (s: number) =>
    Math.round(((f >> s) & 255) * alfa + ((g >> s) & 255) * (1 - alfa));
  return `#${[16, 8, 0].map((s) => canalMix(s).toString(16).padStart(2, "0")).join("")}`;
}
