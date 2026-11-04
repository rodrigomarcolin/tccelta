import { describe, expect, it } from "vitest";
import { analiseDe } from "@/testes/helpers";
import {
  algumValor,
  descricaoCabecalho,
  eixoDaAnalise,
  formatadorDeTempo,
  formatadorDeTempoCompleto,
  instanteMaisProximo,
  rotuloMarcha,
  temLeituras,
  temposDaSerie,
  textoPausa,
} from "./analise";

describe("descricaoCabecalho (critério 11)", () => {
  it("boot: Dongle 1 · boot 8 com data e hora", () => {
    const d = descricaoCabecalho(analiseDe("normal"));
    expect(d.identificacao).toBe("Dongle 1 · boot 8");
    expect(d.quando).toBe("28/10/2026 14:03–14:11");
  });
  it("intervalo: intervalo pedido e número de partes", () => {
    const d = descricaoCabecalho(analiseDe("intervalo"));
    expect(d.identificacao).toBe("Dongle 1 · 28/10 08:00–18:00 · 2 partes");
    expect(d.quando).toBeNull();
  });
  it("intervalo em dias diferentes e com uma parte só", () => {
    const a = analiseDe("intervalo");
    if (a.viagem.entrada.tipo === "intervalo") {
      a.viagem.entrada.t1 = "2026-10-29T21:00:00.000Z";
    }
    a.viagem.partes = a.viagem.partes.slice(0, 1);
    expect(descricaoCabecalho(a).identificacao).toBe("Dongle 1 · 28/10 08:00–29/10 18:00");
  });
  it("boot sem hora absoluta omite data e hora", () => {
    const d = descricaoCabecalho(analiseDe("sem_hora", false, 11));
    expect(d).toEqual({ identificacao: "Dongle 1 · boot 11", quando: null });
  });
});

describe("eixo e formatadores", () => {
  it("hora absoluta com relógio; m:ss sem relógio", () => {
    const a = analiseDe("normal");
    const ini = a.viagem.inicio_t_ms!;
    expect(formatadorDeTempo(a)(ini)).toBe("14:03");
    expect(formatadorDeTempoCompleto(a)(ini)).toBe("14:03:22");
    const s = analiseDe("sem_hora", false, 11);
    expect(formatadorDeTempo(s)(s.viagem.inicio_t_ms! + 65_000)).toBe("1:05");
    expect(formatadorDeTempoCompleto(s)(s.viagem.inicio_t_ms! + 5_000)).toBe("0:05");
  });

  it("o eixo comprime a pausa entre as partes do intervalo e o cursor volta a hora certa", () => {
    const a = analiseDe("intervalo");
    const e = eixoDaAnalise(a);
    expect(e.pausas).toHaveLength(1);
    const [p1, p2] = a.viagem.partes;
    expect(e.pausas[0].duracaoMs).toBe(p2.inicio_t_ms - p1.fim_t_ms);
    expect(e.de(e.para(p2.inicio_t_ms + 30_000))).toBeCloseTo(p2.inicio_t_ms + 30_000);
    const fmt = formatadorDeTempoCompleto(a);
    expect(fmt(e.de(e.para(p1.inicio_t_ms)))).toBe(fmt(p1.inicio_t_ms));
  });

  it("textoPausa mantém a duração real", () => {
    expect(textoPausa(5 * 3600 * 1000 + 120_000)).toBe("Sem dados · 5 h 02 min");
  });
});

describe("instante mais próximo", () => {
  it("devolve as leituras do ponto da grade; null onde falta dado", () => {
    const a = analiseDe("normal");
    const tempos = temposDaSerie(a);
    const i = instanteMaisProximo(a, tempos, tempos[100] + 10)!;
    expect(i.t).toBe(tempos[100]);
    expect(i.velocidade).toBe(a.series.velocidade_kmh[100]);
    const b = analiseDe("intervalo");
    const sep = b.series.rpm.indexOf(null);
    const t = temposDaSerie(b);
    const r = instanteMaisProximo(b, t, t[sep])!;
    expect(r).toMatchObject({ velocidade: null, rpm: null, marcha: null, pedal: null });
    expect(instanteMaisProximo(analiseDe("sem_leituras"), [], 0)).toBeNull();
  });
});

describe("auxiliares", () => {
  it("rotuloMarcha", () => {
    expect(rotuloMarcha(null)).toBe("—");
    expect(rotuloMarcha(0)).toBe("Ponto morto ou embreagem");
    expect(rotuloMarcha(3)).toBe("3ª");
  });
  it("temLeituras e algumValor", () => {
    expect(temLeituras(analiseDe("normal"))).toBe(true);
    expect(temLeituras(analiseDe("sem_leituras"))).toBe(false);
    expect(algumValor([null, null])).toBe(false);
    expect(algumValor([null, 0])).toBe(true);
  });
});
