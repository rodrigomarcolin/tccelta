import { describe, expect, it } from "vitest";
import {
  dataCompleta,
  diaMes,
  duracao,
  hora,
  horaCompleta,
  minutosSegundos,
  numero,
  porcento,
  TRACO,
} from "./numeros";

describe("numero", () => {
  it("usa o padrão pt-BR", () => {
    expect(numero(3.5, 1)).toBe("3,5");
    expect(numero(1240, 0)).toBe("1.240");
    expect(numero(0.83, 2)).toBe("0,83");
  });
  it("mostra traço para null, undefined e não finito", () => {
    expect(numero(null)).toBe(TRACO);
    expect(numero(undefined)).toBe(TRACO);
    expect(numero(Number.NaN)).toBe(TRACO);
  });
  it("não mostra zero negativo", () => {
    expect(numero(-0.001, 1)).toBe("0,0");
  });
  it("mantém zero real", () => {
    expect(numero(0, 0)).toBe("0");
  });
});

describe("porcento", () => {
  it("formata e trata null", () => {
    expect(porcento(10, 0)).toBe("10%");
    expect(porcento(null)).toBe(TRACO);
  });
});

describe("duracao", () => {
  it("formata minutos e segundos", () => {
    expect(duracao(235)).toBe("3 min 55 s");
    expect(duracao(23)).toBe("23 s");
    expect(duracao(185)).toBe("3 min 5 s");
  });
  it("formata horas", () => {
    expect(duracao(3900)).toBe("1 h 05 min");
  });
  it("trata null e negativos", () => {
    expect(duracao(null)).toBe(TRACO);
    expect(duracao(-5)).toBe("0 s");
  });
});

describe("minutosSegundos", () => {
  it("formata m:ss", () => {
    expect(minutosSegundos(65_000)).toBe("1:05");
    expect(minutosSegundos(0)).toBe("0:00");
    expect(minutosSegundos(null)).toBe(TRACO);
  });
});

describe("horas no fuso fixo", () => {
  // 2026-10-28T17:03:22Z = 14:03:22 em America/Sao_Paulo (-03:00)
  const t = Date.parse("2026-10-28T17:03:22Z");
  it("usa America/Sao_Paulo, não o fuso do navegador", () => {
    expect(hora(t)).toBe("14:03");
    expect(horaCompleta(t)).toBe("14:03:22");
    expect(diaMes(t)).toBe("28/10");
    expect(dataCompleta(t)).toBe("28/10/2026");
  });
  it("vira o dia no fuso fixo", () => {
    const t2 = Date.parse("2026-10-29T02:30:00Z"); // 23:30 do dia 28
    expect(diaMes(t2)).toBe("28/10");
    expect(hora(t2)).toBe("23:30");
  });
  it("traço para null", () => {
    expect(hora(null)).toBe(TRACO);
    expect(horaCompleta(undefined)).toBe(TRACO);
    expect(diaMes(null)).toBe(TRACO);
    expect(dataCompleta(null)).toBe(TRACO);
  });
});
