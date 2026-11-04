import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithTheme } from "@/tests/helpers";
import { creationError, ProcessForm, buildBody, toRfc3339, validateFields } from "./ProcessForm";

type Resp = { status: number; body?: unknown; retryAfter?: string };

function fetchWith(resp: Resp | (() => Promise<Resp>)) {
  const f = vi.fn(async () => {
    const r = typeof resp === "function" ? await resp() : resp;
    return {
      status: r.status,
      ok: r.status < 300,
      json: async () => r.body,
      headers: new Headers(r.retryAfter ? { "Retry-After": r.retryAfter } : {}),
    };
  });
  vi.stubGlobal("fetch", f);
  return f;
}

afterEach(() => vi.unstubAllGlobals());

async function fillBoot(name = "João", boot = "8") {
  await userEvent.type(screen.getByLabelText("Motorista"), name);
  await userEvent.type(screen.getByRole("spinbutton", { name: "Boot" }), boot);
}
const submit = () => userEvent.click(screen.getByRole("button", { name: "Processar viagem" }));

describe("initial values", () => {
  it("category feira, E30, dongle 1 and the 6 presets", () => {
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    expect(screen.getByLabelText("Categoria")).toHaveValue("feira");
    expect(screen.getByLabelText("Dongle")).toHaveValue(1);
    const fuel = screen.getByLabelText("Combustível") as HTMLSelectElement;
    expect(fuel.value).toBe("gasolina_e30");
    expect(Array.from(fuel.options).map((o) => o.text)).toEqual([
      "Gasolina E30",
      "Gasolina E32",
      "Gasolina E27",
      "Gasolina pura",
      "Etanol hidratado",
      "Diesel",
    ]);
  });
});

describe("client-side validation", () => {
  it("empty fields are not sent", async () => {
    const f = fetchWith({ status: 202 });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await userEvent.clear(screen.getByLabelText("Categoria"));
    await userEvent.clear(screen.getByLabelText("Dongle"));
    await submit();
    expect(screen.getByText("Informe o nome do motorista.")).toBeInTheDocument();
    expect(screen.getByText("Informe a categoria.")).toBeInTheDocument();
    expect(screen.getByText("Informe o número do dongle.")).toBeInTheDocument();
    expect(screen.getByText("Informe o boot.")).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Motorista")).toHaveAttribute("aria-invalid", "true");
  });

  it("interval requires dates and t1 after t0", async () => {
    const f = fetchWith({ status: 202 });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Motorista"), "Ana");
    await userEvent.click(screen.getByLabelText("Intervalo de tempo"));
    await submit();
    expect(screen.getByText("Informe o início.")).toBeInTheDocument();
    expect(screen.getByText("Informe o fim.")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Início"), { target: { value: "2026-10-28T10:00" } });
    fireEvent.change(screen.getByLabelText("Fim"), { target: { value: "2026-10-28T09:00" } });
    await submit();
    expect(screen.getByText("O fim deve ser depois do início.")).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
  });

  it("pure functions", () => {
    expect(toRfc3339("2026-10-28T08:00")).toBe("2026-10-28T08:00:00-03:00");
    expect(toRfc3339("2026-10-28T08:00:30")).toBe("2026-10-28T08:00:30-03:00");
    const base = {
      driver: " Ana ",
      category: " Feira ",
      fuel: "diesel",
      dongle: "1",
      mode: "boot" as const,
      bootId: "8",
      bootGeneration: "2",
      t0: "",
      t1: "",
    };
    expect(buildBody(base)).toEqual({
      motorista: { nome: "Ana" },
      categoria_viagem: "Feira",
      combustivel: "diesel",
      entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 2 },
    });
    expect(validateFields({ ...base, bootGeneration: "x" }).bootGeneration).toBeTruthy();
    expect(buildBody({ ...base, bootGeneration: "" }).entrada).not.toHaveProperty(
      "boot_generation",
    );
    expect(
      buildBody({ ...base, mode: "interval", t0: "2026-10-28T08:00", t1: "2026-10-28T18:00" })
        .entrada,
    ).toEqual({
      tipo: "intervalo",
      dongle_id: 1,
      t0: "2026-10-28T08:00:00-03:00",
      t1: "2026-10-28T18:00:00-03:00",
    });
  });
});

describe("submit", () => {
  it("202 goes to /viagem/<id> and sends the expected body", async () => {
    const f = fetchWith({ status: 202, body: { viagem_id: "abc", status: "pendente" } });
    const goTo = vi.fn();
    renderWithTheme(<ProcessForm goTo={goTo} />);
    await fillBoot("  João ", "8");
    await submit();
    await waitFor(() => expect(goTo).toHaveBeenCalledWith("/viagem/abc"));
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe("/api/trips");
    expect(JSON.parse(init.body as string)).toEqual({
      motorista: { nome: "João" },
      categoria_viagem: "feira",
      combustivel: "gasolina_e30",
      entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
    });
    // Stays disabled during navigation.
    expect(screen.getByRole("button", { name: "Enviando…" })).toBeDisabled();
  });

  it("sends interval with -03:00 timezone", async () => {
    const f = fetchWith({ status: 202, body: { viagem_id: "x" } });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Motorista"), "Ana");
    await userEvent.click(screen.getByLabelText("Intervalo de tempo"));
    fireEvent.change(screen.getByLabelText("Início"), { target: { value: "2026-10-28T08:00" } });
    fireEvent.change(screen.getByLabelText("Fim"), { target: { value: "2026-10-28T18:00" } });
    await submit();
    await waitFor(() => expect(f).toHaveBeenCalled());
    const init = (f.mock.calls as unknown as [string, RequestInit][])[0][1];
    expect(JSON.parse(init.body as string).entrada).toMatchObject({
      tipo: "intervalo",
      t0: "2026-10-28T08:00:00-03:00",
    });
  });

  it("409 shows on the driver field and keeps the rest", async () => {
    fetchWith({ status: 409, body: { codigo: "motorista_repetido" } });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot("João", "8");
    await userEvent.selectOptions(screen.getByLabelText("Combustível"), "diesel");
    await submit();
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

  it("400 shows the detail above the button", async () => {
    fetchWith({
      status: 400,
      body: { codigo: "entrada_invalida", detail: "entrada.boot_id: de 1 a 65535" },
    });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot("João", "70000");
    await submit();
    const notice = await screen.findByText("entrada.boot_id: de 1 a 65535");
    const button = screen.getByRole("button", { name: "Processar viagem" });
    expect(notice.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("404 dongle_desconhecido shows on the dongle field", async () => {
    fetchWith({ status: 404, body: { codigo: "dongle_desconhecido" } });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot();
    await userEvent.clear(screen.getByLabelText("Dongle"));
    await userEvent.type(screen.getByLabelText("Dongle"), "2");
    await submit();
    expect(await screen.findByText("Dongle não cadastrado.")).toBeInTheDocument();
  });

  it("404 boot_desconhecido shows on the boot field", async () => {
    fetchWith({ status: 404, body: { codigo: "boot_desconhecido" } });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot();
    await submit();
    expect(
      await screen.findByText("Este boot não existe para o dongle informado. Confira o número."),
    ).toBeInTheDocument();
  });

  it("starts with the dongle configured on the server and sends that value", async () => {
    const f = fetchWith({ status: 202, body: { viagem_id: "x" } });
    renderWithTheme(<ProcessForm initialDongle={7} goTo={vi.fn()} />);
    expect(screen.getByLabelText("Dongle")).toHaveValue(7);
    await fillBoot();
    await submit();
    await waitFor(() => expect(f).toHaveBeenCalled());
    const init = (f.mock.calls as unknown as [string, RequestInit][])[0][1];
    expect(JSON.parse(init.body as string).entrada.dongle_id).toBe(7);
  });

  it("409 moves focus to the driver field", async () => {
    fetchWith({ status: 409, body: { codigo: "motorista_repetido" } });
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot();
    await userEvent.click(screen.getByLabelText("Categoria"));
    await submit();
    await screen.findByText(/Já existe uma viagem/);
    expect(screen.getByLabelText("Motorista")).toHaveFocus();
  });

  it('401 shows "Sessão expirada", offers login in another tab and preserves the form', async () => {
    fetchWith({ status: 401 });
    const goTo = vi.fn();
    renderWithTheme(<ProcessForm goTo={goTo} />);
    await fillBoot("João", "8");
    await submit();
    expect(await screen.findByText(/Sessão expirada./)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Entrar de novo" });
    expect(link).toHaveAttribute("href", "/entrar?retorno=%2Fprocessar");
    expect(link).toHaveAttribute("target", "_blank");
    expect(goTo).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Motorista")).toHaveValue("João");
    expect(screen.getByRole("spinbutton", { name: "Boot" })).toHaveValue(8);
  });

  it.each([
    [403, undefined, undefined, "Esta conta não tem permissão de operador.", "Processar viagem"],
    [429, "limite_taxa", "4", "Servidor ocupado. Tente de novo em 4 s.", "Tentar de novo"],
    [
      429,
      "limite_taxa",
      undefined,
      "Servidor ocupado. Tente de novo em instantes.",
      "Tentar de novo",
    ],
    [503, "banco_ocupado", "9", "Servidor sobrecarregado.", "Tentar de novo"],
    [
      503,
      "api_indisponivel",
      undefined,
      "Não foi possível falar com o servidor.",
      "Tentar de novo",
    ],
    [502, "resposta_invalida", undefined, "Erro inesperado no servidor.", "Tentar de novo"],
    [500, undefined, undefined, "Erro inesperado no servidor.", "Tentar de novo"],
    [
      502,
      "credencial_invalida",
      undefined,
      "O servidor não aceitou a credencial do sistema. Chame a organização.",
      "Processar viagem",
    ],
    [
      429,
      "bloqueado",
      "30",
      "Acesso temporariamente bloqueado. Tente de novo em 30 s.",
      "Tentar de novo",
    ],
    [
      429,
      "bloqueado",
      undefined,
      "Acesso temporariamente bloqueado. Tente de novo em instantes.",
      "Tentar de novo",
    ],
  ])(
    "status %i %s shows the right message and button",
    async (status, codigo, ra, text, buttonName) => {
      fetchWith({ status, body: codigo ? { codigo } : undefined, retryAfter: ra });
      renderWithTheme(<ProcessForm goTo={vi.fn()} />);
      await fillBoot();
      await submit();
      expect(await screen.findByText(text)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: buttonName })).toBeEnabled();
    },
  );

  it('network failure shows "Não foi possível falar com o servidor." and the POST does not repeat on its own', async () => {
    const f = vi.fn(async () => Promise.reject(new Error("rede")));
    vi.stubGlobal("fetch", f);
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot();
    await submit();
    expect(await screen.findByText("Não foi possível falar com o servidor.")).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    expect(f).toHaveBeenCalledTimes(2);
  });

  it("creationError does not use title or detail to decide", () => {
    const p = { type: "t", title: "motorista_repetido", status: 409, codigo: "outro", detail: "x" };
    expect(creationError(409, p, null, "a", "b")).toEqual({
      general: "Erro inesperado no servidor.",
      retry: true,
    });
    expect(
      creationError(
        400,
        { ...p, status: 400, codigo: "entrada_invalida", detail: undefined },
        null,
        "a",
        "b",
      ),
    ).toEqual({
      general: "Entrada inválida.",
    });
  });

  it("prevents double submit while waiting", async () => {
    let release: (r: Resp) => void = () => undefined;
    const f = fetchWith(() => new Promise<Resp>((res) => (release = res)));
    renderWithTheme(<ProcessForm goTo={vi.fn()} />);
    await fillBoot();
    const button = screen.getByRole("button", { name: "Processar viagem" });
    await userEvent.click(button);
    expect(button).toBeDisabled();
    fireEvent.submit(button.closest("form")!);
    fireEvent.submit(button.closest("form")!);
    expect(f).toHaveBeenCalledTimes(1);
    release({ status: 500 });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeEnabled(),
    );
  });
});
