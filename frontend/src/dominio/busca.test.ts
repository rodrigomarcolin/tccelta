import { describe, expect, it } from "vitest";
import type { Rota } from "@/contrato/tipos";
import {
  indiceMaisProximo,
  pontoDaRotaNoLugar,
  pontoDaRotaNoTempo,
  preencherTempos,
} from "./busca";

const rota: Rota = {
  t_ms: [0, 1000, 2000, 10000],
  lat: [-25.4, -25.401, -25.402, -25.41],
  lon: [-49.27, -49.271, -49.272, -49.28],
  segmentos: { velocidade_kmh: [10, 10, null], faixa: ["ate_30", "ate_30", "falha_gps"] },
};

describe("preencherTempos", () => {
  it("troca separadores null por valor logo após o anterior", () => {
    expect(preencherTempos([10, 20, null, 50])).toEqual([10, 20, 21, 50]);
    expect(preencherTempos([null, 5])).toEqual([1, 5]);
  });
});

describe("indiceMaisProximo", () => {
  const ts = [0, 100, 200, 300];
  it("acha o vizinho mais próximo", () => {
    expect(indiceMaisProximo(ts, 140)).toBe(1);
    expect(indiceMaisProximo(ts, 160)).toBe(2);
    expect(indiceMaisProximo(ts, -50)).toBe(0);
    expect(indiceMaisProximo(ts, 999)).toBe(3);
  });
  it("devolve -1 sem dados", () => {
    expect(indiceMaisProximo([], 1)).toBe(-1);
  });
});

describe("pontoDaRotaNoTempo", () => {
  it("localiza o ponto mais próximo no tempo", () => {
    expect(pontoDaRotaNoTempo(rota, 1100)?.indice).toBe(1);
  });
  it("devolve null sem GPS", () => {
    expect(pontoDaRotaNoTempo(null, 1)).toBeNull();
  });
  it("rejeita instante a mais de 5 s do ponto", () => {
    const r: Rota = { ...rota, t_ms: [0, 20000], lat: [1, 2], lon: [1, 2] };
    expect(pontoDaRotaNoTempo(r, 10000)).toBeNull();
  });
  it("trata coordenada null", () => {
    const r: Rota = { ...rota, lat: [null, null, null, null], lon: [null, null, null, null] };
    expect(pontoDaRotaNoTempo(r, 0)).toBeNull();
  });
});

describe("pontoDaRotaNoLugar", () => {
  it("acha o ponto próximo e respeita a distância máxima", () => {
    expect(pontoDaRotaNoLugar(rota, -25.4011, -49.2711, 50)?.indice).toBe(1);
    expect(pontoDaRotaNoLugar(rota, -25.5, -49.5, 50)).toBeNull();
    expect(pontoDaRotaNoLugar(null, 0, 0, 10)).toBeNull();
  });
  it("ignora pontos sem coordenada", () => {
    const r: Rota = { ...rota, lat: [null, -25.401, null, null], lon: [null, -49.271, null, null] };
    expect(pontoDaRotaNoLugar(r, -25.401, -49.271, 10)?.indice).toBe(1);
  });
});
