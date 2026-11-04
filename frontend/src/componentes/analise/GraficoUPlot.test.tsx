import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PropsGrafico } from "./GraficoUPlot";

type Opts = {
  hooks: { setCursor: ((u: Fake) => void)[]; draw: ((u: Fake) => void)[] };
  axes: { values: (u: Fake, s: number[]) => string[]; splits?: () => number[] }[];
  series: { stroke?: string; dash?: number[]; paths?: unknown }[];
  scales: { y: { range?: () => [number, number] } };
  cursor: { sync?: { key: string } };
};

const { Fake } = vi.hoisted(() => {
  class FakeUPlot {
    static instancias: FakeUPlot[] = [];
    static paths = { stepped: () => "passo" };
    static chamadasCtx: string[] = [];
    cursor: { idx: number | null; left: number | null } = { idx: null, left: -1 };
    data: number[][];
    bbox = { left: 0, top: 0, width: 400, height: 100 };
    ctx = new Proxy(
      {},
      {
        get: (_t, p) =>
          p === "canvas" ? {} : FakeUPlot.chamadasCtx.push(String(p)) && (() => undefined),
        set: () => true,
      },
    ) as unknown as CanvasRenderingContext2D;
    setCursor = vi.fn();
    setSize = vi.fn();
    destroy = vi.fn();
    valToPos = vi.fn((v: number) => v / 10);
    constructor(
      public opts: Opts,
      dados: number[][],
      public el: HTMLElement,
    ) {
      this.data = dados;
      FakeUPlot.instancias.push(this);
    }
  }
  return { Fake: FakeUPlot };
});
type Fake = InstanceType<typeof Fake>;

vi.mock("uplot", () => ({ default: Fake }));
vi.mock("uplot/dist/uPlot.min.css", () => ({}));

class RO {
  static ultimo: RO;
  constructor(public cb: () => void) {
    RO.ultimo = this;
  }
  observe = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", RO);

import GraficoUPlot from "./GraficoUPlot";
import { renderComTema } from "@/testes/helpers";

const base = (extra: Partial<PropsGrafico> = {}): PropsGrafico => ({
  titulo: "Velocidade",
  x: [0, 100, 200],
  series: [{ rotulo: "v", valores: [1, null, 3], cor: "--accent" }],
  formatarX: (x) => `t${x}`,
  ...extra,
});

beforeEach(() => {
  Fake.instancias = [];
  Fake.chamadasCtx = [];
});

describe("GraficoUPlot", () => {
  it("cria o gráfico com os dados, sincronização e eixos formatados", () => {
    const fmtY = vi.fn((v: number) => `y${v}`);
    renderComTema(
      <GraficoUPlot
        {...base({ sync: "linha", formatarY: fmtY, ticksY: [0, 1], faixaY: [0, 5] })}
      />,
    );
    const u = Fake.instancias[0];
    expect(u.data).toEqual([
      [0, 100, 200],
      [1, null, 3],
    ]);
    expect(u.opts.cursor.sync).toEqual({ key: "linha" });
    expect(u.opts.axes[0].values(u, [0, 100])).toEqual(["t0", "t100"]);
    expect(u.opts.axes[1].values(u, [0, 1])).toEqual(["y0", "y1"]);
    expect(u.opts.axes[1].splits?.()).toEqual([0, 1]);
    expect(u.opts.scales.y.range?.()).toEqual([0, 5]);
    expect(screen.getByRole("img", { name: "Velocidade" })).toBeInTheDocument();
  });

  it("usa degraus e tracejado quando pedido e cor literal", () => {
    renderComTema(
      <GraficoUPlot
        {...base({
          series: [
            { rotulo: "m", valores: [1, 2, 3], cor: "#123456", passo: true, tracejada: true },
          ],
        })}
      />,
    );
    const s = Fake.instancias[0].opts.series[1];
    expect(s.stroke).toBe("#123456");
    expect(s.dash).toEqual([4, 4]);
    expect(s.paths).toBe("passo");
  });

  it("destrói o gráfico ao desmontar e acompanha o redimensionamento", () => {
    const { unmount } = renderComTema(<GraficoUPlot {...base()} />);
    const u = Fake.instancias[0];
    RO.ultimo.cb();
    expect(u.setSize).toHaveBeenCalled();
    unmount();
    expect(u.destroy).toHaveBeenCalled();
    expect(RO.ultimo.disconnect).toHaveBeenCalled();
  });

  it("repassa o cursor do mouse como coordenada X, ou null ao sair", () => {
    const onCursorX = vi.fn();
    renderComTema(<GraficoUPlot {...base({ onCursorX })} />);
    const u = Fake.instancias[0];
    u.cursor = { idx: 1, left: 30 };
    u.opts.hooks.setCursor[0](u);
    expect(onCursorX).toHaveBeenLastCalledWith(100);
    u.cursor = { idx: null, left: -1 };
    u.opts.hooks.setCursor[0](u);
    expect(onCursorX).toHaveBeenLastCalledWith(null);
  });

  it("ignora o evento de cursor sem callback", () => {
    renderComTema(<GraficoUPlot {...base()} />);
    const u = Fake.instancias[0];
    u.cursor = { idx: 1, left: 30 };
    expect(() => u.opts.hooks.setCursor[0](u)).not.toThrow();
  });

  it("aplica o cursor vindo de fora sem disparar o callback", () => {
    const onCursorX = vi.fn();
    const p = base({ onCursorX, cursorX: 100, aplicarCursor: true });
    const { rerender } = render(<GraficoUPlot {...p} />);
    const u = Fake.instancias[0];
    expect(u.setCursor).toHaveBeenCalledWith({ left: 10, top: 10 });
    // O setCursor programático dispara o hook, que deve ser ignorado.
    u.setCursor.mockImplementation(() => {
      u.cursor = { idx: 1, left: 10 };
      u.opts.hooks.setCursor[0](u);
    });
    rerender(<GraficoUPlot {...p} cursorX={200} />);
    expect(onCursorX).not.toHaveBeenCalled();
    rerender(<GraficoUPlot {...p} cursorX={null} />);
    expect(u.setCursor).toHaveBeenLastCalledWith({ left: -10, top: -10 });
  });

  it("não aplica o cursor quando ele veio do próprio gráfico", () => {
    renderComTema(<GraficoUPlot {...base({ cursorX: 100, aplicarCursor: false })} />);
    expect(Fake.instancias[0].setCursor).not.toHaveBeenCalled();
  });

  it("desenha marcadores, pausas e referência", () => {
    renderComTema(
      <GraficoUPlot
        {...base({
          marcadores: [
            { x: 0, y: 1, forma: "tri-baixo" },
            { x: 100, y: 1, forma: "tri-cima" },
            { x: 200, y: 1, forma: "circulo" },
            { x: 200, y: 2, forma: "circulo-vazio" },
          ],
          pausas: [{ x0: 50, x1: 80, rotulo: "Sem dados · 5 h 00 min" }],
          referenciaY: { y: 80, rotulo: "Motor aquecido (80 °C)" },
        })}
      />,
    );
    const u = Fake.instancias[0];
    u.opts.hooks.draw[0](u);
    expect(Fake.chamadasCtx).toEqual(
      expect.arrayContaining(["fillRect", "fillText", "stroke", "fill", "arc", "lineTo"]),
    );
  });

  it("mostra 'Sem leituras' em vez do gráfico quando o sinal falta", () => {
    renderComTema(<GraficoUPlot {...base({ semLeituras: true })} />);
    expect(screen.getByText("Sem leituras")).toBeInTheDocument();
    expect(Fake.instancias).toHaveLength(0);
  });

  it("não cria gráfico sem pontos", () => {
    renderComTema(
      <GraficoUPlot
        {...base({ x: [], series: [{ rotulo: "v", valores: [], cor: "--accent" }] })}
      />,
    );
    expect(Fake.instancias).toHaveLength(0);
  });
});
