import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { listItem } from "@/tests/tripList";
import { renderWithTheme } from "@/tests/helpers";
import { TripCard } from "./TripCard";

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

const card = (extra: Parameters<typeof listItem>[1] = {}, theme: "dark" | "light" = "dark") =>
  renderWithTheme(
    <TripCard trip={listItem(8, extra)} position={1} size={10} onDelete={vi.fn()} />,
    theme,
  );

describe("TripCard", () => {
  it("is a single link to the analysis, without prefetch", () => {
    card();
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/viagem/7d1f3a10-2000-4c5e-9b1a-000000000008");
    expect(link).toHaveAttribute("data-prefetch", "false");
    expect(screen.getAllByRole("link")).toHaveLength(1);
  });

  it("shows driver, when, numbers, source and category for a ready trip", () => {
    card();
    const link = screen.getByRole("link");
    expect(within(link).getByText("Motorista 8")).toBeInTheDocument();
    // 17:03Z is 14:03 in the fixed timezone.
    expect(within(link).getByText("28/10/2026 14:03–14:33")).toBeInTheDocument();
    expect(within(link).getByText("12,3 km · 30 min 20 s")).toBeInTheDocument();
    expect(within(link).getByText("Dongle 1 · boot 8")).toBeInTheDocument();
    expect(within(link).getByText("categoria feira")).toBeInTheDocument();
    // Nothing unusual about it: no badge.
    expect(screen.queryByText(/Processando|Provisória|Falhou/)).toBeNull();
  });

  it("an end on another day carries its date", () => {
    card({ fim_em: "2026-10-29T17:33:00.000Z" });
    expect(screen.getByText("28/10/2026 14:03–29/10 14:33")).toBeInTheDocument();
  });

  it("provisional: warning badge, numbers still shown", () => {
    card({ provisoria: true });
    expect(screen.getByText("Provisória")).toBeInTheDocument();
    expect(screen.getByText("12,3 km · 30 min 20 s")).toBeInTheDocument();
  });

  it.each(["pendente", "aguardando_dados"] as const)(
    "%s: processing badge, creation time, no numbers and no route",
    (status) => {
      card({
        status,
        inicio_em: null,
        fim_em: null,
        distancia_km: null,
        duracao_s: null,
        rota_resumo: null,
      });
      // The badge and the minimap both say it.
      expect(screen.getAllByText("Processando")).toHaveLength(2);
      expect(screen.getByText("criada 28/10/2026 14:40")).toBeInTheDocument();
      expect(screen.queryByText(/km/)).toBeNull();
      expect(screen.queryByTestId("preview-wide")).toBeNull();
    },
  );

  it("failed: alert badge and no numbers", () => {
    card({ status: "falhou", distancia_km: null, duracao_s: null, rota_resumo: null });
    expect(screen.getAllByText("Falhou")).toHaveLength(2);
    expect(screen.queryByText(/km/)).toBeNull();
  });

  it("ready without route says there is no GPS", () => {
    card({ rota_resumo: null });
    expect(screen.getByText("Sem GPS")).toBeInTheDocument();
  });

  it("ready without any number says there is no data", () => {
    card({ distancia_km: null, duracao_s: null, inicio_em: null, fim_em: null });
    expect(screen.getByText("Sem dados")).toBeInTheDocument();
  });

  it("an interval shows no boot", () => {
    card({ entrada_tipo: "intervalo", boot_id: null, boot_generation: null });
    expect(screen.getByText("Dongle 1 · intervalo")).toBeInTheDocument();
  });

  it("shows a generation other than 0", () => {
    card({ boot_generation: 2 });
    expect(screen.getByText("Dongle 1 · boot 8 · ger. 2")).toBeInTheDocument();
  });

  it("is an article of the feed, positioned, and skips off-screen rendering", () => {
    const { container } = card();
    const article = container.querySelector("article")!;
    expect(article).toHaveAttribute("aria-posinset", "1");
    expect(article).toHaveAttribute("aria-setsize", "10");
    expect(article.className).toContain("content-visibility:auto");
    expect(article.className).toContain("contain-intrinsic-size");
  });
});

describe("minimap", () => {
  it("is hidden from assistive technology and draws tiles and the route", () => {
    const { container } = card();
    const wide = screen.getByTestId("preview-wide");
    expect(wide.closest("[aria-hidden='true']")).not.toBeNull();
    const tiles = wide.querySelectorAll("img");
    expect(tiles.length).toBeGreaterThan(0);
    expect(tiles.length).toBeLessThanOrEqual(4);
    for (const t of tiles) {
      expect(t).toHaveAttribute("loading", "lazy");
      expect(t).toHaveAttribute("alt", "");
      expect(t.getAttribute("src")).toMatch(
        /^https:\/\/tile\.openstreetmap\.org\/\d+\/\d+\/\d+\.png$/,
      );
    }
    // Outline and line of the route, and the A/B markers.
    expect(wide.querySelectorAll("path")).toHaveLength(2);
    expect(wide.textContent).toContain("A");
    expect(wide.textContent).toContain("B");
    expect(container.querySelector("[data-testid=preview-narrow]")).not.toBeNull();
  });

  it("darkens the tiles in the dark theme only", () => {
    const dark = card({}, "dark");
    expect(dark.container.querySelector("img")!.className).toContain("invert(1)");
    dark.unmount();
    const light = card({}, "light");
    expect(light.container.querySelector("img")!.className).not.toContain("invert(1)");
  });

  it("a tile that fails to load disappears and the route stays", async () => {
    card();
    const wide = screen.getByTestId("preview-wide");
    const before = wide.querySelectorAll("img").length;
    fireEvent.error(wide.querySelector("img")!);
    await waitFor(() => expect(wide.querySelectorAll("img").length).toBe(before - 1));
    expect(wide.querySelectorAll("path")).toHaveLength(2);
  });

  it("a trip that never moved draws a dot and no markers", () => {
    card({ rota_resumo: [[[-23.55, -46.64]]] });
    const wide = screen.getByTestId("preview-wide");
    expect(wide.querySelectorAll("path")).toHaveLength(0);
    expect(wide.querySelectorAll("circle")).toHaveLength(1);
    expect(wide.textContent).not.toContain("A");
  });

  it("a GPS gap draws one path per stretch", () => {
    card({
      rota_resumo: [
        [
          [-23.55, -46.64],
          [-23.545, -46.635],
        ],
        [
          [-23.535, -46.625],
          [-23.53, -46.62],
        ],
      ],
    });
    // Outline and line for each of the two stretches.
    expect(screen.getByTestId("preview-wide").querySelectorAll("path")).toHaveLength(4);
  });

  it("has a delete button beside the link, not inside it, named after the driver", () => {
    const onDelete = vi.fn();
    const trip = listItem(8);
    renderWithTheme(<TripCard trip={trip} position={1} size={10} onDelete={onDelete} />);
    const button = screen.getByRole("button", { name: "Excluir a viagem de Motorista 8" });
    expect(screen.getByRole("link").contains(button)).toBe(false);
    expect(button.closest("article")).toBe(screen.getByRole("article"));
    expect(button).toHaveAttribute("type", "button");
    fireEvent.click(button);
    expect(onDelete).toHaveBeenCalledExactlyOnceWith(trip);
  });

  it("renders without a theme provider too (dark by default)", () => {
    render(<TripCard trip={listItem(1)} position={1} size={1} onDelete={vi.fn()} />);
    expect(screen.getByTestId("preview-wide")).toBeInTheDocument();
  });
});
