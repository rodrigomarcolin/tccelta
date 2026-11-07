"use client";

import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import type { TripAnalysis, TripEvent } from "@/domain/trip/types";
import {
  analysisAxis,
  timeFormatter,
  fullTimeFormatter,
  nearestInstant,
  hasReadings,
  seriesTimes,
} from "@/domain/trip/analysis";
import { useTheme } from "@/components/ui/Theme";
import { Note } from "@/components/ui/Note";
import { Header } from "./Header";
import { FuelEngine } from "./FuelEngine";
import { AwaitingDataState, NoDataState, ConnectionBanner, ProvisionalBanner } from "./States";
import { NoticeBar } from "./NoticeBar";
import { InstantTooltip } from "./Instant";
import { Timeline, type Cursor } from "./Timeline";
import { RouteMap } from "./RouteMap";
import { Minimap } from "./Minimap";
import { useMapOffscreen } from "./useMapOffscreen";
import { GearsRpm } from "./GearsRpm";
import { Quality } from "./Quality";
import { Legend, Summary } from "./Summary";
import { EventsTable } from "./EventsTable";

type Props = {
  analysis: TripAnalysis;
  provisional: boolean;
  /** The time to follow the provisional trip has run out. */
  provisionalExpired?: boolean;
  /** Connection or busy-server notice. */
  notice?: string | null;
  /** Time (HH:MM) of the latest version that replaced the screen. */
  updatedAt?: string | null;
  /** Page actions, shown in the header. */
  actions?: ReactNode;
};

const SECTION = "mb-9 print:break-inside-avoid";
const SUMMARY = "mb-9 grid gap-7 print:mb-[8mm] print:break-inside-avoid";

/** Explains the event markers of the map (the shapes alone do not say which is which). */
function EventSymbols() {
  return (
    <p className="m-0 text-[12px] text-secondary">
      <span aria-hidden="true">●</span> Frenagem brusca · <span aria-hidden="true">○</span>{" "}
      Aceleração brusca
    </p>
  );
}

export function AnalysisView({
  analysis,
  provisional,
  provisionalExpired = false,
  notice = null,
  updatedAt = null,
  actions,
}: Props) {
  const theme = useTheme();
  const [cursor, setCursorState] = useState<Cursor>({ t: null, source: "chart" });
  const [focus, setFocus] = useState<{ lat: number; lon: number; nonce: number } | null>(null);
  const mapFrameRef = useRef<HTMLDivElement>(null);
  const headerRef = useRef<HTMLElement>(null);
  const { offscreen, headerBottom } = useMapOffscreen(mapFrameRef, headerRef);

  const axis = useMemo(() => analysisAxis(analysis), [analysis]);
  const fmtAxis = useMemo(() => timeFormatter(analysis), [analysis]);
  const fmtFull = useMemo(() => fullTimeFormatter(analysis), [analysis]);
  const formatX = useCallback((x: number) => fmtAxis(axis.fromX(x)), [fmtAxis, axis]);
  const times = useMemo(() => seriesTimes(analysis), [analysis]);

  const setCursor = useCallback((t: number | null, source: Cursor["source"]) => {
    setCursorState((prev) => {
      if (prev.t === t && prev.source === source) return prev;
      // Leaving the chart or the map does not clear a deliberately pinned instant.
      if (t === null && prev.source === "pinned") return prev;
      return { t, source };
    });
  }, []);

  const viewOnMap = useCallback((e: TripEvent) => {
    if (e.lat === null || e.lon === null) return;
    setFocus({ lat: e.lat, lon: e.lon, nonce: Date.now() });
    setCursorState({ t: e.t_inicio_ms, source: "pinned" });
    // Lands just below the sticky header (see `scrollMarginTop` on the frame).
    mapFrameRef.current?.scrollIntoView?.({ block: "start", behavior: "smooth" });
  }, []);

  const markInstant = useCallback((t: number) => setCursorState({ t, source: "pinned" }), []);

  const empty = !hasReadings(analysis);
  const instant = cursor.t === null ? null : nearestInstant(analysis, times, cursor.t);
  const route = analysis.rota;
  const hasMap = route !== null && route.lat.some((v) => v !== null);

  return (
    <div>
      <Header analysis={analysis} headerRef={headerRef} actions={actions} />
      {notice ? <ConnectionBanner text={notice} /> : null}
      {provisional ? <ProvisionalBanner expired={provisionalExpired} /> : null}
      {updatedAt ? <Note role="status">Resultado atualizado às {updatedAt}</Note> : null}
      <NoticeBar warnings={analysis.avisos} />

      {empty ? (
        provisional ? (
          <AwaitingDataState />
        ) : (
          <NoDataState />
        )
      ) : (
        <>
          <section
            className={`${SUMMARY} ${
              hasMap
                ? "grid-cols-[minmax(0,7fr)_minmax(0,5fr)] tablet:grid-cols-[minmax(0,1fr)]"
                : "grid-cols-[minmax(0,1fr)]"
            }`}
            aria-labelledby="t-summary"
          >
            <h2 id="t-summary" className="sr-only">
              Resumo
            </h2>
            {hasMap && route ? (
              <div className="grid content-start gap-4">
                <div
                  className="relative isolate"
                  ref={mapFrameRef}
                  style={{ scrollMarginTop: headerBottom + 12 }}
                >
                  <RouteMap
                    route={route}
                    events={analysis.eventos}
                    stops={analysis.paradas}
                    theme={theme}
                    cursorT={cursor.t}
                    onHoverT={(t) => setCursor(t, "map")}
                    focus={focus}
                    formatT={fmtFull}
                  />
                  <InstantTooltip
                    className="pointer-events-none absolute top-2 left-14 z-[500]"
                    hideEmpty
                    instant={cursor.source === "map" ? instant : null}
                    formatT={fmtFull}
                  />
                </div>
                <EventSymbols />
                <Legend analysis={analysis} />
              </div>
            ) : null}
            <div>
              <Summary analysis={analysis} />
              {!hasMap ? <Legend analysis={analysis} /> : null}
            </div>
          </section>

          {hasMap && route ? (
            <Minimap
              route={route}
              theme={theme}
              cursorT={cursor.t}
              visible={offscreen}
              headerBottom={headerBottom}
            />
          ) : null}

          <section className={SECTION} aria-labelledby="t-timeline">
            <h2 id="t-timeline">Linha do tempo</h2>
            <Timeline
              analysis={analysis}
              axis={axis}
              formatX={formatX}
              cursor={cursor}
              onCursor={setCursor}
            />
          </section>

          <section className={SECTION} aria-labelledby="t-gears">
            <h2 id="t-gears">Marchas e rotação</h2>
            <GearsRpm analysis={analysis} formatT={fmtFull} onMarkInstant={markInstant} />
          </section>

          <section className={SECTION} aria-labelledby="t-fuel">
            <h2 id="t-fuel">Combustível e motor</h2>
            <FuelEngine analysis={analysis} axis={axis} formatX={formatX} />
          </section>

          <section className={SECTION} aria-labelledby="t-events">
            <h2 id="t-events">Eventos</h2>
            <EventsTable events={analysis.eventos} formatT={fmtFull} onViewOnMap={viewOnMap} />
          </section>
        </>
      )}

      <section className={SECTION} aria-label="Qualidade dos dados e metodologia">
        <Quality analysis={analysis} />
      </section>
    </div>
  );
}
