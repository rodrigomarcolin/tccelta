import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.fn();
const signIn = vi.fn();
const redirect = vi.fn((url: string) => {
  throw new Error(`REDIRECT:${url}`);
});

vi.mock("@/auth", () => ({
  auth: () => auth(),
  signIn: (...a: unknown[]) => signIn(...a),
  handlers: { GET: vi.fn(), POST: vi.fn() },
}));
vi.mock("next/navigation", () => ({ redirect: (u: string) => redirect(u) }));
vi.mock("next/headers", () => ({
  cookies: async () => ({ get: (n: string) => (n === "tema" ? { value: "claro" } : undefined) }),
}));
vi.mock("next/font/google", () => ({
  Space_Grotesk: () => ({ variable: "fonte-ui" }),
  JetBrains_Mono: () => ({ variable: "fonte-mono" }),
}));
vi.mock("@/componentes/analise/ViagemPagina", () => ({
  ViagemPagina: ({ id }: { id: string }) => <div data-testid="viagem">{id}</div>,
}));
vi.mock("@/componentes/processar/FormularioProcessar", () => ({
  FormularioProcessar: () => <form data-testid="form" />,
}));

import Raiz from "./page";
import Entrar from "./entrar/page";
import Processar from "./processar/page";
import Viagem from "./viagem/[id]/page";
import RootLayout, { metadata } from "./layout";
import { exigirSessao } from "@/servidor/sessao";

async function esperaRedirect(p: Promise<unknown>): Promise<string> {
  try {
    await p;
  } catch (e) {
    return String((e as Error).message).replace("REDIRECT:", "");
  }
  return "";
}

beforeEach(() => {
  auth.mockReset();
  signIn.mockReset();
  redirect.mockClear();
});

describe("proteção das páginas", () => {
  it("sem sessão, / /processar e /viagem/[id] redirecionam para /entrar e guardam o retorno", async () => {
    auth.mockResolvedValue(null);
    expect(await esperaRedirect(Raiz())).toBe("/entrar?retorno=%2F");
    expect(await esperaRedirect(Processar())).toBe("/entrar?retorno=%2Fprocessar");
    expect(await esperaRedirect(Viagem({ params: Promise.resolve({ id: "abc" }) }))).toBe(
      "/entrar?retorno=%2Fviagem%2Fabc",
    );
    expect(await esperaRedirect(exigirSessao("/x"))).toBe("/entrar?retorno=%2Fx");
  });

  it("com sessão, / vai para /processar", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    expect(await esperaRedirect(Raiz())).toBe("/processar");
  });

  it("com sessão, processar e viagem renderizam", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    render(await Processar());
    expect(screen.getByRole("heading", { name: "Processar viagem" })).toBeInTheDocument();
    expect(screen.getByTestId("form")).toBeInTheDocument();
  }, 15000);

  it("viagem renderiza o componente com o id", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    render(await Viagem({ params: Promise.resolve({ id: "abc" }) }));
    expect(screen.getByTestId("viagem")).toHaveTextContent("abc");
  });
});

describe("/entrar", () => {
  const pagina = (sp: Record<string, string> = {}) => Entrar({ searchParams: Promise.resolve(sp) });

  it("mostra o botão e não mostra erro por padrão", async () => {
    auth.mockResolvedValue(null);
    render(await pagina());
    expect(screen.getByRole("button", { name: "Entrar com Google" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("conta recusada mostra o motivo", async () => {
    auth.mockResolvedValue(null);
    render(await pagina({ erro: "conta_nao_liberada" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Esta conta não está liberada para processar viagens. Use um e-mail USP autorizado.",
    );
  });

  it("erro genérico", async () => {
    auth.mockResolvedValue(null);
    render(await pagina({ erro: "outro" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Não foi possível entrar. Tente de novo.");
    render(await pagina({ error: "Configuration" }));
    expect(screen.getAllByRole("alert").length).toBeGreaterThan(1);
  });

  it("já autenticado volta ao retorno seguro", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    expect(await esperaRedirect(pagina({ retorno: "/viagem/x" }))).toBe("/viagem/x");
    expect(await esperaRedirect(pagina({ retorno: "//evil.com" }))).toBe("/processar");
  });

  it("o formulário entra com Google e volta ao destino", async () => {
    auth.mockResolvedValue(null);
    const jsx = await pagina({ retorno: "/viagem/x" });
    const { container } = render(jsx);
    const form = container.querySelector("form") as HTMLFormElement & { action: unknown };
    // A server action é uma função no React; chamamos o que o <form> recebe.
    const acao = (jsx as { props: { children: { props: { children: unknown[] } } } }).props
      ? findAction(jsx)
      : null;
    expect(form).not.toBeNull();
    expect(acao).toBeTypeOf("function");
    await (acao as () => Promise<void>)();
    expect(signIn).toHaveBeenCalledWith("google", { redirectTo: "/viagem/x" });
  });
});

function findAction(node: unknown): unknown {
  if (!node || typeof node !== "object") return null;
  const n = node as { type?: unknown; props?: { action?: unknown; children?: unknown } };
  if (n.type === "form" && typeof n.props?.action === "function") return n.props.action;
  const filhos = n.props?.children;
  for (const f of Array.isArray(filhos) ? filhos : [filhos]) {
    const r = findAction(f);
    if (r) return r;
  }
  return null;
}

describe("layout raiz", () => {
  it("lê o tema do cookie no servidor", async () => {
    const jsx = (await RootLayout({ children: <p>oi</p> })) as { props: Record<string, unknown> };
    expect(jsx.props["data-theme"]).toBe("claro");
    expect(jsx.props.lang).toBe("pt-BR");
    expect(String(jsx.props.className)).toContain("fonte-ui");
    expect(metadata.robots).toMatchObject({ index: false });
  });
});
