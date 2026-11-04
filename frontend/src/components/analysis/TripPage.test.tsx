import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TripStatusResponse, TripStatus } from "@/domain/trip/types";
import { analysisFor, renderWithTheme } from "@/tests/helpers";
import { TripPage } from "./TripPage";

vi.mock("@/components/analysis/Chart", async () => ({
  Chart: (await import("@/tests/doubles")).ChartDouble,
}));
vi.mock("@/components/analysis/RouteMap", async () => ({
  RouteMap: (await import("@/tests/doubles")).MapDouble,
}));
vi.mock("next/link", () => ({
  default: ({ href, children, ...r }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...r}>
      {children}
    </a>
  ),
}));

type Resp = { status: number; body?: unknown; retryAfter?: string };
type Outcome = Resp | Error | Promise<Resp | Error>;
type Script = {
  status: () => Outcome;
  analysis: () => Outcome;
};

const ID = "7d1f3a10-0001-4c5e-9b1a-000000000008";
const calls: string[] = [];
const signals: AbortSignal[] = [];

function install(script: Script) {
  calls.length = 0;
  signals.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push(url);
      if (init?.signal) signals.push(init.signal);
      const r = await (url.endsWith("/analysis") ? script.analysis() : script.status());
      if (r instanceof Error) throw r;
      return {
        status: r.status,
        ok: r.status >= 200 && r.status < 300,
        json: async () => r.body,
        headers: new Headers(r.retryAfter ? { "Retry-After": r.retryAfter } : {}),
      };
    }),
  );
}

const status = (
  s: TripStatus,
  provisional = false,
  calculated: string | null = null,
): TripStatusResponse => ({
  viagem_id: ID,
  status: s,
  provisoria: provisional,
  motorista: { nome: "Maria" },
  categoria_viagem: "feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 0 },
  criada_em: "2026-10-28T17:00:00.000Z",
  calculada_em: calculated,
});

const advance = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
const count = (suffix: "status" | "analysis") =>
  calls.filter((u) => (suffix === "analysis" ? u.endsWith("/analysis") : !u.endsWith("/analysis")))
    .length;

async function mount(goTo = vi.fn()) {
  renderWithTheme(<TripPage id={ID} goTo={goTo} />);
  await advance(0);
  return goTo;
}

let hidden = false;
const setHidden = (v: boolean) => {
  hidden = v;
  document.dispatchEvent(new Event("visibilitychange"));
};

beforeEach(() => {
  vi.useFakeTimers();
  hidden = false;
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  // React Query reads `visibilityState`, not `hidden`.
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const NETWORK_NOTICE = "Sem conexão com o servidor. Tentando novamente…";

describe("waiting and 1 s polling", () => {
  it("shows the wait with the name and opens the analysis without reloading when ready", async () => {
    let current: TripStatus = "pendente";
    install({
      status: () => ({ status: 200, body: status(current) }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    expect(document.body.querySelector(".dot-pulse")).not.toBeNull();
    expect(screen.queryByTestId("map")).toBeNull();
    expect(count("analysis")).toBe(0);

    current = "aguardando_dados";
    await advance(1000);
    expect(count("status")).toBe(2);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await advance(1000);
    expect(count("status")).toBe(3);

    current = "pronta";
    await advance(1000);
    expect(screen.getByText("Dongle 1 · boot 8")).toBeInTheDocument();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(count("analysis")).toBe(1);

    // Ready and complete: polling stops.
    await advance(10_000);
    expect(count("status")).toBe(4);
  });

  it("keeps waiting while the analysis still answers 409", async () => {
    let analysisOk = false;
    install({
      status: () => ({ status: 200, body: status("pronta") }),
      analysis: () =>
        analysisOk
          ? { status: 200, body: analysisFor("normal") }
          : {
              status: 409,
              body: { codigo: "analise_indisponivel", viagem_status: "aguardando_dados" },
            },
    });
    await mount();
    expect(screen.queryByTestId("map")).toBeNull();
    analysisOk = true;
    await advance(1000);
    expect(screen.getByTestId("map")).toBeInTheDocument();
  });

  it("does not overlap cycles: waits for the slow response before scheduling the next", async () => {
    install({
      status: () =>
        new Promise((ok) => setTimeout(() => ok({ status: 200, body: status("pendente") }), 3500)),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    await advance(3000);
    expect(count("status")).toBe(1);
    await advance(500);
    await advance(999);
    expect(count("status")).toBe(1);
    await advance(1);
    expect(count("status")).toBe(2);
  });
});

describe("other states", () => {
  it("failed: message and released name", async () => {
    install({
      status: () => ({ status: 200, body: status("falhou") }),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(
      screen.getByText("Não foi possível calcular esta viagem. O nome do motorista foi liberado."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Processar outra viagem/ })).toHaveAttribute(
      "href",
      "/processar",
    );
    await advance(5000);
    expect(count("status")).toBe(1);
  });

  it("404: Viagem não encontrada, with a link, and polling stops", async () => {
    install({
      status: () => ({ status: 404, body: { codigo: "viagem_desconhecida" } }),
      analysis: () => ({ status: 404 }),
    });
    await mount();
    expect(screen.getByText("Viagem não encontrada")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Voltar para processar/ })).toHaveAttribute(
      "href",
      "/processar",
    );
    await advance(5000);
    expect(count("status")).toBe(1);
  });

  it("401 on status goes to /entrar with the return path", async () => {
    install({ status: () => ({ status: 401 }), analysis: () => ({ status: 401 }) });
    const goToMock = await mount();
    expect(goToMock).toHaveBeenCalledWith(`/entrar?retorno=${encodeURIComponent(`/viagem/${ID}`)}`);
    await advance(5000);
    expect(count("status")).toBe(1);
  });

  it("401 on the analysis goes to /entrar", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta") }),
      analysis: () => ({ status: 401 }),
    });
    const goToMock = await mount();
    expect(goToMock).toHaveBeenCalledTimes(1);
  });

  it("unexpected response (400 off-contract): message, and 'Tentar de novo' reloads", async () => {
    let fail = true;
    install({
      status: () => (fail ? { status: 400 } : { status: 200, body: status("pronta") }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    expect(screen.getByText("Não foi possível carregar a viagem.")).toBeInTheDocument();
    await advance(5000);
    expect(count("status")).toBe(1);
    fail = false;
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    });
    await advance(0);
    expect(screen.getByTestId("map")).toBeInTheDocument();
  });

  it("an unexpected analysis response shows the message", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta") }),
      analysis: () => ({ status: 400 }),
    });
    await mount();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });

  it("status 200 with an invalid body shows the message", async () => {
    install({ status: () => ({ status: 200, body: null }), analysis: () => ({ status: 409 }) });
    await mount();
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
  });

  it("shows the loading skeleton before the first response", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise(() => undefined)),
    );
    renderWithTheme(<TripPage id={ID} goTo={vi.fn()} />);
    expect(screen.getByLabelText("Carregando a análise")).toBeInTheDocument();
  });
});

describe("network and backoff", () => {
  it("consecutive failures back off 1, 2, 4 and 5 s, warn on the third, and success returns to 1 s", async () => {
    let network = true;
    install({
      status: () => (network ? new Error("network") : { status: 200, body: status("pendente") }),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(count("status")).toBe(1);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    await advance(1000);
    expect(count("status")).toBe(2);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    await advance(1999);
    expect(count("status")).toBe(2);
    await advance(1);
    expect(count("status")).toBe(3);
    expect(screen.getByText(NETWORK_NOTICE)).toBeInTheDocument();
    // Next at 4 s, then at 5 s (cap).
    await advance(3999);
    expect(count("status")).toBe(3);
    await advance(1);
    expect(count("status")).toBe(4);
    await advance(4999);
    expect(count("status")).toBe(4);
    await advance(1);
    expect(count("status")).toBe(5);

    network = false;
    await advance(5000);
    expect(count("status")).toBe(6);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await advance(1000);
    expect(count("status")).toBe(7);
  });

  it("5xx counts as a failure and the notice does not clear the analysis already shown", async () => {
    let outage = false;
    install({
      status: () =>
        outage ? { status: 503 } : { status: 200, body: status("pronta", true, "c1") },
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    outage = true;
    await advance(5000);
    await advance(1000);
    await advance(2000);
    expect(screen.getByText(NETWORK_NOTICE)).toBeInTheDocument();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    outage = false;
    await advance(4000);
    expect(screen.queryByText(/Sem conexão/)).toBeNull();
    expect(screen.getByTestId("map")).toBeInTheDocument();
  });

  it("a 5xx on the analysis retries, and an unexpected response with the analysis on screen does not erase it", async () => {
    let analysisOk = false;
    install({
      status: () => ({ status: 200, body: status("pronta", true, analysisOk ? "c2" : "c1") }),
      analysis: () => (analysisOk ? { status: 400 } : { status: 200, body: analysisFor("normal") }),
    });
    await mount();
    analysisOk = true;
    await advance(5000);
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tentar de novo" })).toBeNull();
  });

  it("a 404 on the analysis of a new version keeps the analysis on screen and retries", async () => {
    let newVersion = false;
    install({
      status: () => ({ status: 200, body: status("pronta", true, newVersion ? "c2" : "c1") }),
      analysis: () => (newVersion ? { status: 404 } : { status: 200, body: analysisFor("normal") }),
    });
    await mount();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    newVersion = true;
    await advance(5000);
    await advance(5000);
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tentar de novo" })).toBeNull();
    expect(count("analysis")).toBeGreaterThan(2);
  });

  it("the online event forces an immediate attempt", async () => {
    install({ status: () => new Error("network"), analysis: () => ({ status: 409 }) });
    await mount();
    await advance(1000);
    await advance(2000);
    await advance(3000);
    const before = count("status");
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(count("status")).toBe(before + 1);
  });

  it("429 honours Retry-After while polling and warns", async () => {
    let busy = true;
    install({
      status: () =>
        busy
          ? { status: 429, body: { codigo: "limite_taxa" }, retryAfter: "5" }
          : { status: 200, body: status("pendente") },
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(screen.getByText("Servidor ocupado. Tentando de novo em 5 s.")).toBeInTheDocument();
    await advance(4999);
    expect(count("status")).toBe(1);
    busy = false;
    await advance(1);
    expect(count("status")).toBe(2);
    expect(screen.queryByText(/Servidor busy/)).toBeNull();
  });

  it("credencial_invalida pauses polling for the deadline and warns", async () => {
    let refused = true;
    install({
      status: () =>
        refused
          ? {
              status: 502,
              body: { codigo: "credencial_invalida", title: "c", status: 502 },
              retryAfter: "60",
            }
          : { status: 200, body: status("pendente") },
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(
      screen.getByText("O servidor não aceitou a credencial do sistema. Chame a organização."),
    ).toBeInTheDocument();
    await advance(59_000);
    expect(count("status")).toBe(1);
    refused = false;
    await advance(1000);
    expect(count("status")).toBe(2);
    expect(screen.queryByText(/credencial/)).toBeNull();
  });

  it("bloqueado shows the remaining time and pauses polling", async () => {
    install({
      status: () => ({
        status: 429,
        body: { codigo: "bloqueado", title: "b", status: 429 },
        retryAfter: "20",
      }),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(
      screen.getByText("Acesso temporariamente bloqueado. Tente de novo em 20 s."),
    ).toBeInTheDocument();
    await advance(19_000);
    expect(count("status")).toBe(1);
    await advance(1000);
    expect(count("status")).toBe(2);
  });

  it("429 on the analysis also defers", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta") }),
      analysis: () => ({ status: 429, retryAfter: "2" }),
    });
    await mount();
    expect(screen.getByText("Servidor ocupado. Tentando de novo em 2 s.")).toBeInTheDocument();
    await advance(2000);
    // Only the analysis is requested again after the 429.
    expect(count("analysis")).toBe(2);
  });
});

describe("hidden tab", () => {
  it("pauses when hidden and polls right away on return", async () => {
    install({
      status: () => ({ status: 200, body: status("pendente") }),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    expect(count("status")).toBe(1);
    act(() => setHidden(true));
    await advance(10_000);
    expect(count("status")).toBe(1);
    await act(async () => {
      setHidden(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(count("status")).toBe(2);
    await advance(1000);
    expect(count("status")).toBe(3);
  });

  it("a cycle that ends with the tab hidden stays deferred until it returns", async () => {
    install({
      status: () => ({ status: 200, body: status("pendente") }),
      analysis: () => ({ status: 409 }),
    });
    hidden = true;
    await mount();
    await advance(5000);
    expect(count("status")).toBe(1);
    await act(async () => {
      setHidden(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(count("status")).toBe(2);
  });

  it("becoming visible again without a deferred cycle does not poll", async () => {
    install({
      status: () => ({ status: 200, body: status("pendente") }),
      analysis: () => ({ status: 409 }),
    });
    await mount();
    act(() => setHidden(false));
    await advance(0);
    expect(count("status")).toBe(1);
  });

  it("after stopping, hiding, returning and going online it does not poll", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta") }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    act(() => setHidden(true));
    await act(async () => {
      setHidden(false);
      window.dispatchEvent(new Event("online"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(count("status")).toBe(1);
  });
});

describe("60 s wait limit", () => {
  it("stops, explains, and 'Continuar esperando' resumes for another 60 s", async () => {
    let current: TripStatus = "pendente";
    install({
      status: () => ({ status: 200, body: status(current) }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    await advance(59_000);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await advance(1000);
    expect(
      screen.getByText(
        "A viagem ainda não foi processada. O processamento no servidor pode estar parado.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Voltar/ })).toHaveAttribute("href", "/processar");
    const stopped = count("status");
    await advance(10_000);
    expect(count("status")).toBe(stopped);

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Continuar esperando" }));
    });
    await advance(0);
    expect(count("status")).toBe(stopped + 1);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await advance(30_000);
    current = "aguardando_dados";
    await advance(29_000);
    expect(screen.getByText("Processando a viagem de «Maria»")).toBeInTheDocument();
    await advance(1000);
    expect(screen.getByText(/ainda não foi processada/)).toBeInTheDocument();

    // The worker comes back: keep waiting completes.
    current = "pronta";
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Continuar esperando" }));
    });
    await advance(0);
    expect(screen.getByTestId("map")).toBeInTheDocument();
  });
});

describe("provisional trip", () => {
  it("polls every 5 s and swaps 'Aguardando dados' for the analysis by itself", async () => {
    let complete = false;
    install({
      status: () => ({
        status: 200,
        body: status(
          "pronta",
          !complete,
          complete ? "2026-10-28T17:05:00.000Z" : "2026-10-28T17:00:20.000Z",
        ),
      }),
      analysis: () => ({
        status: 200,
        body: analysisFor("provisoria_aguardando", complete),
      }),
    });
    await mount();
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
    expect(count("status")).toBe(1);

    await advance(4000);
    expect(count("status")).toBe(1);
    await advance(1000);
    expect(count("status")).toBe(2);
    expect(screen.getByText("Aguardando dados")).toBeInTheDocument();

    complete = true;
    await advance(5000);
    expect(screen.queryByText("Aguardando dados")).toBeNull();
    expect(screen.queryByText("Resultado provisório: ainda podem chegar dados")).toBeNull();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(screen.getByText("Resultado atualizado às 14:05")).toBeInTheDocument();
    expect(count("analysis")).toBe(2);
    const before = count("status");
    await advance(20_000);
    expect(count("status")).toBe(before);
  });

  it("a new calculada_em swaps the analysis without a blank screen and shows the time", async () => {
    let version = 1;
    install({
      status: () => ({
        status: 200,
        body: status(
          "pronta",
          true,
          version === 1 ? "2026-10-28T17:00:20.000Z" : "2026-10-28T17:05:00.000Z",
        ),
      }),
      analysis: () => ({ status: 200, body: analysisFor("normal", version === 2) }),
    });
    await mount();
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(screen.queryByText(/Resultado atualizado/)).toBeNull();

    await advance(5000);
    expect(count("analysis")).toBe(1);
    version = 2;
    await advance(5000);
    expect(count("analysis")).toBe(2);
    expect(screen.getByTestId("map")).toBeInTheDocument();
    expect(screen.getByText("Resultado atualizado às 14:05")).toBeInTheDocument();
    expect(screen.getByText("Resultado provisório: ainda podem chegar dados")).toBeInTheDocument();
  });

  it("provisoria false without a calculada_em change only removes the banner", async () => {
    let provisional = true;
    install({
      status: () => ({
        status: 200,
        body: status("pronta", provisional, "2026-10-28T17:00:20.000Z"),
      }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    expect(screen.getByText(/Resultado provisório/)).toBeInTheDocument();
    provisional = false;
    await advance(5000);
    expect(screen.queryByText(/Resultado provisório/)).toBeNull();
    expect(screen.queryByText(/Resultado atualizado/)).toBeNull();
    expect(count("analysis")).toBe(1);
    expect(screen.getByTestId("map")).toBeInTheDocument();
    await advance(20_000);
    expect(count("status")).toBe(2);
  });

  it("the 5 s poll only runs while the tab is visible", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta", true, "c") }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    await mount();
    act(() => setHidden(true));
    await advance(30_000);
    expect(count("status")).toBe(1);
    await act(async () => {
      setHidden(false);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(count("status")).toBe(2);
  });

  it("stops after 5 min and asks for a reload", async () => {
    install({
      status: () => ({ status: 200, body: status("pronta", true, "c") }),
      analysis: () => ({ status: 200, body: analysisFor("dados_parciais", false) }),
    });
    await mount();
    await advance(5 * 60 * 1000 + 6000);
    const total = count("status");
    expect(total).toBeGreaterThan(30);
    expect(total).toBeLessThanOrEqual(62);
    await advance(30_000);
    expect(count("status")).toBe(total);
    expect(screen.getByText("Resultado provisório. Recarregue para verificar")).toBeInTheDocument();
  });
});

describe("cancellation", () => {
  it("unmounting cancels the in-flight request", async () => {
    install({ status: () => new Promise(() => undefined), analysis: () => ({ status: 409 }) });
    const { unmount } = renderWithTheme(<TripPage id={ID} goTo={vi.fn()} />);
    await advance(0);
    expect(signals[0].aborted).toBe(false);
    unmount();
    expect(signals[0].aborted).toBe(true);
  });

  it("a pending timer does not fire after unmounting", async () => {
    install({
      status: () => ({ status: 200, body: status("pendente") }),
      analysis: () => ({ status: 409 }),
    });
    const { unmount } = renderWithTheme(<TripPage id={ID} goTo={vi.fn()} />);
    await advance(0);
    unmount();
    await advance(10_000);
    expect(count("status")).toBe(1);
  });

  it("a failure that arrives after unmounting schedules nothing", async () => {
    let fail: (e: Error) => void = () => undefined;
    install({
      status: () => new Promise((_ok, no) => (fail = no)),
      analysis: () => ({ status: 409 }),
    });
    const { unmount } = renderWithTheme(<TripPage id={ID} goTo={vi.fn()} />);
    await advance(0);
    unmount();
    fail(new Error("aborted"));
    await advance(10_000);
    expect(count("status")).toBe(1);
  });
});

describe("navigation and restarts", () => {
  it("a navigation between trips does not show the previous trip's data", async () => {
    const OTHER = "7d1f3a10-0001-4c5e-9b1a-000000000009";
    install({
      status: () => ({ status: 200, body: status("pronta", true, "c1") }),
      analysis: () => ({ status: 200, body: analysisFor("normal") }),
    });
    const { rerender } = renderWithTheme(<TripPage id={ID} goTo={vi.fn()} />);
    await advance(0);
    expect(screen.getByTestId("map")).toBeInTheDocument();

    install({
      status: () => new Promise(() => undefined),
      analysis: () => new Promise(() => undefined),
    });
    rerender(<TripPage id={OTHER} goTo={vi.fn()} />);
    await advance(0);
    expect(screen.queryByTestId("map")).toBeNull();
    expect(screen.getByLabelText("Carregando a análise")).toBeInTheDocument();
    expect(calls).toEqual([`/api/trips/${OTHER}`]);
  });

  it("an online event during the backoff restarts the failure count", async () => {
    install({ status: () => new Error("network"), analysis: () => ({ status: 409 }) });
    await mount();
    await advance(1000);
    await advance(2000);
    expect(screen.getByText(NETWORK_NOTICE)).toBeInTheDocument();
    await act(async () => {
      window.dispatchEvent(new Event("online"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.queryByText(NETWORK_NOTICE)).toBeNull();
    await advance(1000);
    await advance(2000);
    expect(screen.getByText(NETWORK_NOTICE)).toBeInTheDocument();
  });
});
