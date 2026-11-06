/** WCAG contrast calculation between two hexadecimal colors (#rrggbb). */

function channel(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

export function luminance(hex: string): number {
  const n = Number.parseInt(hex.replace("#", ""), 16);
  const r = (n >> 16) & 255;
  const g = (n >> 8) & 255;
  const b = n & 255;
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

export function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  const [high, low] = la >= lb ? [la, lb] : [lb, la];
  return (high + 0.05) / (low + 0.05);
}

/** Blends `front` with opacity `alpha` over `back`. */
export function mix(front: string, back: string, alpha: number): string {
  const f = Number.parseInt(front.replace("#", ""), 16);
  const g = Number.parseInt(back.replace("#", ""), 16);
  const channelMix = (s: number) =>
    Math.round(((f >> s) & 255) * alpha + ((g >> s) & 255) * (1 - alpha));
  return `#${[16, 8, 0].map((s) => channelMix(s).toString(16).padStart(2, "0")).join("")}`;
}
