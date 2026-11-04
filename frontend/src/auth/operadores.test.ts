import { describe, expect, it } from "vitest";
import { contaLiberada, listaOperadores, retornoSeguro } from "./operadores";

const lista = "ana@usp.br, Beto@USP.br ,";

describe("contaLiberada", () => {
  it("libera e-mail USP verificado que está na lista", () => {
    expect(contaLiberada({ email: "ana@usp.br", email_verified: true }, lista)).toBe(true);
  });
  it("ignora maiúsculas e espaços na lista e no e-mail", () => {
    expect(contaLiberada({ email: "BETO@usp.br", email_verified: true }, lista)).toBe(true);
    expect(contaLiberada({ email: " ana@usp.br ", email_verified: true }, lista)).toBe(true);
  });
  it("recusa domínio diferente", () => {
    expect(contaLiberada({ email: "ana@gmail.com", email_verified: true }, "ana@gmail.com")).toBe(
      false,
    );
    expect(
      contaLiberada({ email: "ana@usp.br.evil.com", email_verified: true }, "ana@usp.br.evil.com"),
    ).toBe(false);
  });
  it("recusa e-mail USP fora da lista", () => {
    expect(contaLiberada({ email: "carla@usp.br", email_verified: true }, lista)).toBe(false);
  });
  it("recusa e-mail não verificado", () => {
    expect(contaLiberada({ email: "ana@usp.br", email_verified: false }, lista)).toBe(false);
    expect(contaLiberada({ email: "ana@usp.br" }, lista)).toBe(false);
  });
  it("lista vazia ou ausente não libera ninguém", () => {
    expect(contaLiberada({ email: "ana@usp.br", email_verified: true }, "")).toBe(false);
    expect(contaLiberada({ email: "ana@usp.br", email_verified: true }, undefined)).toBe(false);
    expect(contaLiberada({ email: "ana@usp.br", email_verified: true }, " , ,")).toBe(false);
  });
  it("recusa perfil ausente ou sem e-mail", () => {
    expect(contaLiberada(null, lista)).toBe(false);
    expect(contaLiberada({ email_verified: true }, lista)).toBe(false);
    expect(contaLiberada({ email: "a@b@usp.br", email_verified: true }, "a@b@usp.br")).toBe(false);
  });
});

describe("listaOperadores", () => {
  it("normaliza", () => {
    expect(listaOperadores(" A@x , b@y ,,")).toEqual(["a@x", "b@y"]);
    expect(listaOperadores(null)).toEqual([]);
  });
});

describe("retornoSeguro", () => {
  it("aceita só caminhos internos", () => {
    expect(retornoSeguro("/viagem/abc")).toBe("/viagem/abc");
    expect(retornoSeguro("//evil.com")).toBe("/processar");
    expect(retornoSeguro("https://evil.com")).toBe("/processar");
    expect(retornoSeguro("/\\evil")).toBe("/processar");
    expect(retornoSeguro(undefined)).toBe("/processar");
  });
});
