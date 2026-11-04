import { afterEach, describe, expect, it, vi } from "vitest";

type Config = {
  providers: unknown[];
  session: { strategy: string };
  pages: { signIn: string; error: string };
  callbacks: {
    signIn: (a: { profile?: { email?: string; email_verified?: boolean } }) => boolean | string;
  };
};

const capturado = vi.hoisted(() => ({ fabrica: null as null | (() => Config) }));

vi.mock("next-auth", () => ({
  default: (fabrica: () => Config) => {
    capturado.fabrica = fabrica;
    return {
      handlers: { GET: vi.fn(), POST: vi.fn() },
      auth: vi.fn(),
      signIn: vi.fn(),
      signOut: vi.fn(),
    };
  },
}));
vi.mock("next-auth/providers/google", () => ({ default: (o: unknown) => ({ id: "google", o }) }));

import "./auth";
import { register } from "./instrumentation";

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("auth.ts", () => {
  it("usa sessão JWT, Google e as telas de /entrar", () => {
    const c = capturado.fabrica!();
    expect(c.session.strategy).toBe("jwt");
    expect(c.pages).toEqual({ signIn: "/entrar", error: "/entrar" });
    expect(c.providers).toHaveLength(1);
  });

  it("trocar OPERATOR_EMAILS sem refazer o build muda quem entra", () => {
    const perfil = { email: "ana@usp.br", email_verified: true };
    vi.stubEnv("OPERATOR_EMAILS", "");
    expect(capturado.fabrica!().callbacks.signIn({ profile: perfil })).toBe(
      "/entrar?erro=conta_nao_liberada",
    );
    vi.stubEnv("OPERATOR_EMAILS", "Ana@usp.br");
    expect(capturado.fabrica!().callbacks.signIn({ profile: perfil })).toBe(true);
    expect(
      capturado.fabrica!().callbacks.signIn({
        profile: { email: "ana@gmail.com", email_verified: true },
      }),
    ).toBe("/entrar?erro=conta_nao_liberada");
  });
});

describe("instrumentation", () => {
  it("em produção sem ANALISE_FONTE_DADOS o servidor não sobe", async () => {
    vi.stubEnv("NEXT_RUNTIME", "nodejs");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "");
    await expect(register()).rejects.toThrow(/ANALISE_FONTE_DADOS/);
  });

  it("com mock explícito sobe", async () => {
    vi.stubEnv("NEXT_RUNTIME", "nodejs");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "mock");
    await expect(register()).resolves.toBeUndefined();
  });

  it("fora do runtime Node não verifica", async () => {
    vi.stubEnv("NEXT_RUNTIME", "edge");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "");
    await expect(register()).resolves.toBeUndefined();
  });
});
