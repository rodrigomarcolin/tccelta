"use client";

import { useEffect, useRef, useState } from "react";
import type * as Leaflet from "leaflet";
import "leaflet/dist/leaflet.css";
import { TILES_ATTRIBUTION, TILES, type Theme } from "@/config";
import { routePointAtPlace, routePointAtTime } from "@/domain/trip/search";
import { EVENT_LABEL } from "@/domain/trip/analysis";
import type { TripEvent, Stop, Route } from "@/domain/trip/types";
import { formatDuration } from "@/format/numbers";
import { cssColor } from "./colors";

export type MapProps = {
  route: Route;
  events: TripEvent[];
  stops: Stop[];
  theme: Theme;
  cursorT: number | null;
  onHoverT: (t: number | null) => void;
  focus: { lat: number; lon: number; nonce: number } | null;
  formatT: (t: number) => string;
};

const TILE_ERROR_LIMIT = 3;

function eventIcon(L: typeof Leaflet, tipo: TripEvent["tipo"]): Leaflet.DivIcon | null {
  if (tipo === "marcha_lenta_longa") return null;
  const outline = cssColor("--text-primary");
  const fillColor = cssColor("--surface-card");
  const polygonPoints = tipo === "frenagem_brusca" ? "2,3 16,3 9,16" : "2,15 16,15 9,2";
  return L.divIcon({
    className: "event-marker",
    html: `<svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true"><polygon points="${polygonPoints}" fill="${fillColor}" stroke="${outline}" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    iconSize: [18, 18],
    iconAnchor: [9, 9],
  });
}

function abIcon(L: typeof Leaflet, letter: "A" | "B"): Leaflet.DivIcon {
  return L.divIcon({
    className: "ab-marker-wrap",
    html: `<span class="ab-marker">${letter}</span>`,
    iconSize: [24, 24],
    iconAnchor: [12, 12],
  });
}

/** Leaflet map with the route colored by segment. Runs only on the client. */
export default function LeafletRouteMap(props: MapProps) {
  const { route, events, stops, theme, cursorT, focus } = props;
  const el = useRef<HTMLDivElement>(null);
  const L = useRef<typeof Leaflet | null>(null);
  const mapRef = useRef<Leaflet.Map | null>(null);
  const layerRef = useRef<Leaflet.LayerGroup | null>(null);
  const cursor = useRef<Leaflet.CircleMarker | null>(null);
  const tiles = useRef<{ light?: Leaflet.TileLayer; dark?: Leaflet.TileLayer }>({});
  const hoverCallback = useRef(props.onHoverT);
  useEffect(() => {
    hoverCallback.current = props.onHoverT;
  });
  const routeRef = useRef(route);
  useEffect(() => {
    routeRef.current = route;
  });
  const [ready, setReady] = useState(false);
  const [noBase, setNoBase] = useState(false);
  const [printing, setPrinting] = useState(false);

  // Map creation.
  useEffect(() => {
    let cancelled = false;
    let cleanup: (() => void) | undefined;
    void import("leaflet").then((mod) => {
      const lib = (mod as unknown as { default?: typeof Leaflet }).default ?? mod;
      if (cancelled || !el.current) return;
      L.current = lib;
      const m = lib.map(el.current, { preferCanvas: true, zoomControl: true });
      mapRef.current = m;
      layerRef.current = lib.layerGroup().addTo(m);

      let loaded = 0;
      let errors = 0;
      const tileOptions = { attribution: TILES_ATTRIBUTION, maxZoom: 19 };
      const light = lib.tileLayer(TILES.light, { ...tileOptions, className: "tile-light" });
      const dark = lib.tileLayer(TILES.dark, { ...tileOptions, className: "tile-dark" });
      for (const t of [light, dark]) {
        t.on("tileload", () => (loaded += 1));
        t.on("tileerror", () => {
          errors += 1;
          if (loaded === 0 && errors >= TILE_ERROR_LIMIT) {
            light.remove();
            dark.remove();
            setNoBase(true);
          }
        });
      }
      tiles.current = { light, dark };
      light.addTo(m);
      dark.addTo(m);

      const pts = routeRef.current.lat
        .map((la, i) => [la, routeRef.current.lon[i]] as [number | null, number | null])
        .filter((p): p is [number, number] => p[0] !== null && p[1] !== null);
      if (pts.length) m.fitBounds(lib.latLngBounds(pts), { padding: [24, 24] });
      else m.setView([-25.43, -49.27], 13);

      m.on("mousemove", (ev: Leaflet.LeafletMouseEvent) => {
        const metersPerPx =
          (156543.03 * Math.cos((ev.latlng.lat * Math.PI) / 180)) / 2 ** m.getZoom();
        const point = routePointAtPlace(
          routeRef.current,
          ev.latlng.lat,
          ev.latlng.lng,
          metersPerPx * 16,
        );
        hoverCallback.current(point ? point.t : null);
      });
      m.on("mouseout", () => hoverCallback.current(null));

      setReady(true);
      cleanup = () => {
        m.remove();
        mapRef.current = null;
        layerRef.current = null;
        cursor.current = null;
      };
    });
    return () => {
      cancelled = true;
      cleanup?.();
      setReady(false);
    };
  }, []);

  // Printing: redraws with the light theme colors.
  useEffect(() => {
    const before = () => {
      setPrinting(true);
      mapRef.current?.invalidateSize();
    };
    const after = () => setPrinting(false);
    window.addEventListener("beforeprint", before);
    window.addEventListener("afterprint", after);
    return () => {
      window.removeEventListener("beforeprint", before);
      window.removeEventListener("afterprint", after);
    };
  }, []);

  // Route, start and end markers, stops and events.
  useEffect(() => {
    const lib = L.current;
    const group = layerRef.current;
    if (!ready || !lib || !group) return;
    group.clearLayers();
    const forceLight = printing && theme !== "light";
    const pick = (name: string, light: string) => (forceLight ? light : cssColor(name));
    const LIGHT_BANDS: Record<string, string> = {
      parado: "#7b8794",
      ate_30: "#e8934a",
      "30_60": "#d9703a",
      "60_80": "#be5030",
      "80_110": "#983528",
      acima_110: "#6a1f22",
      falha_gps: "#7b8794",
      sem_velocidade: "#7b8794",
    };
    const bandColor = (f: string) =>
      forceLight
        ? (LIGHT_BANDS[f] as string)
        : cssColor(f === "falha_gps" || f === "sem_velocidade" ? "--band-fault" : `--band-${f}`);

    const outline = cssColor("--route-outline", "#ffffff");
    const segs = route.segmentos.faixa;
    const pieces: { a: [number, number]; b: [number, number]; band: string }[] = [];
    for (let i = 0; i < segs.length; i++) {
      const la = route.lat[i];
      const lo = route.lon[i];
      const lb = route.lat[i + 1];
      const ob = route.lon[i + 1];
      if (
        la === null ||
        lo === null ||
        lb === null ||
        ob === null ||
        la === undefined ||
        lb === undefined
      )
        continue;
      pieces.push({ a: [la, lo as number], b: [lb, ob as number], band: segs[i] });
    }
    // White outline underneath, only where there is a speed band.
    for (const t of pieces) {
      if (t.band === "falha_gps" || t.band === "sem_velocidade") continue;
      lib
        .polyline([t.a, t.b], {
          color: outline,
          weight: 8,
          opacity: 1,
          lineCap: "round",
          interactive: false,
        })
        .addTo(group);
    }
    for (const t of pieces) {
      const noData = t.band === "falha_gps" || t.band === "sem_velocidade";
      lib
        .polyline([t.a, t.b], {
          color: bandColor(t.band),
          weight: noData ? 2 : 5,
          opacity: noData ? 0.9 : 1,
          lineCap: "round",
          interactive: false,
        })
        .addTo(group);
    }

    for (const p of stops) {
      if (p.lat === null || p.lon === null) continue;
      const dotRadius = Math.min(14, Math.max(6, 6 + p.duracao_s * 0.35));
      lib
        .circleMarker([p.lat, p.lon], {
          radius: dotRadius,
          color: pick("--text-secondary", "#46525f"),
          weight: 1.5,
          fillColor: pick("--band-parado", "#7b8794"),
          fillOpacity: 0.55,
        })
        .bindTooltip(`Parada · ${formatDuration(p.duracao_s)}`)
        .addTo(group);
    }

    for (const e of events) {
      if (e.lat === null || e.lon === null) continue;
      const icon = eventIcon(lib, e.tipo);
      if (!icon) continue;
      lib
        .marker([e.lat, e.lon], {
          icon,
          title: `${EVENT_LABEL[e.tipo]} às ${props.formatT(e.t_inicio_ms)}`,
          alt: EVENT_LABEL[e.tipo],
          keyboard: true,
        })
        .addTo(group);
    }

    const valid = pieces.length ? pieces : [];
    const first = valid[0]?.a ?? null;
    const last = valid.length ? valid[valid.length - 1].b : null;
    if (first)
      lib
        .marker(first, { icon: abIcon(lib, "A"), title: "Início da viagem", keyboard: false })
        .addTo(group);
    if (last)
      lib
        .marker(last, { icon: abIcon(lib, "B"), title: "Fim da viagem", keyboard: false })
        .addTo(group);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, route, events, stops, theme, printing]);

  // Shared cursor position.
  useEffect(() => {
    const lib = L.current;
    const m = mapRef.current;
    if (!ready || !lib || !m) return;
    const point = cursorT === null ? null : routePointAtTime(route, cursorT);
    if (!point) {
      cursor.current?.remove();
      cursor.current = null;
      return;
    }
    if (!cursor.current) {
      cursor.current = lib
        .circleMarker([point.lat, point.lon], {
          radius: 7,
          color: cssColor("--accent"),
          weight: 3,
          fillColor: cssColor("--surface-card"),
          fillOpacity: 1,
          interactive: false,
        })
        .addTo(m);
    } else {
      cursor.current.setLatLng([point.lat, point.lon]);
    }
  }, [ready, cursorT, route, theme]);

  // "Ver no mapa": centers on the event.
  useEffect(() => {
    const m = mapRef.current;
    if (!ready || !m || !focus) return;
    m.setView([focus.lat, focus.lon], Math.max(m.getZoom(), 16), { animate: false });
  }, [ready, focus]);

  // Theme: shows only the active theme layer (the light one also when printing).
  useEffect(() => {
    const { light, dark } = tiles.current;
    const useLight = theme === "light" || printing;
    light?.setOpacity(useLight ? 1 : 0);
    dark?.setOpacity(useLight ? 0 : 1);
  }, [ready, theme, printing]);

  return (
    <div className={`map${noBase ? " map-no-base" : ""}`}>
      <div
        ref={el}
        className="leaflet-map"
        role="application"
        aria-label="Mapa da rota colorida por velocidade"
      />
      {noBase ? <span className="map-notice">Mapa-base indisponível</span> : null}
    </div>
  );
}
