import { describe, expect, it } from "vitest";
import {
  fullDate,
  dayMonth,
  formatDuration,
  formatTime,
  formatFullTime,
  minutesSeconds,
  formatNumber,
  formatPercent,
  DASH,
} from "./numbers";

describe("formatNumber", () => {
  it("uses the pt-BR format", () => {
    expect(formatNumber(3.5, 1)).toBe("3,5");
    expect(formatNumber(1240, 0)).toBe("1.240");
    expect(formatNumber(0.83, 2)).toBe("0,83");
  });
  it("shows a dash for null, undefined and non-finite", () => {
    expect(formatNumber(null)).toBe(DASH);
    expect(formatNumber(undefined)).toBe(DASH);
    expect(formatNumber(Number.NaN)).toBe(DASH);
  });
  it("does not show negative zero", () => {
    expect(formatNumber(-0.001, 1)).toBe("0,0");
  });
  it("keeps a real zero", () => {
    expect(formatNumber(0, 0)).toBe("0");
  });
});

describe("formatPercent", () => {
  it("formats and handles null", () => {
    expect(formatPercent(10, 0)).toBe("10%");
    expect(formatPercent(null)).toBe(DASH);
  });
});

describe("formatDuration", () => {
  it("formats minutes and seconds", () => {
    expect(formatDuration(235)).toBe("3 min 55 s");
    expect(formatDuration(23)).toBe("23 s");
    expect(formatDuration(185)).toBe("3 min 5 s");
  });
  it("formats hours", () => {
    expect(formatDuration(3900)).toBe("1 h 05 min");
  });
  it("handles null and negatives", () => {
    expect(formatDuration(null)).toBe(DASH);
    expect(formatDuration(-5)).toBe("0 s");
  });
});

describe("minutesSeconds", () => {
  it("formats m:ss", () => {
    expect(minutesSeconds(65_000)).toBe("1:05");
    expect(minutesSeconds(0)).toBe("0:00");
    expect(minutesSeconds(null)).toBe(DASH);
  });
});

describe("times in the fixed timezone", () => {
  // 2026-10-28T17:03:22Z = 14:03:22 in America/Sao_Paulo (-03:00)
  const t = Date.parse("2026-10-28T17:03:22Z");
  it("uses America/Sao_Paulo, not the browser timezone", () => {
    expect(formatTime(t)).toBe("14:03");
    expect(formatFullTime(t)).toBe("14:03:22");
    expect(dayMonth(t)).toBe("28/10");
    expect(fullDate(t)).toBe("28/10/2026");
  });
  it("rolls over the day in the fixed timezone", () => {
    const t2 = Date.parse("2026-10-29T02:30:00Z"); // 23:30 on day 28
    expect(dayMonth(t2)).toBe("28/10");
    expect(formatTime(t2)).toBe("23:30");
  });
  it("dash for null", () => {
    expect(formatTime(null)).toBe(DASH);
    expect(formatFullTime(undefined)).toBe(DASH);
    expect(dayMonth(null)).toBe(DASH);
    expect(fullDate(null)).toBe(DASH);
  });
});
