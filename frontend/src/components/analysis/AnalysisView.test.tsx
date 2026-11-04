import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { analysisFor, renderWithTheme } from "@/tests/helpers";
import { AnalysisView } from "./AnalysisView";

vi.mock("@/components/analysis/Chart", async () => ({
  Chart: (await import("@/tests/doubles")).ChartDouble,
}));
vi.mock("@/components/analysis/RouteMap", async () => ({
  RouteMap: (await import("@/tests/doubles")).MapDouble,
}));
vi.mock("next/link", () => ({
  default: ({ href, children, ...r }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...r}>
      {children}
    </a>
  ),
}));

const charts = () => screen.queryAllByTestId("chart");
const chartButton = (title: string) => screen.getByRole("button", { name: `mover ${title}` });

describe("normal boot (scenario boot 8)", () => {
  it("shows header, map, summary and all sections, with no out-of-scope items", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    expect(
      screen.getByRole("heading", { level: 1, name: "Análise de viagem" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Dongle 1 · boot 8")).toBeInTheDocument();
    expect(screen.getByText("28/10/2026 14:03–14:11")).toBeInTheDocument();
    expect(screen.getByText("Maria")).toBeInTheDocument();
    expect(screen.getByText("categoria feira")).toBeInTheDocument();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    for (const t of ["Linha do tempo", "Marchas e rotação", "Combustível e motor", "Eventos"]) {
      expect(screen.getByRole("heading", { name: t })).toBeInTheDocument();
    }
    expect(screen.getByText("Qualidade dos dados e metodologia")).toBeInTheDocument();

    // Out of scope (criterion 22).
    expect(screen.queryByText(/ajustes/i)).toBeNull();
    expect(screen.queryByText(/limpar/i)).toBeNull();
    expect(screen.queryByText(/^Trecho/)).toBeNull();
    // Language (criterion 23).
    expect(document.body.textContent).not.toMatch(/infra[cç][aã]o|excesso|\bnota\b/i);
  });

  it("labels consumption as estimated with the fuel", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    const efficiency = screen.getByRole("region", { name: /Eficiência/ });
    expect(
      within(efficiency).getAllByText("estimado · Gasolina E30").length,
    ).toBeGreaterThanOrEqual(2);
    expect(within(efficiency).getByText("km/L")).toBeInTheDocument();
    expect(within(efficiency).getByText("L")).toBeInTheDocument();
  });

  it("legend: moving bands add up to 100% and Parado shows time; table opens from the legend", async () => {
    const a = analysisFor("normal");
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    const list = screen.getByRole("list", { name: "Faixas de velocidade" });
    const items = within(list).getAllByRole("listitem");
    expect(items).toHaveLength(6);
    expect(items[0]).toHaveTextContent("Parado");
    expect(items[0]).toHaveTextContent(/\d+ min \d+ s|\d+ s/);
    const sum = a.faixas_velocidade.reduce((s, f) => s + (f.distancia_pct ?? 0), 0);
    expect(sum).toBeGreaterThan(99.5);
    expect(sum).toBeLessThan(100.5);

    expect(screen.queryByRole("table", { name: "Faixas de velocidade" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Ver tabela das faixas" }));
    const table = screen.getByRole("table", { name: "Faixas de velocidade" });
    expect(within(table).getByText("% da distância")).toBeInTheDocument();
    expect(within(table).getAllByRole("row")).toHaveLength(7);
    await userEvent.click(screen.getByRole("button", { name: "Ocultar tabela das faixas" }));
    expect(screen.queryByRole("table", { name: "Faixas de velocidade" })).toBeNull();
  });

  it("timeline: 4 panels sharing axis and cursor", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    const titles = charts().map((g) => g.getAttribute("data-title"));
    expect(titles.slice(0, 4)).toEqual([
      "Velocidade ao longo da viagem",
      "Rotação do motor ao longo da viagem",
      "Marcha estimada ao longo da viagem",
      "Posição do pedal ao longo da viagem",
    ]);
    // Event markers on the speed panel; gear shifts on the rpm panel.
    expect(Number(charts()[0].getAttribute("data-markers"))).toBe(2);
    expect(Number(charts()[1].getAttribute("data-markers"))).toBeGreaterThan(0);
  });

  it("hovering the chart marks the instant on the map and shows the tooltip", async () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    expect(screen.getByText(/Passe o mouse/)).toBeInTheDocument();
    await userEvent.click(chartButton("Velocidade ao longo da viagem"));
    expect(screen.getByTestId("map").getAttribute("data-cursor-t")).not.toBe("");
    const tooltip = screen.getAllByRole("status").find((s) => /km\/h/.test(s.textContent ?? ""));
    expect(tooltip).toHaveTextContent(/rpm/);
    expect(tooltip).toHaveTextContent(/Marcha/);
    expect(tooltip).toHaveTextContent(/Pedal/);
    // The cursor came from the chart: the charts do not reapply it.
    expect(charts()[0].getAttribute("data-apply")).toBe("false");
  });

  it("hovering the route applies the cursor to the charts and shows the tooltip on the map", async () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    await userEvent.click(screen.getByRole("button", { name: "hover na rota" }));
    expect(charts()[0].getAttribute("data-apply")).toBe("true");
    expect(charts()[0].getAttribute("data-cursor-x")).not.toBe("");
    await userEvent.click(screen.getByRole("button", { name: "sair da rota" }));
    expect(charts()[0].getAttribute("data-cursor-x")).toBe("");
  });

  it("'ver no mapa' centers the event and pins the instant; keeps the cursor on leave", async () => {
    const a = analysisFor("normal");
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    const event = a.eventos.find((e) => e.tipo === "frenagem_brusca")!;
    await userEvent.click(screen.getByRole("button", { name: /Ver no mapa: Frenagem brusca/ }));
    expect(screen.getByTestId("map").getAttribute("data-focus")).toBe(`${event.lat},${event.lon}`);
    expect(screen.getByTestId("map").getAttribute("data-cursor-t")).toBe(String(event.t_inicio_ms));
    // Leaving a chart does not clear a deliberately pinned instant.
    await userEvent.click(
      screen.getByRole("button", { name: "sair Velocidade ao longo da viagem" }),
    );
    expect(screen.getByTestId("map").getAttribute("data-cursor-t")).toBe(String(event.t_inicio_ms));
  });

  it("clicking a gear shift pins the instant on the timeline", async () => {
    const a = analysisFor("normal");
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    const shift = a.marchas.trocas[0];
    const point = screen.getAllByRole("button", { name: /Marcar na linha do tempo/ })[0];
    fireEvent.keyDown(point, { key: "Enter" });
    expect(screen.getByTestId("map").getAttribute("data-cursor-t")).toBe(String(shift.t_ms));
    fireEvent.click(screen.getAllByRole("button", { name: /Marcar na linha do tempo/ })[1]);
    expect(screen.getByTestId("map").getAttribute("data-cursor-t")).toBe(
      String(a.marchas.trocas[1].t_ms),
    );
  });

  it("events table works by keyboard", async () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    const table = screen.getByRole("table", { name: "Eventos da viagem" });
    expect(within(table).getByText("Marcha lenta longa")).toBeInTheDocument();
    const button = within(table).getByRole("button", { name: /Aceleração brusca/ });
    button.focus();
    expect(button).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByTestId("map").getAttribute("data-focus")).not.toBe("");
  });

  it("gear shifts table opens and closes", async () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    await userEvent.click(screen.getByRole("button", { name: "Ver tabela de trocas" }));
    expect(screen.getByRole("table", { name: "Trocas de marcha" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Ocultar tabela de trocas" }));
    expect(screen.queryByRole("table", { name: "Trocas de marcha" })).toBeNull();
  });

  it("section 7 is collapsed on screen, with 'limiares do projeto' parameters and content in the DOM", async () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    const details = screen.getByText("Qualidade dos dados e metodologia").closest("details")!;
    expect(details.open).toBe(false);
    expect(within(details).getByText(/limiares do projeto/)).toBeInTheDocument();
    expect(within(details).getByText(/Gasolina E30 · AFR 13,0 · 750 g\/L/)).toBeInTheDocument();
    await userEvent.click(screen.getByText("Qualidade dos dados e metodologia"));
    expect(details.open).toBe(true);
  });

  it("section 7 opens when printing", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    const details = screen.getByText("Qualidade dos dados e metodologia").closest("details")!;
    expect(details.open).toBe(false);
    fireEvent(window, new Event("beforeprint"));
    expect(details.open).toBe(true);
    fireEvent(window, new Event("afterprint"));
    expect(details.open).toBe(false);
  });

  it("passes the active theme to the map", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />, "light");
    expect(screen.getByTestId("map").getAttribute("data-theme")).toBe("light");
  });

  it("does not show the provisional banner when complete", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("normal")} provisional={false} />);
    expect(screen.queryByText(/Resultado provisório/)).toBeNull();
  });
});

describe("no GPS (boot 9)", () => {
  it("no map, full-width summary, notice and sections 3 to 7 working", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("sem_gps")} provisional={false} />);
    expect(screen.queryByTestId("map")).toBeNull();
    // A single full-width column, not the 7fr/5fr split of the map layout.
    const summary = screen.getByRole("region", { name: "Resumo" });
    expect(summary).toHaveClass("grid-cols-[minmax(0,1fr)]");
    expect(summary).not.toHaveClass("grid-cols-[minmax(0,7fr)_minmax(0,5fr)]");
    expect(
      screen.getByText("Sem posições de GPS nesta viagem: a rota não pode ser mostrada"),
    ).toBeInTheDocument();
    expect(charts().length).toBeGreaterThan(4);
    expect(screen.getByRole("list", { name: "Faixas de velocidade" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Eventos da viagem" })).toBeInTheDocument();
  });

  it("'ver no mapa' is disabled with the 'Sem posição de GPS' hint", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("sem_gps")} provisional={false} />);
    const buttons = screen.getAllByRole("button", { name: /Ver no mapa/ });
    expect(buttons.length).toBeGreaterThan(0);
    for (const b of buttons) {
      expect(b).toBeDisabled();
      expect(b.parentElement).toHaveAttribute("title", "Sem posição de GPS");
    }
  });
});

describe("GPS with failures (boot 10)", () => {
  it("shows the map with the failure segments in the route data", () => {
    const a = analysisFor("gps_falhas");
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(a.rota!.segmentos.faixa.filter((f) => f === "falha_gps")).toHaveLength(2);
  });
});

describe("no absolute time (boot 11)", () => {
  it("header without date and time, m:ss axis and notice", () => {
    renderWithTheme(
      <AnalysisView analysis={analysisFor("sem_hora", false, 11)} provisional={false} />,
    );
    expect(screen.getByText("Dongle 1 · boot 11")).toBeInTheDocument();
    expect(screen.queryByText(/\d{2}\/\d{2}\/\d{4}/)).toBeNull();
    expect(
      screen.getByText("Esta viagem não tem hora absoluta. O eixo mostra o tempo desde o início"),
    ).toBeInTheDocument();
    // The tooltip uses m:ss.
    fireEvent.click(chartButton("Velocidade ao longo da viagem"));
    const tooltip = screen.getAllByRole("status").find((s) => /km\/h/.test(s.textContent ?? ""))!;
    expect(tooltip.textContent).toMatch(/^\d+:\d{2}/);
  });

  it("does not show 'Horário' in the summary", () => {
    renderWithTheme(
      <AnalysisView analysis={analysisFor("sem_hora", false, 11)} provisional={false} />,
    );
    expect(screen.queryByText("Horário")).toBeNull();
  });
});

describe("no readings (boot 12)", () => {
  it("not provisional: 'Sem dados para esta viagem' in place of the summary and charts", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("sem_leituras")} provisional={false} />);
    expect(screen.getByText("Sem dados para esta viagem")).toBeInTheDocument();
    expect(screen.queryByTestId("map")).toBeNull();
    expect(charts()).toHaveLength(0);
    expect(screen.queryByText("Distância")).toBeNull();
  });

  it("provisional: 'Aguardando dados' with the provisional result banner", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("provisoria_aguardando")} provisional />);
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(screen.queryByText("Sem dados para esta viagem")).toBeNull();
  });
});

describe("missing PID (boot 14)", () => {
  it("consumption, liters and tank level show 'Sem leituras' or —, never zero", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("pid_ausente")} provisional={false} />);
    expect(
      screen.getByText("O consumo foi estimado com poucas leituras do sensor de ar"),
    ).toBeInTheDocument();
    const efficiency = screen.getByRole("region", { name: /Eficiência/ });
    expect(within(efficiency).getAllByText("—")).toHaveLength(2);
    expect(within(efficiency).getAllByText("Sem leituras")).toHaveLength(2);
    const empties = screen.getAllByTestId("chart-empty").map((g) => g.textContent);
    expect(empties).toEqual(
      expect.arrayContaining([
        expect.stringContaining("Consumo instantâneo"),
        expect.stringContaining("Combustível acumulado"),
        expect.stringContaining("Nível do tanque"),
      ]),
    );
  });
});

describe("partial data (boot 15)", () => {
  it("shows the notice and the provisional banner, with the data", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("dados_parciais")} provisional />);
    expect(
      screen.getByText("Calculada com os dados que chegaram em 20 s. Pode estar incompleta"),
    ).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(charts().length).toBeGreaterThan(4);
  });
});

describe("interval with two parts", () => {
  it("header with interval and parts, time without data and pauses in the charts", () => {
    renderWithTheme(<AnalysisView analysis={analysisFor("intervalo")} provisional={false} />);
    expect(screen.getByText("Dongle 1 · 28/10 08:00–18:00 · 2 partes")).toBeInTheDocument();
    expect(screen.getByText("Sem dados")).toBeInTheDocument();
    expect(Number(charts()[0].getAttribute("data-pauses"))).toBe(1);
    // Parts in section 7.
    expect(screen.getByText("boot 8")).toBeInTheDocument();
    expect(screen.getByText("boot 9")).toBeInTheDocument();
  });
});

describe("warnings", () => {
  it("shows unknown warning text without breaking", () => {
    const a = analysisFor("normal");
    a.avisos = [{ codigo: "coisa_nova" }, { codigo: "boot_sem_hora", detalhe: "boot 7" }];
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    expect(screen.getByText("Aviso: coisa_nova")).toBeInTheDocument();
    expect(
      screen.getByText("Um boot ficou de fora por não ter hora absoluta (boot 7)"),
    ).toBeInTheDocument();
  });

  it("shows all known warnings", () => {
    const a = analysisFor("normal");
    a.avisos = [
      "partes_sobrepostas",
      "motor_nunca_ligado",
      "consumo_pode_estar_subestimado",
      "marcha_lenta_reserva",
    ].map((codigo) => ({ codigo }));
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    expect(
      screen.getAllByText(/a anterior foi cortada no início da seguinte/).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByText("O motor não ligou nesta viagem. Os valores usam todos os dados"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Consumo pode estar subestimado: houve muito tempo com pedal alto"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("A rotação de marcha lenta usada é um valor de reserva"),
    ).toBeInTheDocument();
  });
});

describe("missing signal", () => {
  it("chart without readings for a signal shows 'Sem leituras' and an indicator —", () => {
    const a = analysisFor("normal");
    a.series.rpm = a.series.rpm.map(() => null);
    a.marchas.subida_rpm.mediana = null;
    renderWithTheme(<AnalysisView analysis={a} provisional={false} />);
    expect(
      screen.getByText(/Rotação do motor ao longo da viagem: Sem leituras/),
    ).toBeInTheDocument();
    expect(screen.getByText("Rotação × velocidade").closest("section")).toHaveTextContent(
      "Sem leituras",
    );
  });
});
