import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Botao } from "./Botao";
import { Distintivo } from "./Distintivo";
import { Indicador } from "./Indicador";
import { AlternarTema, gravarTema, TemaProvider, useTema } from "./Tema";

describe("Botao", () => {
  it("variantes, tipo padrão button e ícone", () => {
    render(
      <Botao variante="secundario" larguraTotal icone={<i data-testid="i" />}>
        Ok
      </Botao>,
    );
    const b = screen.getByRole("button", { name: "Ok" });
    expect(b).toHaveAttribute("type", "button");
    expect(b).toHaveClass("botao-secundario", "botao-total");
    expect(screen.getByTestId("i")).toBeInTheDocument();
  });
});

describe("Distintivo", () => {
  it("pulsa só quando pedido", () => {
    const { container, rerender } = render(<Distintivo tom="ao-vivo">Ao vivo</Distintivo>);
    expect(container.querySelector(".ponto-pulsar")).toBeNull();
    rerender(
      <Distintivo tom="alerta" pulsar>
        Ao vivo
      </Distintivo>,
    );
    expect(container.querySelector(".ponto-pulsar")).not.toBeNull();
    expect(container.firstChild).toHaveClass("distintivo-alerta");
  });
});

describe("Indicador", () => {
  it("mostra rótulo, valor em mono, unidade e nota", () => {
    render(<Indicador rotulo="Distância" valor="3,5" unidade="km" nota="estimado" />);
    expect(screen.getByText("Distância")).toBeInTheDocument();
    expect(screen.getByText("3,5")).toHaveClass("dado");
    expect(screen.getByText("km")).toHaveClass("unidade");
    expect(screen.getByText("estimado")).toBeInTheDocument();
  });
  it("omite unidade e nota quando ausentes", () => {
    const { container } = render(<Indicador rotulo="X" valor="1" />);
    expect(container.querySelector(".unidade")).toBeNull();
    expect(container.querySelector(".indicador-nota")).toBeNull();
  });
});

describe("tema", () => {
  beforeEach(() => {
    document.cookie = "tema=; max-age=0; path=/";
    document.documentElement.removeAttribute("data-theme");
  });

  it("alternância usa aria-pressed, persiste em cookie e troca o atributo sem recarregar", async () => {
    function Sonda() {
      return <span data-testid="tema">{useTema()}</span>;
    }
    render(
      <TemaProvider inicial="escuro">
        <AlternarTema />
        <Sonda />
      </TemaProvider>,
    );
    const botao = screen.getByRole("button", { name: "Tema claro" });
    expect(botao).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(botao);
    expect(botao).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("tema")).toHaveTextContent("claro");
    expect(document.documentElement.dataset.theme).toBe("claro");
    expect(document.cookie).toContain("tema=claro");
    await userEvent.click(botao);
    expect(document.documentElement.dataset.theme).toBe("escuro");
    expect(document.cookie).toContain("tema=escuro");
  });

  it("gravarTema aceita outro documento", () => {
    const doc = document.implementation.createHTMLDocument("x");
    gravarTema("claro", doc);
    expect(doc.documentElement.dataset.theme).toBe("claro");
  });

  it("sem provedor o tema padrão é escuro", () => {
    function Sonda() {
      return <span data-testid="t">{useTema()}</span>;
    }
    render(<Sonda />);
    expect(screen.getByTestId("t")).toHaveTextContent("escuro");
    fireEvent.click(document.body);
    vi.restoreAllMocks();
  });
});
