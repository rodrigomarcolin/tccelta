"use client";

import { useState } from "react";
import { TILES } from "@/config";
import {
  layoutPreview,
  tileUrl,
  type PreviewLayout,
  type PreviewSize,
  type PreviewTile,
} from "@/domain/trip/preview";
import type { RoutePreviewData, TripStatus } from "@/domain/trip/types";
import { TILE_DARK, TILE_LIGHT } from "@/components/analysis/tileClasses";
import { useTheme } from "@/components/ui/Theme";

// No Leaflet here: the list shows dozens of these, so the map is plain tile images with the
// route drawn over them in SVG. The tile classes come from a Leaflet-free module on purpose.

/** Box of the minimap beside the text: wide on a desktop, narrow on a phone. */
const WIDE: PreviewSize = { width: 220, height: 132 };
const NARROW: PreviewSize = { width: 110, height: 132 };

type Props = {
  route: RoutePreviewData | null;
  status: TripStatus;
};

function Tile({ tile, dark }: { tile: PreviewTile; dark: boolean }) {
  const [failed, setFailed] = useState(false);
  if (failed) return null;
  return (
    // Plain <img>: `next/image` would proxy the OpenStreetMap tiles through the server.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={tileUrl(TILES.light, tile)}
      alt=""
      loading="lazy"
      decoding="async"
      draggable={false}
      width={256}
      height={256}
      onError={() => setFailed(true)}
      className={`absolute max-w-none select-none ${dark ? TILE_DARK : TILE_LIGHT}`}
      style={{
        left: `${tile.left}%`,
        top: `${tile.top}%`,
        width: `${tile.width}%`,
        height: `${tile.height}%`,
      }}
    />
  );
}

function Marker({ at, letter }: { at: { x: number; y: number }; letter: "A" | "B" }) {
  return (
    <g transform={`translate(${at.x} ${at.y})`}>
      <circle r={6.5} className="fill-card stroke-primary" strokeWidth={1.5} />
      <text
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={8}
        fontWeight={700}
        className="fill-primary font-mono"
      >
        {letter}
      </text>
    </g>
  );
}

function Stage({
  layout,
  size,
  dark,
  variant,
  className,
}: {
  layout: PreviewLayout;
  size: PreviewSize;
  dark: boolean;
  variant: "wide" | "narrow";
  className: string;
}) {
  const parked = layout.paths.length === 0;
  return (
    <div
      data-testid={`preview-${variant}`}
      className={`absolute top-0 left-0 h-full w-full overflow-hidden ${className}`}
    >
      {layout.tiles.map((t) => (
        <Tile key={`${t.z}/${t.x}/${t.y}`} tile={t} dark={dark} />
      ))}
      <svg
        viewBox={`0 0 ${size.width} ${size.height}`}
        className="absolute inset-0 h-full w-full"
        focusable="false"
      >
        {layout.paths.map((d) => (
          <path
            key={`o${d}`}
            d={d}
            fill="none"
            strokeWidth={5.5}
            strokeLinecap="round"
            strokeLinejoin="round"
            className="stroke-route-outline"
          />
        ))}
        {layout.paths.map((d) => (
          <path
            key={`l${d}`}
            d={d}
            fill="none"
            strokeWidth={3}
            strokeLinecap="round"
            strokeLinejoin="round"
            className="stroke-accent"
          />
        ))}
        {layout.dots.map((p, i) => (
          <circle
            key={`d${i}`}
            cx={p.x}
            cy={p.y}
            r={4}
            strokeWidth={2}
            className="fill-accent stroke-route-outline"
          />
        ))}
        {parked ? null : (
          <>
            <Marker at={layout.start} letter="A" />
            <Marker at={layout.end} letter="B" />
          </>
        )}
      </svg>
    </div>
  );
}

function Message({ children, pulse }: { children: string; pulse?: boolean }) {
  return (
    <span
      className={`absolute inset-0 grid place-items-center px-2 text-center text-[12px] text-secondary ${
        pulse ? "animate-breathe motion-reduce:animate-none" : ""
      }`}
    >
      {children}
    </span>
  );
}

/**
 * Minimap of a trip for the list: map tiles under the route. Decorative (`aria-hidden`), since
 * the card text already describes the trip. Without a route it says why: processing, failed or
 * no GPS. There are two layouts, one per box width, and CSS shows the one that fits.
 */
export function RoutePreview({ route, status }: Props) {
  const dark = useTheme() === "dark";
  const wide = layoutPreview(route, WIDE);
  const narrow = layoutPreview(route, NARROW);

  let content;
  if (status === "pendente" || status === "aguardando_dados") {
    content = <Message pulse>Processando</Message>;
  } else if (status === "falhou") {
    content = <Message>Falhou</Message>;
  } else if (!wide || !narrow) {
    content = <Message>Sem GPS</Message>;
  } else {
    content = (
      <>
        <Stage layout={wide} size={WIDE} dark={dark} variant="wide" className="phone:hidden" />
        <Stage
          layout={narrow}
          size={NARROW}
          dark={dark}
          variant="narrow"
          className="hidden phone:block"
        />
      </>
    );
  }
  return (
    <div aria-hidden="true" className="relative h-full w-full overflow-hidden bg-sunken">
      {content}
    </div>
  );
}
