import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Analise, StatusResposta, StatusViagem } from "@/contrato/tipos";
import { analiseDe, renderComTema } from "@/testes/helpers";
import { ViagemPagina } from "./ViagemPagina";

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

type Resp = { status: number; corpo?: unknown };
type Roteiro = {
  status: () => Resp | Error;
  analise: () => Resp | Error;
};

const ID = "7d1f3a10-0001-4c5e-9b1a-000000000008";
const chamadas: string[] = [];

function instalar(roteiro: Roteiro) {
  chamadas.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      chamadas.push(url);
      const r = url.endsWith("/analise") ? roteiro.analise() : roteiro.status();
      if (r instanceof Error) throw r;
      return { status: r.status, ok: r.status >= 200 && r.status < 300, json: async () => r.corpo };
    }),
  );
}

const status = (s: StatusViagem, provisoria = false): StatusResposta => ({
  viagem_id: ID,
  status: s,
  provisoria,
  motorista: { nome: "Maria" },
  categoria_viagem: "feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 0 },
  criada_em: "2026-10-28T17:00:00.000Z",
  calculada_em: null,
});

const avancar = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
const contar = (sufixo: "status" | "analise") =>
  chamadas.filter((u) => (sufixo === "analise" ? u.endsWith("/analise") : !u.endsWith("/analise")))
    .length;

async function montar(irPara = vi.fn()) {
  renderComTema(<ViagemPagina id={ID} irPara={irPara} />);
  await avancar(0);
  return irPara;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("espera e polling de 1 s", () => {
  it("mostra a espera com o nome e abre a análise sem recarregar ao ficar pronta", async () => {
    let atual: StatusViagem = "pendente";
    instalar({
      status: () => ({ status: 200, corpo: status(atual) }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    const { container } = await (async () => {
      renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
      await avancar(0);
      return { container: document.body };
    })();
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    expect(container.querySelector(".ponto-pulsar")).not.toBeNull();
    expect(screen.queryByTestId("mapa")).toBeNull();
    expect(contar("analise")).toBe(0);

    atual = "aguardando_dados";
    await avancar(1000);
    expect(contar("status")).toBe(2);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await avancar(1000);
    expect(contar("status")).toBe(3);

    atual = "pronta";
    await avancar(1000);
    expect(screen.getByText("Dongle 1 · boot 8")).toBeInTheDocument();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(contar("analise")).toBe(1);

    // Pronta e completa: o polling para.
    await avancar(10_000);
    expect(contar("status")).toBe(4);
  });

  it("continua esperando se a análise ainda responde 409", async () => {
    let analiseOk = false;
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () =>
        analiseOk
          ? { status: 200, corpo: analiseDe("normal") }
          : {
              status: 409,
              corpo: { codigo: "analise_indisponivel", viagem_status: "aguardando_dados" },
            },
    });
    await montar();
    expect(screen.queryByTestId("mapa")).toBeNull();
    analiseOk = true;
    await avancar(1000);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
  });
});

describe("outros estados", () => {
  it("falhou: mensagem e nome liberado", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("falhou") }),
      analise: () => ({ status: 409 }),
    });
    await montar();
    expect(
      screen.getByText("Não foi possível calcular esta viagem. O nome do motorista foi liberado."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Processar outra viagem/ })).toHaveAttribute(
      "href",
      "/processar",
    );
    await avancar(5000);
    expect(contar("status")).toBe(1);
  });

  it("404: Viagem não encontrada", async () => {
    instalar({
      status: () => ({ status: 404, corpo: { codigo: "viagem_desconhecida" } }),
      analise: () => ({ status: 404 }),
    });
    await montar();
    expect(screen.getByText("Viagem não encontrada")).toBeInTheDocument();
  });

  it("401 no status leva a /entrar com o retorno", async () => {
    instalar({ status: () => ({ status: 401 }), analise: () => ({ status: 401 }) });
    const irPara = await montar();
    expect(irPara).toHaveBeenCalledWith(`/entrar?retorno=${encodeURIComponent(`/viagem/${ID}`)}`);
  });

  it("401 na análise leva a /entrar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 401 }),
    });
    const irPara = await montar();
    expect(irPara).toHaveBeenCalledTimes(1);
  });

  it("erro de rede: mensagem e 'Tentar de novo' recarrega", async () => {
    let falhar = true;
    instalar({
      status: () => (falhar ? new Error("rede") : { status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    expect(screen.getByText("Não foi possível carregar a viagem.")).toBeInTheDocument();
    falhar = false;
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    });
    await avancar(0);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
  });

  it("status com erro do servidor e análise com erro mostram a mensagem", async () => {
    instalar({ status: () => ({ status: 500 }), analise: () => ({ status: 200 }) });
    await montar();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });

  it("análise com erro do servidor mostra a mensagem", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 500 }),
    });
    await montar();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });

  it("mostra a estrutura de espera antes da primeira resposta", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => undefined)),
    );
    renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
    expect(screen.getByLabelText("Carregando a análise")).toBeInTheDocument();
  });

  it("para de consultar ao desmontar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    const { unmount } = renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
    await avancar(0);
    unmount();
    await avancar(5000);
    expect(contar("status")).toBe(1);
  });
});

describe("viagem provisória", () => {
  it("consulta a cada 5 s e troca 'Aguardando dados' pela análise sozinha", async () => {
    let completa = false;
    instalar({
      status: () => ({ status: 200, corpo: status("pronta", !completa) }),
      analise: () => ({
        status: 200,
        corpo: completa
          ? analiseDe("provisoria_aguardando", true)
          : analiseDe("provisoria_aguardando", false),
      }),
    });
    await montar();
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(contar("status")).toBe(1);

    await avancar(4000);
    expect(contar("status")).toBe(1);
    await avancar(1000);
    expect(contar("status")).toBe(2);
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();

    completa = true;
    await avancar(5000);
    expect(screen.queryByText("Aguardando dados")).toBeNull();
    expect(screen.queryByText("Resultado provisório: ainda podem chegar dados")).toBeNull();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(contar("analise")).toBe(2);
    const antes = contar("status");
    await avancar(20_000);
    expect(contar("status")).toBe(antes);
  });

  it("para depois de 5 min", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta", true) }),
      analise: () => ({ status: 200, corpo: analiseDe("dados_parciais", false) as Analise }),
    });
    await montar();
    await avancar(5 * 60 * 1000 + 6000);
    const total = contar("status");
    expect(total).toBeGreaterThan(30);
    expect(total).toBeLessThanOrEqual(62);
    await avancar(30_000);
    expect(contar("status")).toBe(total);
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
  });
});
