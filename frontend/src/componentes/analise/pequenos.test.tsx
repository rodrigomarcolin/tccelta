import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  EstadoAguardandoDados,
  EstadoCarregando,
  EstadoErro,
  EstadoEspera,
  EstadoFalhou,
  EstadoNaoEncontrada,
  EstadoSemDados,
  FaixaProvisoria,
} from "./Estados";
import { BarrasHorizontais, Dispersao, MapaDeCalor } from "./graficos-svg";
import { TooltipInstante } from "./Instante";
import { picoTexto } from "./TabelaEventos";
import { textoAviso } from "./textos";
import { corCss, corFaixa, corMarcha, varFaixa, varMarcha } from "./cores";
import { TRACO } from "@/formato/numeros";

vi.mock("next/link", () => ({
  default: ({ href, children, ...r }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...r}>
      {children}
    </a>
  ),
}));

describe("estados", () => {
  it("espera com e sem nome", () => {
    const { rerender } = render(<EstadoEspera motorista="Ana" />);
    expect(screen.getByText("Processando a viagem de «Ana»")).toBeInTheDocument();
    rerender(<EstadoEspera motorista={null} />);
    expect(screen.getByText("Processando a viagem")).toBeInTheDocument();
  });
  it("demais estados", async () => {
    render(
      <>
        <EstadoCarregando />
        <EstadoFalhou />
        <EstadoNaoEncontrada />
        <EstadoSemDados />
        <EstadoAguardandoDados />
        <FaixaProvisoria />
      </>,
    );
    expect(screen.getByLabelText("Carregando a análise")).toBeInTheDocument();
    expect(screen.getByText(/O nome do motorista foi liberado/)).toBeInTheDocument();
    expect(screen.getByText("Viagem não encontrada")).toBeInTheDocument();
    expect(screen.getByText("Sem dados para esta viagem")).toBeInTheDocument();
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText(/Resultado provisório/)).toBeInTheDocument();
  });
  it("erro chama 'tentar de novo'", async () => {
    const onTentar = vi.fn();
    render(<EstadoErro onTentar={onTentar} />);
    await userEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    expect(onTentar).toHaveBeenCalled();
  });
});

describe("textoAviso", () => {
  it("traz o texto de cada código conhecido", () => {
    for (const codigo of [
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
      expect(textoAviso({ codigo })).not.toMatch(/^Aviso:/);
    }
  });
  it("boot_sem_hora com e sem detalhe, e código desconhecido", () => {
    expect(textoAviso({ codigo: "boot_sem_hora", detalhe: "boot 7" })).toBe(
      "Um boot ficou de fora por não ter hora absoluta (boot 7)",
    );
    expect(textoAviso({ codigo: "boot_sem_hora" })).toBe(
      "Um boot ficou de fora por não ter hora absoluta",
    );
    expect(textoAviso({ codigo: "xyz" })).toBe("Aviso: xyz");
  });
});

describe("TooltipInstante", () => {
  it("mostra traço onde o valor é null", () => {
    render(
      <TooltipInstante
        instante={{ t: 1, velocidade: null, rpm: null, marcha: null, pedal: null }}
        formatarT={() => "14:03:22"}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(TRACO);
    expect(screen.getAllByText(TRACO).length).toBeGreaterThanOrEqual(3);
  });
  it("marcha 0 tem rótulo próprio, sem número", () => {
    render(
      <TooltipInstante
        instante={{ t: 1, velocidade: 40, rpm: 900, marcha: 0, pedal: 0 }}
        formatarT={() => "14:03:22"}
      />,
    );
    expect(screen.getByText("Ponto morto ou embreagem")).toBeInTheDocument();
  });
});

describe("TabelaEventos.picoTexto", () => {
  it("usa m/s² ou segundos", () => {
    const base = {
      tipo: "frenagem_brusca" as const,
      t_inicio_ms: 0,
      duracao_s: 1,
      velocidade_inicial_kmh: 1,
      velocidade_final_kmh: 1,
      lat: null,
      lon: null,
    };
    expect(picoTexto({ ...base, pico: -6.8, pico_unidade: "m/s2" })).toBe("-6,8 m/s²");
    expect(picoTexto({ ...base, tipo: "marcha_lenta_longa", pico: 64.5, pico_unidade: "s" })).toBe(
      "1 min 5 s",
    );
  });
});

describe("cores", () => {
  it("lê variáveis CSS do tema e usa fallback", () => {
    document.documentElement.style.setProperty("--faixa-30_60", "#b44d2c");
    expect(corFaixa("30_60")).toBe("#b44d2c");
    expect(corCss("--nao-existe", "#123456")).toBe("#123456");
    expect(varFaixa("falha_gps")).toBe("--faixa-falha");
    expect(varFaixa("sem_velocidade")).toBe("--faixa-falha");
    expect(varFaixa("60_80")).toBe("--faixa-60_80");
    expect(varMarcha(0)).toBe("--marcha-0");
    expect(varMarcha(3)).toBe("--marcha-3");
    expect(varMarcha(9)).toBe("--marcha-6");
    expect(corMarcha(0)).toBe("#888888");
  });
});

describe("graficos-svg", () => {
  it("BarrasHorizontais: % e tempo, e traço sem valor", () => {
    render(
      <BarrasHorizontais
        titulo="Tempo"
        itens={[
          { rotulo: "1ª", pct: 50, tempoS: 30, cor: "--marcha-1" },
          { rotulo: "2ª", pct: null, tempoS: null, cor: "#fff" },
          { rotulo: "3ª", pct: 10, tempoS: 6 },
        ]}
      />,
    );
    expect(screen.getByRole("list", { name: "Tempo" })).toBeInTheDocument();
    expect(screen.getByText("50% · 30 s")).toBeInTheDocument();
    expect(screen.getByText("— · —")).toBeInTheDocument();
  });

  it("Dispersao: pontos clicáveis por mouse e teclado, linhas e traços", async () => {
    const onClick = vi.fn();
    render(
      <Dispersao
        titulo="Teste"
        pontos={[
          { x: 1, y: 1, cor: "--accent" },
          { x: 2, y: 2, cor: "#fff", rotulo: "Ponto A", onClick },
        ]}
        linhas={[
          { x1: 0, y1: 0, x2: 4, y2: 4, cor: "--marcha-1", rotulo: "1ª" },
          { x1: 0, y1: 0, x2: 4, y2: 2, cor: "#fff" },
        ]}
        tracos={[
          { x1: 0, x2: 1, y: 2, rotulo: "mediana" },
          { x1: 0, x2: 1, y: 3 },
        ]}
        dominioX={[0, 4]}
        dominioY={[0, 4]}
        ticksX={[{ v: 0, rotulo: "0" }]}
        ticksY={[{ v: 0, rotulo: "0" }]}
        rotuloX="x"
        rotuloY="y"
      />,
    );
    const ponto = screen.getByRole("button", { name: "Ponto A" });
    await userEvent.click(ponto);
    ponto.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard(" ");
    await userEvent.keyboard("a");
    expect(onClick).toHaveBeenCalledTimes(3);
    expect(screen.getByText("mediana")).toBeInTheDocument();
  });

  it("MapaDeCalor: só mostra as faixas com tempo, com legenda", () => {
    const { container } = render(
      <MapaDeCalor
        rpmBin={500}
        pedalBin={10}
        tempoS={[
          [0, 0],
          [10, 0],
          [0, 5],
          [0, 0],
        ]}
      />,
    );
    expect(container.querySelectorAll("rect")).toHaveLength(4);
    expect(screen.getByText(/Cor: tempo/)).toBeInTheDocument();
    expect(container.querySelector("rect")?.getAttribute("style")).toContain("rgba(var(--calor)");
  });

  it("MapaDeCalor sem tempo mostra 'Sem leituras'", () => {
    render(<MapaDeCalor rpmBin={500} pedalBin={10} tempoS={[]} />);
    expect(screen.getByText("Sem leituras")).toBeInTheDocument();
  });
});
