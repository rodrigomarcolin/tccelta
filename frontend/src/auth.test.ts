import { afterEach, describe, expect, it, vi } from "vitest";
import { BASE_PATH } from "@/config";

type Config = {
  providers: unknown[];
  session: { strategy: string };
  pages: { signIn: string; error: string };
  callbacks: {
    signIn: (a: { profile?: { email?: string; email_verified?: boolean } }) => boolean | string;
  };
};

const captured = vi.hoisted(() => ({ factory: null as null | (() => Config) }));

vi.mock("next-auth", () => ({
  default: (factory: () => Config) => {
    captured.factory = factory;
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
  it("uses a JWT session, Google and the /entrar screens", () => {
    const c = captured.factory!();
    expect(c.session.strategy).toBe("jwt");
    expect(c.pages).toEqual({ signIn: `${BASE_PATH}/entrar`, error: `${BASE_PATH}/entrar` });
    expect(c.providers).toHaveLength(1);
  });

  it("changing OPERATOR_EMAILS without rebuilding changes who gets in", () => {
    const profile = { email: "ana@usp.br", email_verified: true };
    vi.stubEnv("OPERATOR_EMAILS", "");
    expect(captured.factory!().callbacks.signIn({ profile })).toBe(
      `${BASE_PATH}/entrar?erro=conta_nao_liberada`,
    );
    vi.stubEnv("OPERATOR_EMAILS", "Ana@usp.br");
    expect(captured.factory!().callbacks.signIn({ profile })).toBe(true);
    expect(
      captured.factory!().callbacks.signIn({
        profile: { email: "ana@gmail.com", email_verified: true },
      }),
    ).toBe(`${BASE_PATH}/entrar?erro=conta_nao_liberada`);
  });
});

describe("instrumentation", () => {
  it("in production without ANALISE_FONTE_DADOS the server does not start", async () => {
    vi.stubEnv("NEXT_RUNTIME", "nodejs");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "");
    await expect(register()).rejects.toThrow(/ANALISE_FONTE_DADOS/);
  });

  it("with explicit mock it starts", async () => {
    vi.stubEnv("NEXT_RUNTIME", "nodejs");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "mock");
    await expect(register()).resolves.toBeUndefined();
  });

  it("outside the Node runtime it does not check", async () => {
    vi.stubEnv("NEXT_RUNTIME", "edge");
    vi.stubEnv("NODE_ENV", "production");
    vi.stubEnv("ANALISE_FONTE_DADOS", "");
    await expect(register()).resolves.toBeUndefined();
  });
});
