import { fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { analiseDe, renderComTema } from "@/testes/helpers";
import { AnaliseView } from "./AnaliseView";

vi.mock("@/componentes/analise/Grafico", async () => ({
  Grafico: (await import("@/testes/dubles")).GraficoDuble,
}));
vi.mock("@/componentes/analise/Mapa", async () => ({
  Mapa: (await import("@/testes/dubles")).MapaDuble,
}));
vi.mock("next/link", () => ({
  default: ({ href, children, ...r }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...r}>
      {children}
    </a>
  ),
}));

const graficos = () => screen.queryAllByTestId("grafico");
const botaoGrafico = (titulo: string) => screen.getByRole("button", { name: `mover ${titulo}` });

describe("boot normal (cenário boot 8)", () => {
  it("mostra cabeçalho, mapa, resumo e todas as seções, sem itens fora de escopo", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    expect(
      screen.getByRole("heading", { level: 1, name: "Análise de viagem" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Dongle 1 · boot 8")).toBeInTheDocument();
    expect(screen.getByText("28/10/2026 14:03–14:11")).toBeInTheDocument();
    expect(screen.getByText("Maria")).toBeInTheDocument();
    expect(screen.getByText("categoria feira")).toBeInTheDocument();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    for (const t of ["Linha do tempo", "Marchas e rotação", "Combustível e motor", "Eventos"]) {
      expect(screen.getByRole("heading", { name: t })).toBeInTheDocument();
    }
    expect(screen.getByText("Qualidade dos dados e metodologia")).toBeInTheDocument();

    // Fora de escopo (critério 22).
    expect(screen.queryByText(/ajustes/i)).toBeNull();
    expect(screen.queryByText(/limpar/i)).toBeNull();
    expect(screen.queryByText(/^Trecho/)).toBeNull();
    // Linguagem (critério 23).
    expect(document.body.textContent).not.toMatch(/infra[cç][aã]o|excesso|\bnota\b/i);
  });

  it("rotula consumo como estimado com o combustível", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    const efic = screen.getByRole("region", { name: /Eficiência/ });
    expect(within(efic).getAllByText("estimado · Gasolina E30").length).toBeGreaterThanOrEqual(2);
    expect(within(efic).getByText("km/L")).toBeInTheDocument();
    expect(within(efic).getByText("L")).toBeInTheDocument();
  });

  it("legenda: faixas em movimento somam 100% e Parado mostra tempo; tabela abre pela legenda", async () => {
    const a = analiseDe("normal");
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    const lista = screen.getByRole("list", { name: "Faixas de velocidade" });
    const itens = within(lista).getAllByRole("listitem");
    expect(itens).toHaveLength(6);
    expect(itens[0]).toHaveTextContent("Parado");
    expect(itens[0]).toHaveTextContent(/\d+ min \d+ s|\d+ s/);
    const soma = a.faixas_velocidade.reduce((s, f) => s + (f.distancia_pct ?? 0), 0);
    expect(soma).toBeGreaterThan(99.5);
    expect(soma).toBeLessThan(100.5);

    expect(screen.queryByRole("table", { name: "Faixas de velocidade" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Ver tabela das faixas" }));
    const tabela = screen.getByRole("table", { name: "Faixas de velocidade" });
    expect(within(tabela).getByText("% da distância")).toBeInTheDocument();
    expect(within(tabela).getAllByRole("row")).toHaveLength(7);
    await userEvent.click(screen.getByRole("button", { name: "Ocultar tabela das faixas" }));
    expect(screen.queryByRole("table", { name: "Faixas de velocidade" })).toBeNull();
  });

  it("linha do tempo: 4 painéis com eixo e cursor compartilhados", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    const titulos = graficos().map((g) => g.getAttribute("data-titulo"));
    expect(titulos.slice(0, 4)).toEqual([
      "Velocidade ao longo da viagem",
      "Rotação do motor ao longo da viagem",
      "Marcha estimada ao longo da viagem",
      "Posição do pedal ao longo da viagem",
    ]);
    // Marcadores de evento no painel de velocidade; trocas no de rotação.
    expect(Number(graficos()[0].getAttribute("data-marcadores"))).toBe(2);
    expect(Number(graficos()[1].getAttribute("data-marcadores"))).toBeGreaterThan(0);
  });

  it("hover no gráfico marca o instante no mapa e mostra o tooltip", async () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    expect(screen.getByText(/Passe o mouse/)).toBeInTheDocument();
    await userEvent.click(botaoGrafico("Velocidade ao longo da viagem"));
    expect(screen.getByTestId("mapa").getAttribute("data-cursor-t")).not.toBe("");
    const tooltip = screen.getAllByRole("status").find((s) => /km\/h/.test(s.textContent ?? ""));
    expect(tooltip).toHaveTextContent(/rpm/);
    expect(tooltip).toHaveTextContent(/Marcha/);
    expect(tooltip).toHaveTextContent(/Pedal/);
    // Cursor veio do gráfico: os gráficos não o reaplicam.
    expect(graficos()[0].getAttribute("data-aplicar")).toBe("false");
  });

  it("hover na rota aplica o cursor nos gráficos e mostra o tooltip no mapa", async () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    await userEvent.click(screen.getByRole("button", { name: "hover na rota" }));
    expect(graficos()[0].getAttribute("data-aplicar")).toBe("true");
    expect(graficos()[0].getAttribute("data-cursor-x")).not.toBe("");
    await userEvent.click(screen.getByRole("button", { name: "sair da rota" }));
    expect(graficos()[0].getAttribute("data-cursor-x")).toBe("");
  });

  it("'ver no mapa' centraliza o evento e marca o instante; mantém o cursor ao sair", async () => {
    const a = analiseDe("normal");
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    const evento = a.eventos.find((e) => e.tipo === "frenagem_brusca")!;
    await userEvent.click(screen.getByRole("button", { name: /Ver no mapa: Frenagem brusca/ }));
    expect(screen.getByTestId("mapa").getAttribute("data-foco")).toBe(
      `${evento.lat},${evento.lon}`,
    );
    expect(screen.getByTestId("mapa").getAttribute("data-cursor-t")).toBe(
      String(evento.t_inicio_ms),
    );
    // Sair de um gráfico não apaga o instante marcado de propósito.
    await userEvent.click(
      screen.getByRole("button", { name: "sair Velocidade ao longo da viagem" }),
    );
    expect(screen.getByTestId("mapa").getAttribute("data-cursor-t")).toBe(
      String(evento.t_inicio_ms),
    );
  });

  it("clicar em uma troca marca o instante na linha do tempo", async () => {
    const a = analiseDe("normal");
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    const troca = a.marchas.trocas[0];
    const ponto = screen.getAllByRole("button", { name: /Marcar na linha do tempo/ })[0];
    fireEvent.keyDown(ponto, { key: "Enter" });
    expect(screen.getByTestId("mapa").getAttribute("data-cursor-t")).toBe(String(troca.t_ms));
    fireEvent.click(screen.getAllByRole("button", { name: /Marcar na linha do tempo/ })[1]);
    expect(screen.getByTestId("mapa").getAttribute("data-cursor-t")).toBe(
      String(a.marchas.trocas[1].t_ms),
    );
  });

  it("tabela de eventos funciona por teclado", async () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    const tabela = screen.getByRole("table", { name: "Eventos da viagem" });
    expect(within(tabela).getByText("Marcha lenta longa")).toBeInTheDocument();
    const botao = within(tabela).getByRole("button", { name: /Aceleração brusca/ });
    botao.focus();
    expect(botao).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByTestId("mapa").getAttribute("data-foco")).not.toBe("");
  });

  it("tabela de trocas abre e fecha", async () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    await userEvent.click(screen.getByRole("button", { name: "Ver tabela de trocas" }));
    expect(screen.getByRole("table", { name: "Trocas de marcha" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Ocultar tabela de trocas" }));
    expect(screen.queryByRole("table", { name: "Trocas de marcha" })).toBeNull();
  });

  it("seção 7 recolhida em tela, com parâmetros 'limiares do projeto' e conteúdo no DOM", async () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    const det = screen.getByText("Qualidade dos dados e metodologia").closest("details")!;
    expect(det.open).toBe(false);
    expect(within(det).getByText(/limiares do projeto/)).toBeInTheDocument();
    expect(within(det).getByText(/Gasolina E30 · AFR 13,0 · 750 g\/L/)).toBeInTheDocument();
    await userEvent.click(screen.getByText("Qualidade dos dados e metodologia"));
    expect(det.open).toBe(true);
  });

  it("seção 7 abre na impressão", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    const det = screen.getByText("Qualidade dos dados e metodologia").closest("details")!;
    expect(det.open).toBe(false);
    fireEvent(window, new Event("beforeprint"));
    expect(det.open).toBe(true);
    fireEvent(window, new Event("afterprint"));
    expect(det.open).toBe(false);
  });

  it("passa o tema ativo ao mapa", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />, "claro");
    expect(screen.getByTestId("mapa").getAttribute("data-tema")).toBe("claro");
  });

  it("não mostra faixa de provisório quando completa", () => {
    renderComTema(<AnaliseView analise={analiseDe("normal")} provisoria={false} />);
    expect(screen.queryByText(/Resultado provisório/)).toBeNull();
  });
});

describe("sem GPS (boot 9)", () => {
  it("sem mapa, resumo em largura total, aviso e seções 3 a 7 funcionando", () => {
    const { container } = renderComTema(
      <AnaliseView analise={analiseDe("sem_gps")} provisoria={false} />,
    );
    expect(screen.queryByTestId("mapa")).toBeNull();
    expect(container.querySelector(".resumo-total")).not.toBeNull();
    expect(
      screen.getByText("Sem posições de GPS nesta viagem: a rota não pode ser mostrada"),
    ).toBeInTheDocument();
    expect(graficos().length).toBeGreaterThan(4);
    expect(screen.getByRole("list", { name: "Faixas de velocidade" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Eventos da viagem" })).toBeInTheDocument();
  });

  it("'ver no mapa' fica desativado com a dica 'Sem posição de GPS'", () => {
    renderComTema(<AnaliseView analise={analiseDe("sem_gps")} provisoria={false} />);
    const botoes = screen.getAllByRole("button", { name: /Ver no mapa/ });
    expect(botoes.length).toBeGreaterThan(0);
    for (const b of botoes) {
      expect(b).toBeDisabled();
      expect(b.parentElement).toHaveAttribute("title", "Sem posição de GPS");
    }
  });
});

describe("GPS com falhas (boot 10)", () => {
  it("mostra o mapa com os trechos de falha nos dados da rota", () => {
    const a = analiseDe("gps_falhas");
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(a.rota!.segmentos.faixa.filter((f) => f === "falha_gps")).toHaveLength(2);
  });
});

describe("sem hora absoluta (boot 11)", () => {
  it("cabeçalho sem data e hora, eixo em m:ss e aviso", () => {
    renderComTema(<AnaliseView analise={analiseDe("sem_hora", false, 11)} provisoria={false} />);
    expect(screen.getByText("Dongle 1 · boot 11")).toBeInTheDocument();
    expect(screen.queryByText(/\d{2}\/\d{2}\/\d{4}/)).toBeNull();
    expect(
      screen.getByText("Esta viagem não tem hora absoluta. O eixo mostra o tempo desde o início"),
    ).toBeInTheDocument();
    // O tooltip usa m:ss.
    fireEvent.click(botaoGrafico("Velocidade ao longo da viagem"));
    const tooltip = screen.getAllByRole("status").find((s) => /km\/h/.test(s.textContent ?? ""))!;
    expect(tooltip.textContent).toMatch(/^\d+:\d{2}/);
  });

  it("não mostra 'Horário' no resumo", () => {
    renderComTema(<AnaliseView analise={analiseDe("sem_hora", false, 11)} provisoria={false} />);
    expect(screen.queryByText("Horário")).toBeNull();
  });
});

describe("sem leituras (boot 12)", () => {
  it("não provisória: 'Sem dados para esta viagem' no lugar do resumo e dos gráficos", () => {
    renderComTema(<AnaliseView analise={analiseDe("sem_leituras")} provisoria={false} />);
    expect(screen.getByText("Sem dados para esta viagem")).toBeInTheDocument();
    expect(screen.queryByTestId("mapa")).toBeNull();
    expect(graficos()).toHaveLength(0);
    expect(screen.queryByText("Distância")).toBeNull();
  });

  it("provisória: 'Aguardando dados' com a faixa de resultado provisório", () => {
    renderComTema(<AnaliseView analise={analiseDe("provisoria_aguardando")} provisoria />);
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(screen.queryByText("Sem dados para esta viagem")).toBeNull();
  });
});

describe("PID ausente (boot 14)", () => {
  it("consumo, litros e nível do tanque mostram 'Sem leituras' ou —, nunca zero", () => {
    renderComTema(<AnaliseView analise={analiseDe("pid_ausente")} provisoria={false} />);
    expect(
      screen.getByText("O consumo foi estimado com poucas leituras do sensor de ar"),
    ).toBeInTheDocument();
    const efic = screen.getByRole("region", { name: /Eficiência/ });
    expect(within(efic).getAllByText("—")).toHaveLength(2);
    expect(within(efic).getAllByText("Sem leituras")).toHaveLength(2);
    const vazios = screen.getAllByTestId("grafico-vazio").map((g) => g.textContent);
    expect(vazios).toEqual(
      expect.arrayContaining([
        expect.stringContaining("Consumo instantâneo"),
        expect.stringContaining("Combustível acumulado"),
        expect.stringContaining("Nível do tanque"),
      ]),
    );
  });
});

describe("dados parciais (boot 15)", () => {
  it("mostra o aviso e a faixa provisória, com os dados", () => {
    renderComTema(<AnaliseView analise={analiseDe("dados_parciais")} provisoria />);
    expect(
      screen.getByText("Calculada com os dados que chegaram em 20 s. Pode estar incompleta"),
    ).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(graficos().length).toBeGreaterThan(4);
  });
});

describe("intervalo com duas partes", () => {
  it("cabeçalho com intervalo e partes, tempo sem dados e pausas nos gráficos", () => {
    renderComTema(<AnaliseView analise={analiseDe("intervalo")} provisoria={false} />);
    expect(screen.getByText("Dongle 1 · 28/10 08:00–18:00 · 2 partes")).toBeInTheDocument();
    expect(screen.getByText("Sem dados")).toBeInTheDocument();
    expect(Number(graficos()[0].getAttribute("data-pausas"))).toBe(1);
    // Partes na seção 7.
    expect(screen.getByText("boot 8")).toBeInTheDocument();
    expect(screen.getByText("boot 9")).toBeInTheDocument();
  });
});

describe("avisos", () => {
  it("mostra texto de aviso desconhecido sem quebrar", () => {
    const a = analiseDe("normal");
    a.avisos = [{ codigo: "coisa_nova" }, { codigo: "boot_sem_hora", detalhe: "boot 7" }];
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    expect(screen.getByText("Aviso: coisa_nova")).toBeInTheDocument();
    expect(
      screen.getByText("Um boot ficou de fora por não ter hora absoluta (boot 7)"),
    ).toBeInTheDocument();
  });

  it("mostra todos os avisos conhecidos", () => {
    const a = analiseDe("normal");
    a.avisos = [
      "partes_sobrepostas",
      "motor_nunca_ligado",
      "consumo_pode_estar_subestimado",
      "marcha_lenta_reserva",
    ].map((codigo) => ({ codigo }));
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
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

describe("sinal ausente", () => {
  it("gráfico sem leituras de um sinal mostra 'Sem leituras' e indicador —", () => {
    const a = analiseDe("normal");
    a.series.rpm = a.series.rpm.map(() => null);
    a.marchas.subida_rpm.mediana = null;
    renderComTema(<AnaliseView analise={a} provisoria={false} />);
    expect(
      screen.getByText(/Rotação do motor ao longo da viagem: Sem leituras/),
    ).toBeInTheDocument();
    expect(screen.getByText("Rotação × velocidade").closest("section")).toHaveTextContent(
      "Sem leituras",
    );
  });
});
