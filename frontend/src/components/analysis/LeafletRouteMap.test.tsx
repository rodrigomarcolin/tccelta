import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { analysisFor } from "@/tests/helpers";
import type { MapProps } from "./LeafletRouteMap";

/* Fake Leaflet: records what is created and keeps the event handlers. */
type Obj = Record<string, unknown>;
const state = vi.hoisted(() => ({
  created: [] as { kind: string; args: unknown[]; obj: Record<string, unknown> }[],
  map: null as null | Record<string, unknown>,
  mapHandlers: {} as Record<string, (e: unknown) => void>,
  tileHandlers: [] as Record<string, (() => void)[]>[],
  tiles: [] as Record<string, unknown>[],
}));

vi.mock("leaflet", () => {
  const make = (kind: string, args: unknown[], extra: Obj = {}) => {
    const obj: Obj = {};
    const proxy = new Proxy(obj, {
      get(_t, p: string) {
        if (p in extra) return extra[p];
        if (p === "then") return undefined;
        if (p.startsWith("called_")) return (obj as Record<string, unknown>)[p];
        return (...a: unknown[]) => {
          (obj as Record<string, unknown[]>)[`called_${p}`] = [
            ...(((obj as Record<string, unknown[]>)[`called_${p}`] as unknown[]) ?? []),
            a,
          ];
          return proxy;
        };
      },
    });
    state.created.push({ kind, args, obj: proxy as Record<string, unknown> });
    return proxy;
  };
  const factory =
    (kind: string) =>
    (...args: unknown[]) =>
      make(kind, args);
  const lib = {
    map: (...args: unknown[]) => {
      const m = make("map", args, {
        on: (ev: string, cb: (e: unknown) => void) => {
          state.mapHandlers[ev] = cb;
          return m;
        },
        getZoom: () => 15,
        addLayer: () => m,
      });
      state.map = m as Record<string, unknown>;
      return m;
    },
    layerGroup: () => make("group", []),
    tileLayer: (...args: unknown[]) => {
      const handlers: Record<string, (() => void)[]> = {};
      state.tileHandlers.push(handlers);
      const t = make("tile", args, {
        on: (ev: string, cb: () => void) => {
          (handlers[ev] ??= []).push(cb);
          return t;
        },
      });
      state.tiles.push(t as Record<string, unknown>);
      return t;
    },
    polyline: factory("polyline"),
    circleMarker: factory("circle"),
    marker: factory("marker"),
    divIcon: (o: unknown) => ({ icon: o }),
    latLngBounds: factory("bounds"),
  };
  return { default: lib, ...lib };
});

import LeafletRouteMap from "./LeafletRouteMap";

const ofKind = (t: string) => state.created.filter((c) => c.kind === t);
const colors = () => ofKind("polyline").map((p) => (p.args[1] as { color: string }).color);

function props(extra: Partial<MapProps> = {}): MapProps {
  const a = analysisFor("normal");
  return {
    route: a.rota!,
    events: a.eventos,
    stops: a.paradas,
    theme: "dark",
    cursorT: null,
    onHoverT: vi.fn(),
    focus: null,
    formatT: (t) => `t${t}`,
    ...extra,
  };
}

async function mountMap(p: MapProps) {
  const r = render(<LeafletRouteMap {...p} />);
  await waitFor(() => expect(ofKind("polyline").length).toBeGreaterThan(0));
  return r;
}

beforeEach(() => {
  state.created = [];
  state.map = null;
  state.mapHandlers = {};
  state.tileHandlers = [];
  state.tiles = [];
  document.documentElement.style.setProperty("--band-ate_30", "#8c3326");
  document.documentElement.style.setProperty("--band-fault", "#8b97a6");
  document.documentElement.style.setProperty("--text-primary", "#111111");
  document.documentElement.style.setProperty("--surface-card", "#eeeeee");
});

describe("route", () => {
  it("draws a white outline underneath and the band color on top, with A, B, stops and events", async () => {
    const p = props();
    await mountMap(p);
    const lines = ofKind("polyline").map((l) => l.args[1] as { color: string; weight: number });
    const outlines = lines.filter((l) => l.weight === 8);
    expect(outlines.length).toBeGreaterThan(100);
    expect(outlines.every((l) => l.color === "#ffffff" || l.color === "")).toBe(true);
    const bandLines = lines.filter((l) => l.weight === 5);
    expect(bandLines.length).toBe(outlines.length);
    // A and B markers.
    const icons = ofKind("marker").map(
      (m) => (m.args[1] as { icon: { icon: { html: string } } }).icon.icon.html,
    );
    expect(icons.some((h) => h.includes(">A<"))).toBe(true);
    expect(icons.some((h) => h.includes(">B<"))).toBe(true);
    // Events: only braking and acceleration; stops as circles.
    const eventIcons = icons.filter((h) => h.includes("<circle"));
    expect(eventIcons).toHaveLength(2);
    expect(icons.some((h) => h.includes("<polygon"))).toBe(false);
    // Braking is filled, acceleration is empty: different fills.
    const fills = eventIcons.map((h) => /fill="([^"]*)"/.exec(h)?.[1]);
    expect(fills[0]).not.toBe(fills[1]);
    expect(ofKind("circle")).toHaveLength(p.stops.length);
  });

  it("a piece without data is a thin gray line, without outline", async () => {
    const a = analysisFor("gps_falhas");
    await mountMap(props({ route: a.rota!, events: a.eventos, stops: a.paradas }));
    const lines = ofKind("polyline").map((l) => l.args[1] as { weight: number });
    expect(lines.filter((l) => l.weight === 2)).toHaveLength(2);
  });

  it("creates no event marker without position nor stop without position", async () => {
    const a = analysisFor("normal");
    a.eventos.forEach((e) => ((e.lat = null), (e.lon = null)));
    a.paradas.forEach((s) => ((s.lat = null), (s.lon = null)));
    await mountMap(props({ route: a.rota!, events: a.eventos, stops: a.paradas }));
    expect(ofKind("circle")).toHaveLength(0);
    expect(ofKind("marker")).toHaveLength(2); // only A and B
  });

  it("fits the route in view", async () => {
    await mountMap(props());
    expect(ofKind("bounds")).toHaveLength(1);
    expect((state.map as Record<string, unknown[]>).called_fitBounds).toHaveLength(1);
  });

  it("without coordinates, uses a default view", async () => {
    const a = analysisFor("normal");
    a.rota!.lat = a.rota!.lat.map(() => null);
    a.rota!.lon = a.rota!.lon.map(() => null);
    render(<LeafletRouteMap {...props({ route: a.rota! })} />);
    await waitFor(() => expect(state.map).not.toBeNull());
    await waitFor(() =>
      expect((state.map as Record<string, unknown[]>).called_setView).toBeDefined(),
    );
  });
});

describe("compact (minimap)", () => {
  it("has no zoom control, no A/B, no stops nor events, and no mouse handlers", async () => {
    await mountMap(props({ compact: true, events: [], stops: [] }));
    const options = state.created.find((c) => c.kind === "map")!.args[1] as Record<string, unknown>;
    expect(options.zoomControl).toBe(false);
    expect(options.dragging).toBe(false);
    expect(options.keyboard).toBe(false);
    expect(ofKind("marker")).toHaveLength(0);
    expect(ofKind("circle")).toHaveLength(0);
    expect(state.mapHandlers.mousemove).toBeUndefined();
    expect(state.mapHandlers.mouseout).toBeUndefined();
  });

  it("still draws the cursor and fits the whole route", async () => {
    await mountMap(
      props({ compact: true, events: [], stops: [], cursorT: props().route.t_ms[5] as number }),
    );
    await waitFor(() => expect(ofKind("circle")).toHaveLength(1));
    expect((state.map as Record<string, unknown[]>).called_fitBounds).toHaveLength(1);
  });
});

describe("base map", () => {
  it("unavailable after failing with no tile loaded: plain background and small caption", async () => {
    await mountMap(props());
    const errorHandlers = state.tileHandlers[0].tileerror;
    act(() => errorHandlers.forEach((cb) => cb()));
    act(() => errorHandlers.forEach((cb) => cb()));
    expect(screen.queryByText("Mapa-base indisponível")).toBeNull();
    act(() => errorHandlers.forEach((cb) => cb()));
    expect(screen.getByText("Mapa-base indisponível")).toBeInTheDocument();
    expect(state.tiles[0].called_remove).toBeDefined();
    // The route stays drawn and nothing covers the screen.
    expect(ofKind("polyline").length).toBeGreaterThan(0);
  });

  it("does not fall back to the plain background if a tile already loaded", async () => {
    await mountMap(props());
    act(() => state.tileHandlers[0].tileload.forEach((cb) => cb()));
    for (let i = 0; i < 5; i++) act(() => state.tileHandlers[0].tileerror.forEach((cb) => cb()));
    expect(screen.queryByText("Mapa-base indisponível")).toBeNull();
  });

  it("shows only the active theme layer", async () => {
    await mountMap(props());
    const [light, dark] = state.tiles as unknown as Record<string, unknown[][]>[];
    expect(light.called_setOpacity.at(-1)).toEqual([0]);
    expect(dark.called_setOpacity.at(-1)).toEqual([1]);
  });

  it("in the light theme shows the light layer", async () => {
    await mountMap(props({ theme: "light" }));
    const [light, dark] = state.tiles as unknown as Record<string, unknown[][]>[];
    expect(light.called_setOpacity.at(-1)).toEqual([1]);
    expect(dark.called_setOpacity.at(-1)).toEqual([0]);
  });
});

describe("interaction", () => {
  it("hovering near the route reports the instant; far away or leaving, null", async () => {
    const p = props();
    await mountMap(p);
    const target = 100;
    state.mapHandlers.mousemove({ latlng: { lat: p.route.lat[target], lng: p.route.lon[target] } });
    expect(p.onHoverT).toHaveBeenLastCalledWith(p.route.t_ms[target]);
    state.mapHandlers.mousemove({ latlng: { lat: -10, lng: -10 } });
    expect(p.onHoverT).toHaveBeenLastCalledWith(null);
    state.mapHandlers.mouseout({});
    expect(p.onHoverT).toHaveBeenLastCalledWith(null);
  });

  it("creates, moves and removes the cursor marker", async () => {
    const p = props();
    const { rerender } = await mountMap(p);
    expect(ofKind("circle").length).toBe(p.stops.length);
    const t1 = p.route.t_ms[5] as number;
    rerender(<LeafletRouteMap {...p} cursorT={t1} />);
    await waitFor(() => expect(ofKind("circle").length).toBe(p.stops.length + 1));
    const cursor = ofKind("circle").at(-1)!;
    rerender(<LeafletRouteMap {...p} cursorT={p.route.t_ms[9] as number} />);
    await waitFor(() => expect(cursor.obj.called_setLatLng).toBeDefined());
    rerender(<LeafletRouteMap {...p} cursorT={null} />);
    await waitFor(() => expect(cursor.obj.called_remove).toBeDefined());
  });

  it("without a GPS position at the instant, shows no marker", async () => {
    const p = props();
    const { rerender } = await mountMap(p);
    rerender(<LeafletRouteMap {...p} cursorT={99_999_999} />);
    expect(ofKind("circle").length).toBe(p.stops.length);
  });

  it("'ver no mapa' centers on the position", async () => {
    const p = props();
    const { rerender } = await mountMap(p);
    rerender(<LeafletRouteMap {...p} focus={{ lat: -25.4, lon: -49.2, nonce: 1 }} />);
    await waitFor(() =>
      expect((state.map as Record<string, unknown[][]>).called_setView?.at(-1)?.[0]).toEqual([
        -25.4, -49.2,
      ]),
    );
  });

  it("redraws with the light colors when printing", async () => {
    const p = props();
    await mountMap(p);
    const before = ofKind("polyline").length;
    act(() => void fireEvent(window, new Event("beforeprint")));
    await waitFor(() => expect(ofKind("polyline").length).toBeGreaterThan(before));
    expect(colors().slice(before)).toContain("#e8934a");
    const [light] = state.tiles as unknown as Record<string, unknown[][]>[];
    expect(light.called_setOpacity.at(-1)).toEqual([1]);
    act(() => void fireEvent(window, new Event("afterprint")));
  });

  it("removes the map on unmount", async () => {
    const { unmount } = await mountMap(props());
    const m = state.map!;
    unmount();
    expect(m.called_remove).toBeDefined();
  });
});
