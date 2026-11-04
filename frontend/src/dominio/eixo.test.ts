import { describe, expect, it } from "vitest";
import { criarEixo } from "./eixo";

const HORA = 3_600_000;

describe("criarEixo", () => {
  it("é a identidade deslocada com uma parte só", () => {
    const e = criarEixo([{ inicio: 1000, fim: 5000 }]);
    expect(e.para(1000)).toBe(0);
    expect(e.para(3000)).toBe(2000);
    expect(e.de(2000)).toBe(3000);
    expect(e.pausas).toHaveLength(0);
  });

  it("comprime uma pausa de 5 h a 5% da soma das durações", () => {
    const e = criarEixo([
      { inicio: 0, fim: HORA },
      { inicio: 6 * HORA, fim: 7 * HORA },
    ]);
    expect(e.pausas).toHaveLength(1);
    expect(e.pausas[0].duracaoMs).toBe(5 * HORA);
    const largura = e.pausas[0].x1 - e.pausas[0].x0;
    expect(largura).toBeCloseTo(0.05 * 2 * HORA);
    const parcela = largura / (e.xMax - e.xMin);
    expect(parcela).toBeGreaterThan(0.045);
    expect(parcela).toBeLessThan(0.055);
  });

  it("tem inversa exata dos dois lados da pausa", () => {
    const e = criarEixo([
      { inicio: 0, fim: HORA },
      { inicio: 6 * HORA, fim: 7 * HORA },
    ]);
    for (const t of [0, 1234, HORA, 6 * HORA, 6 * HORA + 999, 7 * HORA, 3 * HORA]) {
      expect(e.de(e.para(t))).toBeCloseTo(t, 3);
    }
    expect(e.de(e.para(HORA - 1000))).toBeCloseTo(HORA - 1000);
    expect(e.de(e.para(6 * HORA + 1000))).toBeCloseTo(6 * HORA + 1000);
  });

  it("é monotônico e extrapola fora dos limites", () => {
    const e = criarEixo([
      { inicio: 100, fim: 200 },
      { inicio: 1000, fim: 1100 },
    ]);
    expect(e.para(50)).toBe(-50);
    expect(e.de(e.para(50))).toBe(50);
    expect(e.para(5000)).toBeGreaterThan(e.para(1100));
    expect(e.de(e.xMax + 10)).toBe(1110);
  });

  it("não alarga pausa menor que o limite", () => {
    const e = criarEixo([
      { inicio: 0, fim: 100_000 },
      { inicio: 101_000, fim: 200_000 },
    ]);
    expect(e.pausas[0].x1 - e.pausas[0].x0).toBe(1000);
  });

  it("aceita partes fora de ordem, encostadas e lista vazia", () => {
    const e = criarEixo([
      { inicio: 500, fim: 900 },
      { inicio: 0, fim: 500 },
    ]);
    expect(e.pausas).toHaveLength(0);
    expect(e.para(500)).toBe(500);
    expect(criarEixo([]).para(0)).toBe(0);
  });
});
