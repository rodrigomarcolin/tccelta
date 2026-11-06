import { describe, expect, it } from "vitest";
import { analysisFor } from "@/tests/helpers";
import {
  hasAnyValue,
  headerDescription,
  analysisAxis,
  timeFormatter,
  fullTimeFormatter,
  nearestInstant,
  gearLabel,
  hasReadings,
  seriesTimes,
  pauseText,
} from "./analysis";

describe("headerDescription (criterion 11)", () => {
  it("boot: Dongle 1 · boot 8 with date and time", () => {
    const d = headerDescription(analysisFor("normal"));
    expect(d.identification).toBe("Dongle 1 · boot 8");
    expect(d.when).toBe("28/10/2026 14:03–14:11");
  });
  it("requested interval and number of parts", () => {
    const d = headerDescription(analysisFor("intervalo"));
    expect(d.identification).toBe("Dongle 1 · 28/10 08:00–18:00 · 2 partes");
    expect(d.when).toBeNull();
  });
  it("interval on different days and with a single part", () => {
    const a = analysisFor("intervalo");
    if (a.viagem.entrada.tipo === "intervalo") {
      a.viagem.entrada.t1 = "2026-10-29T21:00:00.000Z";
    }
    a.viagem.partes = a.viagem.partes.slice(0, 1);
    expect(headerDescription(a).identification).toBe("Dongle 1 · 28/10 08:00–29/10 18:00");
  });
  it("boot without absolute time omits date and time", () => {
    const d = headerDescription(analysisFor("sem_hora", false, 11));
    expect(d).toEqual({ identification: "Dongle 1 · boot 11", when: null });
  });
});

describe("axis and formatters", () => {
  it("absolute time with clock; m:ss without clock", () => {
    const a = analysisFor("normal");
    const ini = a.viagem.inicio_t_ms!;
    expect(timeFormatter(a)(ini)).toBe("14:03");
    expect(fullTimeFormatter(a)(ini)).toBe("14:03:22");
    const s = analysisFor("sem_hora", false, 11);
    expect(timeFormatter(s)(s.viagem.inicio_t_ms! + 65_000)).toBe("1:05");
    expect(fullTimeFormatter(s)(s.viagem.inicio_t_ms! + 5_000)).toBe("0:05");
  });

  it("the axis compresses the pause between interval parts and the cursor returns the right time", () => {
    const a = analysisFor("intervalo");
    const e = analysisAxis(a);
    expect(e.pauses).toHaveLength(1);
    const [p1, p2] = a.viagem.partes;
    expect(e.pauses[0].durationMs).toBe(p2.inicio_t_ms - p1.fim_t_ms);
    expect(e.fromX(e.toX(p2.inicio_t_ms + 30_000))).toBeCloseTo(p2.inicio_t_ms + 30_000);
    const fmt = fullTimeFormatter(a);
    expect(fmt(e.fromX(e.toX(p1.inicio_t_ms)))).toBe(fmt(p1.inicio_t_ms));
  });

  it("pauseText keeps the real duration", () => {
    expect(pauseText(5 * 3600 * 1000 + 120_000)).toBe("Sem dados · 5 h 02 min");
  });
});

describe("nearest instant", () => {
  it("returns the readings of the grid point; null where data is missing", () => {
    const a = analysisFor("normal");
    const times = seriesTimes(a);
    const i = nearestInstant(a, times, times[100] + 10)!;
    expect(i.t).toBe(times[100]);
    expect(i.speed).toBe(a.series.velocidade_kmh[100]);
    const b = analysisFor("intervalo");
    const sep = b.series.rpm.indexOf(null);
    const t = seriesTimes(b);
    const r = nearestInstant(b, t, t[sep])!;
    expect(r).toMatchObject({ speed: null, rpm: null, gear: null, pedal: null });
    expect(nearestInstant(analysisFor("sem_leituras"), [], 0)).toBeNull();
  });
});

describe("helpers", () => {
  it("gearLabel", () => {
    expect(gearLabel(null)).toBe("—");
    expect(gearLabel(0)).toBe("Ponto morto ou embreagem");
    expect(gearLabel(3)).toBe("3ª");
  });
  it("hasReadings and hasAnyValue", () => {
    expect(hasReadings(analysisFor("normal"))).toBe(true);
    expect(hasReadings(analysisFor("sem_leituras"))).toBe(false);
    expect(hasAnyValue([null, null])).toBe(false);
    expect(hasAnyValue([null, 0])).toBe(true);
  });
});
