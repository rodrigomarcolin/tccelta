import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BASE_PATH } from "@/config";

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
  cookies: async () => ({ get: (n: string) => (n === "theme" ? { value: "light" } : undefined) }),
}));
vi.mock("next/font/google", () => ({
  Space_Grotesk: () => ({ variable: "typeface-ui" }),
  JetBrains_Mono: () => ({ variable: "typeface-mono" }),
}));
vi.mock("@/components/analysis/TripPage", () => ({
  TripPage: ({ id }: { id: string }) => <div data-testid="trip">{id}</div>,
}));
vi.mock("@/components/process/ProcessForm", () => ({
  ProcessForm: () => <form data-testid="form" />,
}));

import RootPage from "./page";
import SignInPage from "./entrar/page";
import ProcessPage from "./processar/page";
import TripRoute from "./viagem/[id]/page";
import RootLayout, { metadata } from "./layout";
import { requireSession } from "@/server/session";

async function expectRedirect(p: Promise<unknown>): Promise<string> {
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

describe("page protection", () => {
  it("without a session, / /processar and /viagem/[id] redirect to /entrar and keep the return path", async () => {
    auth.mockResolvedValue(null);
    expect(await expectRedirect(RootPage())).toBe("/entrar?retorno=%2F");
    expect(await expectRedirect(ProcessPage())).toBe("/entrar?retorno=%2Fprocessar");
    expect(await expectRedirect(TripRoute({ params: Promise.resolve({ id: "abc" }) }))).toBe(
      "/entrar?retorno=%2Fviagem%2Fabc",
    );
    expect(await expectRedirect(requireSession("/x"))).toBe("/entrar?retorno=%2Fx");
  });

  it("with a session, / goes to /processar", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    expect(await expectRedirect(RootPage())).toBe("/processar");
  });

  it("with a session, processar and viagem render", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    render(await ProcessPage());
    expect(screen.getByRole("heading", { name: "Processar viagem" })).toBeInTheDocument();
    expect(screen.getByTestId("form")).toBeInTheDocument();
  }, 15000);

  it("viagem renders the component with the id", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    render(await TripRoute({ params: Promise.resolve({ id: "abc" }) }));
    expect(screen.getByTestId("trip")).toHaveTextContent("abc");
  });
});

describe("/entrar", () => {
  const page = (sp: Record<string, string> = {}) =>
    SignInPage({ searchParams: Promise.resolve(sp) });

  it("shows the button and no error by default", async () => {
    auth.mockResolvedValue(null);
    render(await page());
    expect(screen.getByRole("button", { name: "Entrar com Google" })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("a rejected account shows the reason", async () => {
    auth.mockResolvedValue(null);
    render(await page({ erro: "conta_nao_liberada" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Esta conta não está liberada para processar viagens. Use um e-mail USP autorizado.",
    );
  });

  it("generic error", async () => {
    auth.mockResolvedValue(null);
    render(await page({ erro: "outro" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Não foi possível entrar. Tente de novo.");
    render(await page({ error: "Configuration" }));
    expect(screen.getAllByRole("alert").length).toBeGreaterThan(1);
  });

  it("already signed in goes back to the safe return path", async () => {
    auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
    expect(await expectRedirect(page({ retorno: "/viagem/x" }))).toBe("/viagem/x");
    expect(await expectRedirect(page({ retorno: "//evil.com" }))).toBe("/processar");
  });

  it("the form signs in with Google and returns to the destination", async () => {
    auth.mockResolvedValue(null);
    const jsx = await page({ retorno: "/viagem/x" });
    const { container } = render(jsx);
    const form = container.querySelector("form") as HTMLFormElement & { action: unknown };
    // A server action is a function in React; we call what the <form> receives.
    const action = (jsx as { props: { children: { props: { children: unknown[] } } } }).props
      ? findAction(jsx)
      : null;
    expect(form).not.toBeNull();
    expect(action).toBeTypeOf("function");
    await (action as () => Promise<void>)();
    expect(signIn).toHaveBeenCalledWith("google", { redirectTo: `${BASE_PATH}/viagem/x` });
  });
});

function findAction(node: unknown): unknown {
  if (!node || typeof node !== "object") return null;
  const n = node as { type?: unknown; props?: { action?: unknown; children?: unknown } };
  if (n.type === "form" && typeof n.props?.action === "function") return n.props.action;
  const kids = n.props?.children;
  for (const k of Array.isArray(kids) ? kids : [kids]) {
    const r = findAction(k);
    if (r) return r;
  }
  return null;
}

describe("root layout", () => {
  it("reads the theme from the cookie on the server", async () => {
    const jsx = (await RootLayout({ children: <p>hi</p> })) as { props: Record<string, unknown> };
    expect(jsx.props["data-theme"]).toBe("light");
    expect(jsx.props.lang).toBe("pt-BR");
    expect(String(jsx.props.className)).toContain("typeface-ui");
    expect(metadata.robots).toMatchObject({ index: false });
  });
});
