import { afterEach, describe, expect, it } from "vitest";
import { escolherFonte, obterCliente } from "./cliente";

afterEach(() => {
  delete (globalThis as Record<string, unknown>).__tccelta_cliente__;
});

describe("escolherFonte", () => {
  it("usa mock em desenvolvimento quando a variável falta", () => {
    expect(escolherFonte({ NODE_ENV: "development" })).toBe("mock");
    expect(escolherFonte({ NODE_ENV: "test", ANALISE_FONTE_DADOS: "  " })).toBe("mock");
  });
  it("em produção a variável é obrigatória", () => {
    expect(() => escolherFonte({ NODE_ENV: "production" })).toThrow(/ANALISE_FONTE_DADOS/);
    expect(() => escolherFonte({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "" })).toThrow();
  });
  it("aceita mock explícito em produção", () => {
    expect(escolherFonte({ NODE_ENV: "production", ANALISE_FONTE_DADOS: "mock" })).toBe("mock");
  });
  it("api ainda não existe", () => {
    expect(() => escolherFonte({ ANALISE_FONTE_DADOS: "api" })).toThrow(
      "Cliente da API ainda não implementado (INT1)",
    );
  });
  it("recusa valor desconhecido", () => {
    expect(() => escolherFonte({ ANALISE_FONTE_DADOS: "banco" })).toThrow(/desconhecida/);
  });
});

describe("obterCliente", () => {
  it("devolve sempre a mesma instância", () => {
    const a = obterCliente({ NODE_ENV: "test" });
    expect(obterCliente({ NODE_ENV: "test" })).toBe(a);
  });
  it("não sobe em produção sem a variável", () => {
    expect(() => obterCliente({ NODE_ENV: "production" })).toThrow();
  });
});
