import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderComTema } from "@/testes/helpers";
import {
  FormularioProcessar,
  montarCorpo,
  paraRfc3339,
  validarCampos,
} from "./FormularioProcessar";

type Resp = { status: number; corpo?: unknown };

function fetchCom(resp: Resp | (() => Promise<Resp>)) {
  const f = vi.fn(async () => {
    const r = typeof resp === "function" ? await resp() : resp;
    return { status: r.status, ok: r.status < 300, json: async () => r.corpo };
  });
  vi.stubGlobal("fetch", f);
  return f;
}

afterEach(() => vi.unstubAllGlobals());

async function preencherBoot(nome = "João", boot = "8") {
  await userEvent.type(screen.getByLabelText("Motorista"), nome);
  await userEvent.type(screen.getByRole("spinbutton", { name: "Boot" }), boot);
}
const enviar = () => userEvent.click(screen.getByRole("button", { name: "Processar viagem" }));

describe("valores iniciais", () => {
  it("categoria feira, E30, dongle 1 e os 6 presets", () => {
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    expect(screen.getByLabelText("Categoria")).toHaveValue("feira");
    expect(screen.getByLabelText("Dongle")).toHaveValue(1);
    const combustivel = screen.getByLabelText("Combustível") as HTMLSelectElement;
    expect(combustivel.value).toBe("gasolina_e30");
    expect(Array.from(combustivel.options).map((o) => o.text)).toEqual([
      "Gasolina E30",
      "Gasolina E32",
      "Gasolina E27",
      "Gasolina pura",
      "Etanol hidratado",
      "Diesel",
    ]);
  });
});

describe("validação no cliente", () => {
  it("campos vazios não enviam", async () => {
    const f = fetchCom({ status: 202 });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await userEvent.clear(screen.getByLabelText("Categoria"));
    await userEvent.clear(screen.getByLabelText("Dongle"));
    await enviar();
    expect(screen.getByText("Informe o nome do motorista.")).toBeInTheDocument();
    expect(screen.getByText("Informe a categoria.")).toBeInTheDocument();
    expect(screen.getByText("Informe o número do dongle.")).toBeInTheDocument();
    expect(screen.getByText("Informe o boot.")).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Motorista")).toHaveAttribute("aria-invalid", "true");
  });

  it("intervalo exige datas e t1 depois de t0", async () => {
    const f = fetchCom({ status: 202 });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Motorista"), "Ana");
    await userEvent.click(screen.getByLabelText("Intervalo de tempo"));
    await enviar();
    expect(screen.getByText("Informe o início.")).toBeInTheDocument();
    expect(screen.getByText("Informe o fim.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Início"), { target: { value: "2026-10-28T10:00" } });
    fireEvent.change(screen.getByLabelText("Fim"), { target: { value: "2026-10-28T09:00" } });
    await enviar();
    expect(screen.getByText("O fim deve ser depois do início.")).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
  });

  it("funções puras", () => {
    expect(paraRfc3339("2026-10-28T08:00")).toBe("2026-10-28T08:00:00-03:00");
    expect(paraRfc3339("2026-10-28T08:00:30")).toBe("2026-10-28T08:00:30-03:00");
    const base = {
      motorista: " Ana ",
      categoria: " Feira ",
      combustivel: "diesel",
      dongle: "1",
      tipo: "boot" as const,
      bootId: "8",
      bootGeneration: "2",
      t0: "",
      t1: "",
    };
    expect(montarCorpo(base)).toEqual({
      motorista: { nome: "Ana" },
      categoria_viagem: "Feira",
      combustivel: "diesel",
      entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 2 },
    });
    expect(validarCampos({ ...base, bootGeneration: "x" }).bootGeneration).toBeTruthy();
    expect(montarCorpo({ ...base, bootGeneration: "" }).entrada).not.toHaveProperty(
      "boot_generation",
    );
    expect(
      montarCorpo({ ...base, tipo: "intervalo", t0: "2026-10-28T08:00", t1: "2026-10-28T18:00" })
        .entrada,
    ).toEqual({
      tipo: "intervalo",
      dongle_id: 1,
      t0: "2026-10-28T08:00:00-03:00",
      t1: "2026-10-28T18:00:00-03:00",
    });
  });
});

describe("envio", () => {
  it("202 vai para /viagem/<id> e envia o corpo esperado", async () => {
    const f = fetchCom({ status: 202, corpo: { viagem_id: "abc", status: "pendente" } });
    const irPara = vi.fn();
    renderComTema(<FormularioProcessar irPara={irPara} />);
    await preencherBoot("  João ", "8");
    await enviar();
    await waitFor(() => expect(irPara).toHaveBeenCalledWith("/viagem/abc"));
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/viagens");
    expect(JSON.parse(init.body as string)).toEqual({
      motorista: { nome: "João" },
      categoria_viagem: "feira",
      combustivel: "gasolina_e30",
      entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
    });
    // Fica desativado durante a navegação.
    expect(screen.getByRole("button", { name: "Enviando…" })).toBeDisabled();
  });

  it("envia intervalo com fuso -03:00", async () => {
    const f = fetchCom({ status: 202, corpo: { viagem_id: "x" } });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Motorista"), "Ana");
    await userEvent.click(screen.getByLabelText("Intervalo de tempo"));
    fireEvent.change(screen.getByLabelText("Início"), { target: { value: "2026-10-28T08:00" } });
    fireEvent.change(screen.getByLabelText("Fim"), { target: { value: "2026-10-28T18:00" } });
    await enviar();
    await waitFor(() => expect(f).toHaveBeenCalled());
    const init = (f.mock.calls as unknown as [string, RequestInit][])[0][1];
    expect(JSON.parse(init.body as string).entrada).toMatchObject({
      tipo: "intervalo",
      t0: "2026-10-28T08:00:00-03:00",
    });
  });

  it("409 aparece no campo motorista e não apaga o resto", async () => {
    fetchCom({ status: 409, corpo: { codigo: "motorista_repetido" } });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await preencherBoot("João", "8");
    await userEvent.selectOptions(screen.getByLabelText("Combustível"), "diesel");
    await enviar();
    expect(
      await screen.findByText(
        "Já existe uma viagem de «João» na categoria «feira». Use outro nome.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Motorista")).toHaveValue("João");
    expect(screen.getByRole("spinbutton", { name: "Boot" })).toHaveValue(8);
    expect(screen.getByLabelText("Combustível")).toHaveValue("diesel");
    expect(screen.getByRole("button", { name: "Processar viagem" })).toBeEnabled();
  });

  it("400 mostra o detalhe acima do botão", async () => {
    fetchCom({
      status: 400,
      corpo: { codigo: "entrada_invalida", detail: "entrada.boot_id: de 1 a 65535" },
    });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await preencherBoot("João", "70000");
    await enviar();
    const aviso = await screen.findByText("entrada.boot_id: de 1 a 65535");
    const botao = screen.getByRole("button", { name: "Processar viagem" });
    expect(aviso.compareDocumentPosition(botao) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("404 dongle_desconhecido aparece no campo dongle", async () => {
    fetchCom({ status: 404, corpo: { codigo: "dongle_desconhecido" } });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await preencherBoot();
    await userEvent.clear(screen.getByLabelText("Dongle"));
    await userEvent.type(screen.getByLabelText("Dongle"), "2");
    await enviar();
    expect(await screen.findByText("Dongle desconhecido.")).toBeInTheDocument();
  });

  it("401 vai para /entrar", async () => {
    fetchCom({ status: 401 });
    const irPara = vi.fn();
    renderComTema(<FormularioProcessar irPara={irPara} />);
    await preencherBoot();
    await enviar();
    await waitFor(() => expect(irPara).toHaveBeenCalledWith("/entrar?retorno=%2Fprocessar"));
  });

  it("outro erro e falha de rede mostram a mensagem genérica", async () => {
    fetchCom({ status: 500 });
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await preencherBoot();
    await enviar();
    expect(
      await screen.findByText("Não foi possível criar a viagem. Tente de novo."),
    ).toBeInTheDocument();

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Promise.reject(new Error("rede"))),
    );
    await enviar();
    expect(
      await screen.findByText("Não foi possível criar a viagem. Tente de novo."),
    ).toBeInTheDocument();
  });

  it("impede envio duplo enquanto aguarda", async () => {
    let liberar: (r: Resp) => void = () => undefined;
    const f = fetchCom(() => new Promise<Resp>((res) => (liberar = res)));
    renderComTema(<FormularioProcessar irPara={vi.fn()} />);
    await preencherBoot();
    const botao = screen.getByRole("button", { name: "Processar viagem" });
    await userEvent.click(botao);
    expect(botao).toBeDisabled();
    fireEvent.submit(botao.closest("form")!);
    fireEvent.submit(botao.closest("form")!);
    expect(f).toHaveBeenCalledTimes(1);
    liberar({ status: 500 });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Processar viagem" })).toBeEnabled(),
    );
  });
});
