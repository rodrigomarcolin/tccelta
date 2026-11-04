import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { analiseDe } from "@/testes/helpers";
import type { PropsMapa } from "./MapaRotaLeaflet";

/* Leaflet falso: registra o que é criado e guarda os manipuladores de evento. */
type Obj = Record<string, unknown>;
const estado = vi.hoisted(() => ({
  criados: [] as { tipo: string; args: unknown[]; obj: Record<string, unknown> }[],
  mapa: null as null | Record<string, unknown>,
  mapaHandlers: {} as Record<string, (e: unknown) => void>,
  tileHandlers: [] as Record<string, (() => void)[]>[],
  tiles: [] as Record<string, unknown>[],
}));

vi.mock("leaflet", () => {
  const novo = (tipo: string, args: unknown[], extra: Obj = {}) => {
    const obj: Obj = {};
    const proxy = new Proxy(obj, {
      get(_t, p: string) {
        if (p in extra) return extra[p];
        if (p === "then") return undefined;
        if (p.startsWith("chamou_")) return (obj as Record<string, unknown>)[p];
        return (...a: unknown[]) => {
          (obj as Record<string, unknown[]>)[`chamou_${p}`] = [
            ...(((obj as Record<string, unknown[]>)[`chamou_${p}`] as unknown[]) ?? []),
            a,
          ];
          return proxy;
        };
      },
    });
    estado.criados.push({ tipo, args, obj: proxy as Record<string, unknown> });
    return proxy;
  };
  const fabrica =
    (tipo: string) =>
    (...args: unknown[]) =>
      novo(tipo, args);
  const lib = {
    map: (...args: unknown[]) => {
      const m = novo("map", args, {
        on: (ev: string, cb: (e: unknown) => void) => {
          estado.mapaHandlers[ev] = cb;
          return m;
        },
        getZoom: () => 15,
        addLayer: () => m,
      });
      estado.mapa = m as Record<string, unknown>;
      return m;
    },
    layerGroup: () => novo("grupo", []),
    tileLayer: (...args: unknown[]) => {
      const handlers: Record<string, (() => void)[]> = {};
      estado.tileHandlers.push(handlers);
      const t = novo("tile", args, {
        on: (ev: string, cb: () => void) => {
          (handlers[ev] ??= []).push(cb);
          return t;
        },
      });
      estado.tiles.push(t as Record<string, unknown>);
      return t;
    },
    polyline: fabrica("polyline"),
    circleMarker: fabrica("circulo"),
    marker: fabrica("marcador"),
    divIcon: (o: unknown) => ({ icone: o }),
    latLngBounds: fabrica("limites"),
  };
  return { default: lib, ...lib };
});
vi.mock("leaflet/dist/leaflet.css", () => ({}));

import MapaRotaLeaflet from "./MapaRotaLeaflet";

const tipos = (t: string) => estado.criados.filter((c) => c.tipo === t);
const cores = () => tipos("polyline").map((p) => (p.args[1] as { color: string }).color);

function props(extra: Partial<PropsMapa> = {}): PropsMapa {
  const a = analiseDe("normal");
  return {
    rota: a.rota!,
    eventos: a.eventos,
    paradas: a.paradas,
    tema: "escuro",
    cursorT: null,
    onHoverT: vi.fn(),
    foco: null,
    formatarT: (t) => `t${t}`,
    ...extra,
  };
}

async function montar(p: PropsMapa) {
  const r = render(<MapaRotaLeaflet {...p} />);
  await waitFor(() => expect(tipos("polyline").length).toBeGreaterThan(0));
  return r;
}

beforeEach(() => {
  estado.criados = [];
  estado.mapa = null;
  estado.mapaHandlers = {};
  estado.tileHandlers = [];
  estado.tiles = [];
  document.documentElement.style.setProperty("--faixa-ate_30", "#8c3326");
  document.documentElement.style.setProperty("--faixa-falha", "#8b97a6");
});

describe("rota", () => {
  it("desenha contorno branco por baixo e a cor da faixa por cima, com A, B, paradas e eventos", async () => {
    const p = props();
    await montar(p);
    const linhas = tipos("polyline").map((l) => l.args[1] as { color: string; weight: number });
    const contornos = linhas.filter((l) => l.weight === 8);
    expect(contornos.length).toBeGreaterThan(100);
    expect(contornos.every((l) => l.color === "#ffffff" || l.color === "")).toBe(true);
    const faixas = linhas.filter((l) => l.weight === 5);
    expect(faixas.length).toBe(contornos.length);
    // Marcadores A e B.
    const icones = tipos("marcador").map(
      (m) => (m.args[1] as { icon: { icone: { html: string } } }).icon.icone.html,
    );
    expect(icones.some((h) => h.includes(">A<"))).toBe(true);
    expect(icones.some((h) => h.includes(">B<"))).toBe(true);
    // Eventos: só frenagem e aceleração; paradas como círculo.
    expect(icones.filter((h) => h.includes("polygon"))).toHaveLength(2);
    expect(tipos("circulo")).toHaveLength(p.paradas.length);
  });

  it("trecho sem dados é linha cinza fina, sem contorno", async () => {
    const a = analiseDe("gps_falhas");
    await montar(props({ rota: a.rota!, eventos: a.eventos, paradas: a.paradas }));
    const linhas = tipos("polyline").map((l) => l.args[1] as { weight: number });
    expect(linhas.filter((l) => l.weight === 2)).toHaveLength(2);
  });

  it("não cria marcador de evento sem posição nem parada sem posição", async () => {
    const a = analiseDe("normal");
    a.eventos.forEach((e) => ((e.lat = null), (e.lon = null)));
    a.paradas.forEach((s) => ((s.lat = null), (s.lon = null)));
    await montar(props({ rota: a.rota!, eventos: a.eventos, paradas: a.paradas }));
    expect(tipos("circulo")).toHaveLength(0);
    expect(tipos("marcador")).toHaveLength(2); // só A e B
  });

  it("enquadra a rota", async () => {
    await montar(props());
    expect(tipos("limites")).toHaveLength(1);
    expect((estado.mapa as Record<string, unknown[]>).chamou_fitBounds).toHaveLength(1);
  });

  it("sem coordenadas, usa uma vista padrão", async () => {
    const a = analiseDe("normal");
    a.rota!.lat = a.rota!.lat.map(() => null);
    a.rota!.lon = a.rota!.lon.map(() => null);
    render(<MapaRotaLeaflet {...props({ rota: a.rota! })} />);
    await waitFor(() => expect(estado.mapa).not.toBeNull());
    await waitFor(() =>
      expect((estado.mapa as Record<string, unknown[]>).chamou_setView).toBeDefined(),
    );
  });
});

describe("mapa-base", () => {
  it("indisponível depois de falhar sem nenhum carregamento: fundo liso e legenda pequena", async () => {
    await montar(props());
    const erro = estado.tileHandlers[0].tileerror;
    act(() => erro.forEach((cb) => cb()));
    act(() => erro.forEach((cb) => cb()));
    expect(screen.queryByText("Mapa-base indisponível")).toBeNull();
    act(() => erro.forEach((cb) => cb()));
    expect(screen.getByText("Mapa-base indisponível")).toBeInTheDocument();
    expect(estado.tiles[0].chamou_remove).toBeDefined();
    // A rota continua desenhada e nada cobre a tela.
    expect(tipos("polyline").length).toBeGreaterThan(0);
  });

  it("não cai no fundo liso se algum tile já carregou", async () => {
    await montar(props());
    act(() => estado.tileHandlers[0].tileload.forEach((cb) => cb()));
    for (let i = 0; i < 5; i++) act(() => estado.tileHandlers[0].tileerror.forEach((cb) => cb()));
    expect(screen.queryByText("Mapa-base indisponível")).toBeNull();
  });

  it("mostra só a camada do tema ativo", async () => {
    await montar(props());
    const [claro, escuro] = estado.tiles as unknown as Record<string, unknown[][]>[];
    expect(claro.chamou_setOpacity.at(-1)).toEqual([0]);
    expect(escuro.chamou_setOpacity.at(-1)).toEqual([1]);
  });

  it("no tema claro mostra a camada clara", async () => {
    await montar(props({ tema: "claro" }));
    const [claro, escuro] = estado.tiles as unknown as Record<string, unknown[][]>[];
    expect(claro.chamou_setOpacity.at(-1)).toEqual([1]);
    expect(escuro.chamou_setOpacity.at(-1)).toEqual([0]);
  });
});

describe("interação", () => {
  it("passar o mouse perto da rota informa o instante; longe ou sair, null", async () => {
    const p = props();
    await montar(p);
    const alvo = 100;
    estado.mapaHandlers.mousemove({ latlng: { lat: p.rota.lat[alvo], lng: p.rota.lon[alvo] } });
    expect(p.onHoverT).toHaveBeenLastCalledWith(p.rota.t_ms[alvo]);
    estado.mapaHandlers.mousemove({ latlng: { lat: -10, lng: -10 } });
    expect(p.onHoverT).toHaveBeenLastCalledWith(null);
    estado.mapaHandlers.mouseout({});
    expect(p.onHoverT).toHaveBeenLastCalledWith(null);
  });

  it("cria, move e remove o marcador do cursor", async () => {
    const p = props();
    const { rerender } = await montar(p);
    expect(tipos("circulo").length).toBe(p.paradas.length);
    const t1 = p.rota.t_ms[5] as number;
    rerender(<MapaRotaLeaflet {...p} cursorT={t1} />);
    await waitFor(() => expect(tipos("circulo").length).toBe(p.paradas.length + 1));
    const cursor = tipos("circulo").at(-1)!;
    rerender(<MapaRotaLeaflet {...p} cursorT={p.rota.t_ms[9] as number} />);
    await waitFor(() => expect(cursor.obj.chamou_setLatLng).toBeDefined());
    rerender(<MapaRotaLeaflet {...p} cursorT={null} />);
    await waitFor(() => expect(cursor.obj.chamou_remove).toBeDefined());
  });

  it("sem posição de GPS no instante, não mostra marcador", async () => {
    const p = props();
    const { rerender } = await montar(p);
    rerender(<MapaRotaLeaflet {...p} cursorT={99_999_999} />);
    expect(tipos("circulo").length).toBe(p.paradas.length);
  });

  it("'ver no mapa' centraliza a posição", async () => {
    const p = props();
    const { rerender } = await montar(p);
    rerender(<MapaRotaLeaflet {...p} foco={{ lat: -25.4, lon: -49.2, nonce: 1 }} />);
    await waitFor(() =>
      expect((estado.mapa as Record<string, unknown[][]>).chamou_setView?.at(-1)?.[0]).toEqual([
        -25.4, -49.2,
      ]),
    );
  });

  it("redesenha com as cores claras na impressão", async () => {
    const p = props();
    await montar(p);
    const antes = tipos("polyline").length;
    act(() => void fireEvent(window, new Event("beforeprint")));
    await waitFor(() => expect(tipos("polyline").length).toBeGreaterThan(antes));
    expect(cores().slice(antes)).toContain("#e8934a");
    const [claro] = estado.tiles as unknown as Record<string, unknown[][]>[];
    expect(claro.chamou_setOpacity.at(-1)).toEqual([1]);
    act(() => void fireEvent(window, new Event("afterprint")));
  });

  it("remove o mapa ao desmontar", async () => {
    const { unmount } = await montar(props());
    const m = estado.mapa!;
    unmount();
    expect(m.chamou_remove).toBeDefined();
  });
});
