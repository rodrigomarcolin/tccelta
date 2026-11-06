import { describe, expect, it } from "vitest";
import { bandForSpeed, BAND_ORDER, bandLabel, BAND_LABEL } from "./bands";

describe("bandForSpeed", () => {
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
  ])("%s km/h falls in %s", (v, expected) => {
    expect(bandForSpeed(v)).toBe(expected);
  });
});

describe("labels", () => {
  it("covers all bands", () => {
    for (const f of BAND_ORDER) expect(BAND_LABEL[f]).toBeTruthy();
  });
  it("labels segments without data", () => {
    expect(bandLabel("falha_gps")).toBe("Falha de GPS");
    expect(bandLabel("sem_velocidade")).toBe("Sem velocidade");
    expect(bandLabel("60_80")).toBe("60–80 km/h");
  });
});
