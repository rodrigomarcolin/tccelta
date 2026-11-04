import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StatusResposta, StatusViagem } from "@/contrato/tipos";
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

type Resp = { status: number; corpo?: unknown; retryAfter?: string };
type Saida = Resp | Error | Promise<Resp | Error>;
type Roteiro = {
  status: () => Saida;
  analise: () => Saida;
};

const ID = "7d1f3a10-0001-4c5e-9b1a-000000000008";
const chamadas: string[] = [];
const sinais: AbortSignal[] = [];

function instalar(roteiro: Roteiro) {
  chamadas.length = 0;
  sinais.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      chamadas.push(url);
      if (init?.signal) sinais.push(init.signal);
      const r = await (url.endsWith("/analise") ? roteiro.analise() : roteiro.status());
      if (r instanceof Error) throw r;
      return {
        status: r.status,
        ok: r.status >= 200 && r.status < 300,
        json: async () => r.corpo,
        headers: new Headers(r.retryAfter ? { "Retry-After": r.retryAfter } : {}),
      };
    }),
  );
}

const status = (
  s: StatusViagem,
  provisoria = false,
  calculada: string | null = null,
): StatusResposta => ({
  viagem_id: ID,
  status: s,
  provisoria,
  motorista: { nome: "Maria" },
  categoria_viagem: "feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 0 },
  criada_em: "2026-10-28T17:00:00.000Z",
  calculada_em: calculada,
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

let oculta = false;
const definirOculta = (v: boolean) => {
  oculta = v;
  document.dispatchEvent(new Event("visibilitychange"));
};

beforeEach(() => {
  vi.useFakeTimers();
  oculta = false;
  Object.defineProperty(document, "hidden", { configurable: true, get: () => oculta });
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const AVISO_REDE = "Sem conexão com o servidor. Tentando novamente…";

describe("espera e polling de 1 s", () => {
  it("mostra a espera com o nome e abre a análise sem recarregar ao ficar pronta", async () => {
    let atual: StatusViagem = "pendente";
    instalar({
      status: () => ({ status: 200, corpo: status(atual) }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    expect(document.body.querySelector(".ponto-pulsar")).not.toBeNull();
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

  it("não sobrepõe ciclos: espera a resposta lenta antes de agendar o próximo", async () => {
    instalar({
      status: () =>
        new Promise((ok) => setTimeout(() => ok({ status: 200, corpo: status("pendente") }), 3500)),
      analise: () => ({ status: 409 }),
    });
    await montar();
    await avancar(3000);
    expect(contar("status")).toBe(1);
    await avancar(500);
    await avancar(999);
    expect(contar("status")).toBe(1);
    await avancar(1);
    expect(contar("status")).toBe(2);
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

  it("404: Viagem não encontrada, com link, e o polling para", async () => {
    instalar({
      status: () => ({ status: 404, corpo: { codigo: "viagem_desconhecida" } }),
      analise: () => ({ status: 404 }),
    });
    await montar();
    expect(screen.getByText("Viagem não encontrada")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Voltar para processar/ })).toHaveAttribute(
      "href",
      "/processar",
    );
    await avancar(5000);
    expect(contar("status")).toBe(1);
  });

  it("401 no status leva a /entrar com o retorno", async () => {
    instalar({ status: () => ({ status: 401 }), analise: () => ({ status: 401 }) });
    const irPara = await montar();
    expect(irPara).toHaveBeenCalledWith(`/entrar?retorno=${encodeURIComponent(`/viagem/${ID}`)}`);
    await avancar(5000);
    expect(contar("status")).toBe(1);
  });

  it("401 na análise leva a /entrar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 401 }),
    });
    const irPara = await montar();
    expect(irPara).toHaveBeenCalledTimes(1);
  });

  it("resposta inesperada (400 sem contrato): mensagem e 'Tentar de novo' recarrega", async () => {
    let falhar = true;
    instalar({
      status: () => (falhar ? { status: 400 } : { status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    expect(screen.getByText("Não foi possível carregar a viagem.")).toBeInTheDocument();
    await avancar(5000);
    expect(contar("status")).toBe(1);
    falhar = false;
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    });
    await avancar(0);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
  });

  it("análise com resposta inesperada mostra a mensagem", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 400 }),
    });
    await montar();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });

  it("status 200 com corpo inválido mostra a mensagem", async () => {
    instalar({ status: () => ({ status: 200, corpo: null }), analise: () => ({ status: 409 }) });
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
});

describe("rede e recuo", () => {
  it("falhas seguidas recuam 1, 2, 4 e 5 s, avisam na terceira e o sucesso volta a 1 s", async () => {
    let rede = true;
    instalar({
      status: () => (rede ? new Error("rede") : { status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    await montar();
    expect(contar("status")).toBe(1);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    await avancar(1000);
    expect(contar("status")).toBe(2);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    await avancar(1999);
    expect(contar("status")).toBe(2);
    await avancar(1);
    expect(contar("status")).toBe(3);
    expect(screen.getByText(AVISO_REDE)).toBeInTheDocument();
    // Próximo em 4 s e depois em 5 s (teto).
    await avancar(3999);
    expect(contar("status")).toBe(3);
    await avancar(1);
    expect(contar("status")).toBe(4);
    await avancar(4999);
    expect(contar("status")).toBe(4);
    await avancar(1);
    expect(contar("status")).toBe(5);

    rede = false;
    await avancar(5000);
    expect(contar("status")).toBe(6);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await avancar(1000);
    expect(contar("status")).toBe(7);
  });

  it("5xx conta como falha e o aviso não esvazia a análise já exibida", async () => {
    let queda = false;
    instalar({
      status: () =>
        queda ? { status: 503 } : { status: 200, corpo: status("pronta", true, "c1") },
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    queda = true;
    await avancar(5000);
    await avancar(1000);
    await avancar(2000);
    expect(screen.getByText(AVISO_REDE)).toBeInTheDocument();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    queda = false;
    await avancar(4000);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
  });

  it("análise com 5xx volta a tentar e resposta inesperada com análise na tela não a apaga", async () => {
    let analiseOk = false;
    instalar({
      status: () => ({ status: 200, corpo: status("pronta", true, analiseOk ? "c2" : "c1") }),
      analise: () => (analiseOk ? { status: 400 } : { status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    analiseOk = true;
    await avancar(5000);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tentar de novo" })).toBeNull();
  });

  it("o evento online força uma tentativa imediata", async () => {
    instalar({ status: () => new Error("rede"), analise: () => ({ status: 409 }) });
    await montar();
    await avancar(1000);
    await avancar(2000);
    await avancar(3000);
    const antes = contar("status");
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(contar("status")).toBe(antes + 1);
  });

  it("429 respeita Retry-After no polling e avisa", async () => {
    let ocupado = true;
    instalar({
      status: () =>
        ocupado
          ? { status: 429, corpo: { codigo: "limite_taxa" }, retryAfter: "5" }
          : { status: 200, corpo: status("pendente") },
      analise: () => ({ status: 409 }),
    });
    await montar();
    expect(screen.getByText("Servidor ocupado. Tentando de novo em 5 s.")).toBeInTheDocument();
    await avancar(4999);
    expect(contar("status")).toBe(1);
    ocupado = false;
    await avancar(1);
    expect(contar("status")).toBe(2);
    expect(screen.queryByText(/Servidor ocupado/)).toBeNull();
  });

  it("429 na análise também adia", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 429, retryAfter: "2" }),
    });
    await montar();
    expect(screen.getByText("Servidor ocupado. Tentando de novo em 2 s.")).toBeInTheDocument();
    await avancar(2000);
    expect(contar("status")).toBe(2);
  });
});

describe("aba oculta", () => {
  it("pausa ao ocultar e consulta na hora ao voltar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    await montar();
    expect(contar("status")).toBe(1);
    act(() => definirOculta(true));
    await avancar(10_000);
    expect(contar("status")).toBe(1);
    await act(async () => {
      definirOculta(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(contar("status")).toBe(2);
    await avancar(1000);
    expect(contar("status")).toBe(3);
  });

  it("ciclo que termina com a aba oculta fica adiado até voltar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    oculta = true;
    await montar();
    await avancar(5000);
    expect(contar("status")).toBe(1);
    await act(async () => {
      definirOculta(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(contar("status")).toBe(2);
  });

  it("voltar a ficar visível sem ciclo adiado não consulta", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    await montar();
    act(() => definirOculta(false));
    await avancar(0);
    expect(contar("status")).toBe(1);
  });

  it("depois de parar, ocultar, voltar e ficar online não consulta", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta") }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    act(() => definirOculta(true));
    await act(async () => {
      definirOculta(false);
      window.dispatchEvent(new Event("online"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(contar("status")).toBe(1);
  });
});

describe("limite de espera de 60 s", () => {
  it("para, explica, e 'Continuar esperando' retoma por mais 60 s", async () => {
    let atual: StatusViagem = "pendente";
    instalar({
      status: () => ({ status: 200, corpo: status(atual) }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    await avancar(59_000);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await avancar(1000);
    expect(
      screen.getByText(
        "A viagem ainda não foi processada. O processamento no servidor pode estar parado.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Voltar/ })).toHaveAttribute("href", "/processar");
    const parado = contar("status");
    await avancar(10_000);
    expect(contar("status")).toBe(parado);

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Continuar esperando" }));
    });
    await avancar(0);
    expect(contar("status")).toBe(parado + 1);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await avancar(30_000);
    atual = "aguardando_dados";
    await avancar(29_000);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await avancar(1000);
    expect(screen.getByText(/ainda não foi processada/)).toBeInTheDocument();

    // O worker volta: continuar esperando conclui.
    atual = "pronta";
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Continuar esperando" }));
    });
    await avancar(0);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
  });
});

describe("viagem provisória", () => {
  it("consulta a cada 5 s e troca 'Aguardando dados' pela análise sozinha", async () => {
    let completa = false;
    instalar({
      status: () => ({
        status: 200,
        corpo: status(
          "pronta",
          !completa,
          completa ? "2026-10-28T17:05:00.000Z" : "2026-10-28T17:00:20.000Z",
        ),
      }),
      analise: () => ({
        status: 200,
        corpo: analiseDe("provisoria_aguardando", completa),
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
    expect(screen.getByText("Resultado atualizado às 14:05")).toBeInTheDocument();
    expect(contar("analise")).toBe(2);
    const antes = contar("status");
    await avancar(20_000);
    expect(contar("status")).toBe(antes);
  });

  it("calculada_em nova troca a análise sem tela em branco e avisa a hora", async () => {
    let versao = 1;
    instalar({
      status: () => ({
        status: 200,
        corpo: status(
          "pronta",
          true,
          versao === 1 ? "2026-10-28T17:00:20.000Z" : "2026-10-28T17:05:00.000Z",
        ),
      }),
      analise: () => ({ status: 200, corpo: analiseDe("normal", versao === 2) }),
    });
    await montar();
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(screen.queryByText(/Resultado atualizado/)).toBeNull();

    await avancar(5000);
    expect(contar("analise")).toBe(1);
    versao = 2;
    await avancar(5000);
    expect(contar("analise")).toBe(2);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    expect(screen.getByText("Resultado atualizado às 14:05")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
  });

  it("provisoria false sem mudar calculada_em só remove a faixa", async () => {
    let provisoria = true;
    instalar({
      status: () => ({
        status: 200,
        corpo: status("pronta", provisoria, "2026-10-28T17:00:20.000Z"),
      }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    expect(screen.getByText(/Resultado provisório/)).toBeInTheDocument();
    provisoria = false;
    await avancar(5000);
    expect(screen.queryByText(/Resultado provisório/)).toBeNull();
    expect(screen.queryByText(/Resultado atualizado/)).toBeNull();
    expect(contar("analise")).toBe(1);
    expect(screen.getByTestId("mapa")).toBeInTheDocument();
    await avancar(20_000);
    expect(contar("status")).toBe(2);
  });

  it("a consulta de 5 s só roda com a aba visível", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta", true, "c") }),
      analise: () => ({ status: 200, corpo: analiseDe("normal") }),
    });
    await montar();
    act(() => definirOculta(true));
    await avancar(30_000);
    expect(contar("status")).toBe(1);
    await act(async () => {
      definirOculta(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(contar("status")).toBe(2);
  });

  it("para depois de 5 min e passa a pedir recarga", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pronta", true, "c") }),
      analise: () => ({ status: 200, corpo: analiseDe("dados_parciais", false) }),
    });
    await montar();
    await avancar(5 * 60 * 1000 + 6000);
    const total = contar("status");
    expect(total).toBeGreaterThan(30);
    expect(total).toBeLessThanOrEqual(62);
    await avancar(30_000);
    expect(contar("status")).toBe(total);
    expect(screen.getByText("Resultado provisório. Recarregue para verificar")).toBeInTheDocument();
  });
});

describe("cancelamento", () => {
  it("desmontar cancela a requisição em voo", async () => {
    instalar({ status: () => new Promise(() => undefined), analise: () => ({ status: 409 }) });
    const { unmount } = renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
    await avancar(0);
    expect(sinais[0].aborted).toBe(false);
    unmount();
    expect(sinais[0].aborted).toBe(true);
  });

  it("um timer pendente não dispara depois de desmontar", async () => {
    instalar({
      status: () => ({ status: 200, corpo: status("pendente") }),
      analise: () => ({ status: 409 }),
    });
    const { unmount } = renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
    await avancar(0);
    unmount();
    await avancar(10_000);
    expect(contar("status")).toBe(1);
  });

  it("falha que chega depois de desmontar não agenda nada", async () => {
    let falhar: (e: Error) => void = () => undefined;
    instalar({
      status: () => new Promise((_ok, no) => (falhar = no)),
      analise: () => ({ status: 409 }),
    });
    const { unmount } = renderComTema(<ViagemPagina id={ID} irPara={vi.fn()} />);
    await avancar(0);
    unmount();
    falhar(new Error("abortada"));
    await avancar(10_000);
    expect(contar("status")).toBe(1);
  });
});
