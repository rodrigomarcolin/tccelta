import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { BASE_PATH, POLL_PROVISIONAL_MS, SEARCH_DEBOUNCE_MS } from "@/config";
import type { TripListItem, TripListResponse } from "@/domain/trip/types";
import { renderWithTheme } from "@/tests/helpers";
import { router } from "@/tests/router";
import { FakeObserver, listItem, listPage } from "@/tests/tripList";
import { TripsScreen } from "./TripsScreen";

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    prefetch,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
    prefetch?: boolean;
  }) => (
    <a href={href} data-prefetch={String(prefetch)} {...rest}>
      {children}
    </a>
  ),
}));

type Reply = { status: number; body?: unknown } | Error;
let replies: ((params: URLSearchParams) => Reply | Promise<Reply>)[] = [];
const requests: URLSearchParams[] = [];

/** Answers the next request with the next reply of the script (the last one repeats). */
function script(...fns: typeof replies) {
  replies = fns;
  requests.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const params = new URL(url, "http://x").searchParams;
      requests.push(params);
      const fn = replies[Math.min(requests.length - 1, replies.length - 1)];
      const r = await fn(params);
      if (r instanceof Error) throw r;
      return {
        status: r.status,
        ok: r.status < 300,
        json: async () => r.body,
        headers: new Headers(),
      };
    }),
  );
}
const ok = (body: TripListResponse) => () => ({ status: 200, body });
const problem = (status: number, codigo: string) => () => ({
  status,
  body: { codigo, title: codigo, status, type: "t" },
});

const items = (from: number, n: number, extra: Partial<TripListItem> = {}) =>
  Array.from({ length: n }, (_, i) => listItem(from + i, extra));

const cards = () => screen.getAllByRole("article");

beforeEach(() => {
  FakeObserver.reset();
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  window.scrollTo = vi.fn();
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

/** Fires the observer that is watching the end of the list. */
async function reachEnd() {
  const active = FakeObserver.active();
  expect(active).toHaveLength(1);
  await act(async () => active[0].fire());
}

describe("loading and the first batch", () => {
  it("shows skeletons, then the cards, the count and the attribution", async () => {
    script(ok(listPage(items(1, 3), 3)));
    renderWithTheme(<TripsScreen search="" />);
    expect(screen.getByRole("status", { name: "Carregando viagens" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(await screen.findByRole("feed", { name: "Viagens" })).toBeInTheDocument();
    expect(cards()).toHaveLength(3);
    expect(screen.getByRole("heading", { name: "Viagens" })).toBeInTheDocument();
    expect(screen.getByText(/3 viagens · Mapas ©/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "OpenStreetMap" })).toHaveAttribute(
      "href",
      "https://www.openstreetmap.org/copyright",
    );
    expect(requests[0].get("limite")).toBe("24");
    expect(requests[0].has("cursor")).toBe(false);
    expect(requests[0].has("busca")).toBe(false);
  });

  it("the header has the navigation, with the list marked as current", async () => {
    script(ok(listPage(items(1, 1), 1)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    const nav = screen.getByRole("navigation", { name: "Principal" });
    expect(within(nav).getByRole("link", { name: "Viagens" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).getByRole("link", { name: "Nova viagem" })).toHaveAttribute(
      "href",
      "/processar",
    );
    expect(screen.getByRole("button", { name: "Tema claro" })).toBeInTheDocument();
  });

  it("the header is sticky, so the search and navigation stay in reach", async () => {
    script(ok(listPage(items(1, 1), 1)));
    const { container } = renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(container.querySelector("header")!.className).toContain("sticky");
  });

  it("opens each trip in its own page", async () => {
    script(ok(listPage(items(1, 2), 2)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    const hrefs = screen
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"))
      .filter((h) => h?.startsWith("/viagem/"));
    expect(hrefs).toEqual([
      "/viagem/7d1f3a10-2000-4c5e-9b1a-000000000001",
      "/viagem/7d1f3a10-2000-4c5e-9b1a-000000000002",
    ]);
  });
});

describe("infinite scroll", () => {
  it("asks for the next batch with the cursor when the end comes near, with a look-ahead", async () => {
    script(ok(listPage(items(1, 24), 30, "cur-1")), ok(listPage(items(25, 6), 30)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(cards()).toHaveLength(24);
    expect(requests).toHaveLength(1);
    expect(FakeObserver.active()[0].options?.rootMargin).toBe("600px 0px");

    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(30));
    expect(requests).toHaveLength(2);
    expect(requests[1].get("cursor")).toBe("cur-1");
    // The first batch stays, in order.
    expect(cards()[0]).toHaveTextContent("Motorista 1");
    expect(cards()[29]).toHaveTextContent("Motorista 30");
  });

  it("does not repeat the request while a batch is on its way", async () => {
    let release: (r: Reply) => void = () => undefined;
    const second = new Promise<Reply>((ok) => (release = ok));
    script(ok(listPage(items(1, 24), 30, "cur-1")), () => second);
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");

    await reachEnd();
    // The skeletons of the batch are shown and nothing observes the end meanwhile.
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(FakeObserver.active()).toHaveLength(0);
    expect(screen.getByRole("feed")).toHaveAttribute("aria-busy", "true");
    await act(async () => void (await Promise.resolve()));
    expect(requests).toHaveLength(2);

    await act(async () => release({ status: 200, body: listPage(items(25, 6), 30) }));
    await waitFor(() => expect(cards()).toHaveLength(30));
    expect(requests).toHaveLength(2);
  });

  it("keeps asking while the end is still in sight after a batch", async () => {
    script(
      ok(listPage(items(1, 2), 6, "c1")),
      ok(listPage(items(3, 2), 6, "c2")),
      ok(listPage(items(5, 2), 6)),
    );
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(4));
    // A new observer was made for the new end, and it reports on its own.
    await reachEnd();
    await waitFor(() => expect(cards()).toHaveLength(6));
  });

  it("at the end, says all were loaded and stops observing", async () => {
    script(ok(listPage(items(1, 24), 26, "c1")), ok(listPage(items(25, 2), 26)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await reachEnd();
    expect(await screen.findByText("Todas as 26 viagens foram carregadas.")).toBeInTheDocument();
    expect(FakeObserver.active()).toHaveLength(0);
    expect(screen.queryByRole("button", { name: "Carregar mais" })).toBeNull();
  });

  it("cleans the observer up when the screen goes away", async () => {
    script(ok(listPage(items(1, 24), 30, "c1")));
    const { unmount } = renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(FakeObserver.active()).toHaveLength(1);
    unmount();
    expect(FakeObserver.active()).toHaveLength(0);
  });

  it("an observer that reports 'not intersecting' asks for nothing", async () => {
    script(ok(listPage(items(1, 24), 30, "c1")));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await act(async () => FakeObserver.active()[0].fire(false));
    expect(requests).toHaveLength(1);
  });

  it("a failed batch keeps what is on screen and offers 'Tentar de novo'", async () => {
    vi.useFakeTimers();
    let failing = true;
    script(ok(listPage(items(1, 24), 30, "c1")), () =>
      failing ? problem(503, "banco_ocupado")() : ok(listPage(items(25, 6), 30))(),
    );
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    await act(async () => FakeObserver.active()[0].fire());
    for (const ms of [1000, 2000, 4000]) {
      await act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
    }
    const retry = screen.getByRole("button", { name: "Tentar de novo" });
    expect(cards()).toHaveLength(24);
    // The failure is in the footer, not a banner over the list.
    expect(screen.queryByText(/Mostrando o que já foi carregado/)).toBeNull();
    // A failed list is not asked again by scrolling.
    expect(FakeObserver.active()).toHaveLength(0);

    failing = false;
    await act(async () => fireEvent.click(retry));
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    expect(cards()).toHaveLength(30);
    expect(requests[requests.length - 1].get("cursor")).toBe("c1");
    expect(screen.queryByRole("button", { name: "Tentar de novo" })).toBeNull();
  });

  it("a network failure is retried a few times before it shows", async () => {
    vi.useFakeTimers();
    script(ok(listPage(items(1, 24), 30, "c1")), () => new TypeError("failed"));
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    expect(cards()).toHaveLength(24);
    await act(async () => FakeObserver.active()[0].fire());
    expect(requests).toHaveLength(2);
    await act(async () => void (await vi.advanceTimersByTimeAsync(1000)));
    await act(async () => void (await vi.advanceTimersByTimeAsync(2000)));
    await act(async () => void (await vi.advanceTimersByTimeAsync(4000)));
    // The first attempt and three retries.
    expect(requests).toHaveLength(5);
    expect(screen.getByRole("button", { name: "Tentar de novo" })).toBeInTheDocument();
    await act(async () => void (await vi.advanceTimersByTimeAsync(30_000)));
    expect(requests).toHaveLength(5);
  });

  it("without IntersectionObserver it offers 'Carregar mais'", async () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    script(ok(listPage(items(1, 24), 30, "c1")), ok(listPage(items(25, 6), 30)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(FakeObserver.instances).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "Carregar mais" }));
    await waitFor(() => expect(cards()).toHaveLength(30));
    expect(screen.queryByRole("button", { name: "Carregar mais" })).toBeNull();
    expect(screen.getByText("Todas as 30 viagens foram carregadas.")).toBeInTheDocument();
  });
});

describe("search", () => {
  it("the field starts from the URL and the list asks for that search", async () => {
    script(ok(listPage(items(1, 1), 1)));
    renderWithTheme(<TripsScreen search="joão" />);
    await screen.findByRole("feed");
    expect(screen.getByRole("searchbox", { name: "Buscar motorista" })).toHaveValue("joão");
    expect(requests[0].get("busca")).toBe("joão");
    expect(screen.getByText(/1 viagem encontradas · Mapas/)).toBeInTheDocument();
  });

  it("waits 300 ms after typing, then replaces the URL and goes back to the top", async () => {
    vi.useFakeTimers();
    script(ok(listPage(items(1, 2), 2)));
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    const box = screen.getByRole("searchbox", { name: "Buscar motorista" });

    fireEvent.change(box, { target: { value: "ma" } });
    await act(async () => void (await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 50)));
    fireEvent.change(box, { target: { value: "maria " } });
    await act(async () => void (await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 50)));
    expect(router.replace).not.toHaveBeenCalled();
    await act(async () => void (await vi.advanceTimersByTimeAsync(60)));
    expect(router.replace).toHaveBeenCalledTimes(1);
    expect(router.replace).toHaveBeenCalledWith("/viagens?busca=maria");
    expect(window.scrollTo).toHaveBeenCalled();

    // Nothing more is pushed for the same text.
    await act(async () => void (await vi.advanceTimersByTimeAsync(2000)));
    expect(router.replace).toHaveBeenCalledTimes(1);
  });

  it("emptying the field goes back to the plain list", async () => {
    vi.useFakeTimers();
    script(ok(listPage(items(1, 2), 2)));
    renderWithTheme(<TripsScreen search="maria" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "" } });
    await act(async () => void (await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS)));
    expect(router.replace).toHaveBeenCalledWith("/viagens");
  });

  it("keeps the previous list while the new search loads", async () => {
    let release: (r: Reply) => void = () => undefined;
    const pending = new Promise<Reply>((ok) => (release = ok));
    script(ok(listPage(items(1, 2), 2)));
    const view = renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    script(() => pending);
    view.rerender(<TripsScreen search="ana" />);
    // Still the old cards, marked busy, no skeleton flash.
    expect(cards()).toHaveLength(2);
    expect(screen.getByRole("feed")).toHaveAttribute("aria-busy", "true");
    expect(FakeObserver.active()).toHaveLength(0);
    await act(async () => release({ status: 200, body: listPage(items(7, 1), 1) }));
    await waitFor(() => expect(cards()).toHaveLength(1));
    expect(requests[0].get("busca")).toBe("ana");
  });

  it("no result: says so and 'Limpar busca' clears the search", async () => {
    script(ok(listPage([], 0)));
    renderWithTheme(<TripsScreen search="zé" />);
    expect(await screen.findByText("Nenhuma viagem encontrada para «zé»")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Limpar busca" }));
    expect(router.replace).toHaveBeenCalledWith("/viagens");
    expect(screen.getByRole("searchbox")).toHaveValue("");
  });

  it("following the URL (back button) updates the field", async () => {
    script(ok(listPage(items(1, 1), 1)));
    const view = renderWithTheme(<TripsScreen search="ana" />);
    await screen.findByRole("feed");
    view.rerender(<TripsScreen search="" />);
    await waitFor(() => expect(screen.getByRole("searchbox")).toHaveValue(""));
    expect(router.replace).not.toHaveBeenCalled();
  });
});

describe("states", () => {
  it("empty list: 'Nenhuma viagem ainda' and a way to process one", async () => {
    script(ok(listPage([], 0)));
    renderWithTheme(<TripsScreen search="" />);
    expect(await screen.findByText("Nenhuma viagem ainda")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Processar viagem" })).toHaveAttribute(
      "href",
      "/processar",
    );
    expect(screen.queryByRole("feed")).toBeNull();
  });

  it("error on the first batch: ErrorState, and 'Tentar de novo' loads it", async () => {
    script(problem(400, "entrada_invalida"), ok(listPage(items(1, 2), 2)));
    renderWithTheme(<TripsScreen search="" />);
    expect(await screen.findByText("Não foi possível carregar as viagens.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Tentar de novo" }));
    await waitFor(() => expect(cards()).toHaveLength(2));
  });

  it("an expired session goes to the login, coming back to the list with the search", async () => {
    script(problem(401, "nao_autenticado"));
    renderWithTheme(<TripsScreen search="ana" />);
    await waitFor(() =>
      expect(router.replace).toHaveBeenCalledWith(
        `/entrar?retorno=${encodeURIComponent("/viagens?busca=ana")}`,
      ),
    );
    expect(requests).toHaveLength(1);
  });

  it("a refresh that fails shows a banner and keeps the list", async () => {
    vi.useFakeTimers();
    script(
      ok(listPage([listItem(1, { status: "pendente", rota_resumo: null })], 1)),
      problem(400, "entrada_invalida"),
    );
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    await act(async () => void (await vi.advanceTimersByTimeAsync(POLL_PROVISIONAL_MS)));
    // The refresh is retried a few times (1, 2 and 4 s) before it shows.
    await act(async () => void (await vi.advanceTimersByTimeAsync(7000)));
    expect(screen.getByText(/Não foi possível atualizar a lista/)).toBeInTheDocument();
    expect(cards()).toHaveLength(1);
  });
});

describe("polling", () => {
  const pending = (n: number) =>
    listItem(n, { status: "pendente", rota_resumo: null, distancia_km: null, duracao_s: null });

  it("refreshes every 5 s while a loaded trip is processing, and stops when it is ready", async () => {
    vi.useFakeTimers();
    script(
      ok(listPage([pending(1)], 1)),
      ok(listPage([{ ...pending(1), status: "aguardando_dados" }], 1)),
      ok(listPage([listItem(1)], 1)),
    );
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    expect(requests).toHaveLength(1);
    expect(screen.getAllByText("Processando").length).toBeGreaterThan(0);

    await act(async () => void (await vi.advanceTimersByTimeAsync(POLL_PROVISIONAL_MS - 100)));
    expect(requests).toHaveLength(1);
    await act(async () => void (await vi.advanceTimersByTimeAsync(100)));
    expect(requests).toHaveLength(2);
    await act(async () => void (await vi.advanceTimersByTimeAsync(POLL_PROVISIONAL_MS)));
    expect(requests).toHaveLength(3);
    expect(screen.queryByText("Processando")).toBeNull();
    expect(screen.getByText("12,3 km · 30 min 20 s")).toBeInTheDocument();

    await act(async () => void (await vi.advanceTimersByTimeAsync(60_000)));
    expect(requests).toHaveLength(3);
  });

  it("does not poll when every trip is settled, even a provisional one", async () => {
    vi.useFakeTimers();
    script(ok(listPage([listItem(1, { provisoria: true }), listItem(2)], 2)));
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    await act(async () => void (await vi.advanceTimersByTimeAsync(60_000)));
    expect(requests).toHaveLength(1);
  });

  it("a refresh fetches again every batch already loaded", async () => {
    vi.useFakeTimers();
    script(
      ok(listPage([pending(1)], 3, "c1")),
      ok(listPage([listItem(2)], 3)),
      ok(listPage([listItem(1)], 3, "c1")),
      ok(listPage([listItem(2)], 3)),
    );
    renderWithTheme(<TripsScreen search="" />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    await act(async () => FakeObserver.active()[0].fire());
    await act(async () => void (await vi.advanceTimersByTimeAsync(0)));
    expect(requests).toHaveLength(2);
    await act(async () => void (await vi.advanceTimersByTimeAsync(POLL_PROVISIONAL_MS)));
    expect(requests).toHaveLength(4);
    expect(requests[2].has("cursor")).toBe(false);
    expect(requests[3].get("cursor")).toBe("c1");
  });
});

describe("requests go through the BFF", () => {
  it("under the base path", async () => {
    script(ok(listPage(items(1, 1), 1)));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    const f = fetch as unknown as ReturnType<typeof vi.fn>;
    expect(String(f.mock.calls[0][0]).startsWith(`${BASE_PATH}/api/trips?`)).toBe(true);
  });
});

describe("delete", () => {
  let remaining: TripListItem[] = [];
  let deleteReply: { status: number; body?: unknown } = { status: 204 };
  const deletes: string[] = [];

  /** A tiny API: GET lists what remains, DELETE removes the trip (unless the reply is an error). */
  function api(initial: TripListItem[]) {
    remaining = [...initial];
    deleteReply = { status: 204 };
    deletes.length = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === "DELETE") {
          const id = decodeURIComponent(url.split("/").pop()!);
          deletes.push(id);
          if (deleteReply.status === 204 || deleteReply.status === 404) {
            remaining = remaining.filter((t) => t.viagem_id !== id);
          }
          return {
            status: deleteReply.status,
            json: async () => {
              if (deleteReply.body === undefined) throw new SyntaxError("no body");
              return deleteReply.body;
            },
            headers: new Headers(),
          };
        }
        return {
          status: 200,
          json: async () => listPage(remaining, remaining.length),
          headers: new Headers(),
        };
      }),
    );
  }

  const trash = (n: number) =>
    screen.getByRole("button", { name: `Excluir a viagem de Motorista ${n}` });
  const confirmDialog = () => screen.getByRole("dialog", { name: /^Excluir a viagem de/ });

  it("the trash button opens the dialog; Cancelar closes it and the card stays", async () => {
    api(items(1, 3));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(screen.queryByRole("dialog")).toBeNull();
    await userEvent.click(trash(2));
    const dialog = confirmDialog();
    expect(dialog).toHaveAccessibleName("Excluir a viagem de Motorista 2?");
    expect(within(dialog).getByRole("button", { name: "Cancelar" })).toHaveFocus();
    expect(dialog).toHaveTextContent("Ela some da lista e da análise");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(deletes).toEqual([]);
    expect(cards()).toHaveLength(3);
  });

  it("confirming removes the card at once, updates the total, moves the focus and announces", async () => {
    api(items(1, 3));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    expect(cards()[1]).toHaveAttribute("aria-setsize", "3");
    expect(screen.getByText(/3 viagens · Mapas ©/)).toBeInTheDocument();

    await userEvent.click(trash(2));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));

    await waitFor(() => expect(cards()).toHaveLength(2));
    expect(deletes).toEqual(["7d1f3a10-2000-4c5e-9b1a-000000000002"]);
    expect(screen.queryByText("Motorista 2")).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
    // The total follows (the next card, 3, is now the second of two).
    expect(cards()[1]).toHaveAttribute("aria-setsize", "2");
    expect(cards()[1]).toHaveAttribute("aria-posinset", "2");
    expect(screen.getByText(/2 viagens · Mapas ©/)).toBeInTheDocument();
    expect(screen.getByText("Todas as 2 viagens foram carregadas.")).toBeInTheDocument();
    // The focus goes to the card that took its place.
    expect(within(cards()[1]).getByRole("link")).toHaveFocus();
    expect(screen.getByText("Viagem excluída")).toHaveAttribute("role", "status");
    // The refresh that follows keeps it out.
    await waitFor(() => expect(cards()).toHaveLength(2));
  });

  it("deleting the last card moves the focus to the previous one", async () => {
    api(items(1, 3));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(3));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    await waitFor(() => expect(cards()).toHaveLength(2));
    expect(within(cards()[1]).getByRole("link")).toHaveFocus();
    expect(within(cards()[1]).getByText("Motorista 2")).toBeInTheDocument();
  });

  it("deleting the only trip shows the empty state", async () => {
    api(items(1, 1));
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(1));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    expect(await screen.findByText("Nenhuma viagem ainda")).toBeInTheDocument();
    expect(screen.getByText("Viagem excluída")).toBeInTheDocument();
  });

  it("a 404 means it is already gone: the card leaves as a success", async () => {
    api(items(1, 2));
    deleteReply = { status: 404, body: { codigo: "viagem_desconhecida", title: "t", status: 404 } };
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(1));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    await waitFor(() => expect(screen.queryByText("Motorista 1")).toBeNull());
    expect(screen.getByText("Viagem excluída")).toBeInTheDocument();
  });

  it("a failure keeps the card and the dialog, with the error", async () => {
    api(items(1, 2));
    deleteReply = { status: 503, body: { codigo: "banco_ocupado", title: "t", status: 503 } };
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(1));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    expect(await screen.findByText("Não foi possível excluir. Tente de novo.")).toBeInTheDocument();
    expect(confirmDialog()).toHaveAttribute("open");
    expect(cards()).toHaveLength(2);
    expect(screen.queryByText("Viagem excluída")).toBeNull();
    expect(router.replace).not.toHaveBeenCalled();
    // The operator can try again.
    deleteReply = { status: 204 };
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    await waitFor(() => expect(cards()).toHaveLength(1));
  });

  it("a 401 sends the operator to the login, back to the list", async () => {
    api(items(1, 2));
    deleteReply = { status: 401, body: { codigo: "nao_autenticado", title: "t", status: 401 } };
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(1));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    await waitFor(() =>
      expect(router.replace).toHaveBeenCalledWith(
        `/entrar?retorno=${encodeURIComponent("/viagens")}`,
      ),
    );
  });

  it("while the request runs the buttons are disabled and say Excluindo…", async () => {
    api(items(1, 2));
    let release: () => void = () => undefined;
    const gate = new Promise<void>((ok) => (release = ok));
    const inner = vi.mocked(fetch).getMockImplementation()!;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === "DELETE") await gate;
        return inner(url, init);
      }),
    );
    renderWithTheme(<TripsScreen search="" />);
    await screen.findByRole("feed");
    await userEvent.click(trash(1));
    await userEvent.click(within(confirmDialog()).getByRole("button", { name: "Excluir" }));
    const dialog = confirmDialog();
    expect(within(dialog).getByRole("button", { name: "Excluindo…" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Cancelar" })).toBeDisabled();
    await act(async () => release());
    await waitFor(() => expect(cards()).toHaveLength(1));
  });
});
