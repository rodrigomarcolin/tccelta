import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  AwaitingDataState,
  LoadingState,
  ErrorState,
  WaitingState,
  FailedState,
  NotFoundState,
  NoDataState,
  ProvisionalBanner,
} from "./States";
import { HorizontalBars, Scatter, Heatmap } from "./svgCharts";
import { InstantTooltip } from "./Instant";
import { peakText } from "./EventsTable";
import { warningText } from "./texts";
import { cssColor, bandColor, gearColor, bandVar, gearVar } from "./colors";
import { DASH } from "@/format/numbers";

vi.mock("next/link", () => ({
  default: ({ href, children, ...r }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...r}>
      {children}
    </a>
  ),
}));

describe("states", () => {
  it("waiting with and without a name", () => {
    const { rerender } = render(<WaitingState driverName="Ana" />);
    expect(screen.getByText("Processando a viagem de «Ana»")).toBeInTheDocument();
    rerender(<WaitingState driverName={null} />);
    expect(screen.getByText("Processando a viagem")).toBeInTheDocument();
  });
  it("remaining states", async () => {
    render(
      <>
        <LoadingState />
        <FailedState />
        <NotFoundState />
        <NoDataState />
        <AwaitingDataState />
        <ProvisionalBanner />
      </>,
    );
    expect(screen.getByLabelText("Carregando a análise")).toBeInTheDocument();
    expect(screen.getByText(/O nome do motorista foi liberado/)).toBeInTheDocument();
    expect(screen.getByText("Viagem não encontrada")).toBeInTheDocument();
    expect(screen.getByText("Sem dados para esta viagem")).toBeInTheDocument();
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText(/Resultado provisório/)).toBeInTheDocument();
  });
  it("error calls retry", async () => {
    const onRetry = vi.fn();
    render(<ErrorState onRetry={onRetry} />);
    await userEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    expect(onRetry).toHaveBeenCalled();
  });
});

describe("warningText", () => {
  it("returns text for each known code", () => {
    for (const code of [
      "sem_gps",
      "sem_hora_absoluta",
      "partes_sobrepostas",
      "sem_leituras",
      "motor_nunca_ligado",
      "cobertura_maf_baixa",
      "consumo_pode_estar_subestimado",
      "marcha_lenta_reserva",
      "dados_parciais",
    ]) {
      expect(warningText({ codigo: code })).not.toMatch(/^Aviso:/);
    }
  });
  it("boot_sem_hora with and without detail, and unknown code", () => {
    expect(warningText({ codigo: "boot_sem_hora", detalhe: "boot 7" })).toBe(
      "Um boot ficou de fora por não ter hora absoluta (boot 7)",
    );
    expect(warningText({ codigo: "boot_sem_hora" })).toBe(
      "Um boot ficou de fora por não ter hora absoluta",
    );
    expect(warningText({ codigo: "xyz" })).toBe("Aviso: xyz");
  });
});

describe("InstantTooltip", () => {
  it("shows a dash where the value is null", () => {
    render(
      <InstantTooltip
        instant={{ t: 1, speed: null, rpm: null, gear: null, pedal: null }}
        formatT={() => "14:03:22"}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(DASH);
    expect(screen.getAllByText(DASH).length).toBeGreaterThanOrEqual(3);
  });
  it("gear 0 has its own label, without a number", () => {
    render(
      <InstantTooltip
        instant={{ t: 1, speed: 40, rpm: 900, gear: 0, pedal: 0 }}
        formatT={() => "14:03:22"}
      />,
    );
    expect(screen.getByText("Ponto morto ou embreagem")).toBeInTheDocument();
  });
});

describe("EventsTable.peakText", () => {
  it("uses m/s² or seconds", () => {
    const base = {
      tipo: "frenagem_brusca" as const,
      t_inicio_ms: 0,
      duracao_s: 1,
      velocidade_inicial_kmh: 1,
      velocidade_final_kmh: 1,
      lat: null,
      lon: null,
    };
    expect(peakText({ ...base, pico: -6.8, pico_unidade: "m/s2" })).toBe("-6,8 m/s²");
    expect(peakText({ ...base, tipo: "marcha_lenta_longa", pico: 64.5, pico_unidade: "s" })).toBe(
      "1 min 5 s",
    );
  });
});

describe("colors", () => {
  it("reads theme CSS variables and falls back", () => {
    document.documentElement.style.setProperty("--band-30_60", "#b44d2c");
    expect(bandColor("30_60")).toBe("#b44d2c");
    expect(cssColor("--does-not-exist", "#123456")).toBe("#123456");
    expect(bandVar("falha_gps")).toBe("--band-fault");
    expect(bandVar("sem_velocidade")).toBe("--band-fault");
    expect(bandVar("60_80")).toBe("--band-60_80");
    expect(gearVar(0)).toBe("--gear-0");
    expect(gearVar(3)).toBe("--gear-3");
    expect(gearVar(9)).toBe("--gear-6");
    expect(gearColor(0)).toBe("#888888");
  });
});

describe("svgCharts", () => {
  it("HorizontalBars: % and time, and a dash without a value", () => {
    render(
      <HorizontalBars
        title="Tempo"
        items={[
          { label: "1ª", pct: 50, timeS: 30, color: "--gear-1" },
          { label: "2ª", pct: null, timeS: null, color: "#fff" },
          { label: "3ª", pct: 10, timeS: 6 },
        ]}
      />,
    );
    expect(screen.getByRole("list", { name: "Tempo" })).toBeInTheDocument();
    expect(screen.getByText("50% · 30 s")).toBeInTheDocument();
    expect(screen.getByText("— · —")).toBeInTheDocument();
  });

  it("Scatter: points clickable by mouse and keyboard, lines and dashes", async () => {
    const onClick = vi.fn();
    render(
      <Scatter
        title="Teste"
        points={[
          { x: 1, y: 1, color: "--accent" },
          { x: 2, y: 2, color: "#fff", label: "Ponto A", onClick },
        ]}
        lines={[
          { x1: 0, y1: 0, x2: 4, y2: 4, color: "--gear-1", label: "1ª" },
          { x1: 0, y1: 0, x2: 4, y2: 2, color: "#fff" },
        ]}
        dashes={[
          { x1: 0, x2: 1, y: 2, label: "mediana" },
          { x1: 0, x2: 1, y: 3 },
        ]}
        domainX={[0, 4]}
        domainY={[0, 4]}
        ticksX={[{ v: 0, label: "0" }]}
        ticksY={[{ v: 0, label: "0" }]}
        labelX="x"
        labelY="y"
      />,
    );
    const point = screen.getByRole("button", { name: "Ponto A" });
    await userEvent.click(point);
    point.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");
    await userEvent.keyboard("a");
    expect(onClick).toHaveBeenCalledTimes(3);
    expect(screen.getByText("mediana")).toBeInTheDocument();
  });

  it("Heatmap: only shows the bins with time, with a legend", () => {
    const { container } = render(
      <Heatmap
        rpmBin={500}
        pedalBin={10}
        timeS={[
          [0, 0],
          [10, 0],
          [0, 5],
          [0, 0],
        ]}
      />,
    );
    expect(container.querySelectorAll("rect")).toHaveLength(4);
    expect(screen.getByText(/Cor: tempo/)).toBeInTheDocument();
    expect(container.querySelector("rect")?.getAttribute("style")).toContain("rgba(var(--heat)");
  });

  it("Heatmap without time shows 'Sem leituras'", () => {
    render(<Heatmap rpmBin={500} pedalBin={10} timeS={[]} />);
    expect(screen.getByText("Sem leituras")).toBeInTheDocument();
  });
});
