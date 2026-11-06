import type { ChartProps } from "@/components/analysis/UPlotChart";
import type { MapProps } from "@/components/analysis/LeafletRouteMap";

/**
 * Doubles of Chart and RouteMap for jsdom (no canvas or Leaflet).
 * Each one exposes the data it received and buttons that simulate the mouse.
 */
export function ChartDouble(p: ChartProps) {
  if (p.noReadings) return <div data-testid="chart-empty">{p.title}: Sem leituras</div>;
  const middle = p.x[Math.floor(p.x.length / 2)];
  return (
    <div
      data-testid="chart"
      data-title={p.title}
      data-cursor-x={p.cursorX ?? ""}
      data-apply={String(p.applyCursor)}
      data-markers={p.markers?.length ?? 0}
      data-pauses={p.pauses?.length ?? 0}
      data-reference={p.referenceY?.label ?? ""}
      data-series={p.series.length}
    >
      <button type="button" onClick={() => p.onCursorX?.(middle)}>
        {`mover ${p.title}`}
      </button>
      <button type="button" onClick={() => p.onCursorX?.(null)}>
        {`sair ${p.title}`}
      </button>
    </div>
  );
}

export function MapDouble(p: MapProps) {
  return (
    <div
      data-testid="map"
      data-cursor-t={p.cursorT ?? ""}
      data-focus={p.focus ? `${p.focus.lat},${p.focus.lon}` : ""}
      data-theme={p.theme}
      data-events={p.events.length}
      data-compact={String(p.compact ?? false)}
    >
      <button type="button" onClick={() => p.onHoverT(p.route.t_ms[10])}>
        hover na rota
      </button>
      <button type="button" onClick={() => p.onHoverT(null)}>
        sair da rota
      </button>
    </div>
  );
}
