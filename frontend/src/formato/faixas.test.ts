import { describe, expect, it } from "vitest";
import { faixaDeVelocidade, ORDEM_FAIXAS, rotuloFaixa, ROTULO_FAIXA } from "./faixas";

describe("faixaDeVelocidade", () => {
  it.each([
    [2.9, "parado"],
    [3, "ate_30"],
    [29.9, "ate_30"],
    [30, "30_60"],
    [59.9, "30_60"],
    [60.0, "60_80"],
    [79.9, "60_80"],
    [80, "80_110"],
    [109.9, "80_110"],
    [110.0, "acima_110"],
    [200, "acima_110"],
    [0, "parado"],
  ])("%s km/h cai em %s", (v, esperado) => {
    expect(faixaDeVelocidade(v)).toBe(esperado);
  });
});

describe("rótulos", () => {
  it("cobre todas as faixas", () => {
    for (const f of ORDEM_FAIXAS) expect(ROTULO_FAIXA[f]).toBeTruthy();
  });
  it("rotula segmentos sem dado", () => {
    expect(rotuloFaixa("falha_gps")).toBe("Falha de GPS");
    expect(rotuloFaixa("sem_velocidade")).toBe("Sem velocidade");
    expect(rotuloFaixa("60_80")).toBe("60–80 km/h");
  });
});
