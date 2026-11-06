"use client";

import { useState } from "react";
import type { Route } from "@/domain/trip/types";
import type { Theme } from "@/config";
import { RouteMap } from "./RouteMap";

const NO_EVENTS: never[] = [];
const NO_STOPS: never[] = [];
const NOOP = () => {};
const FORMAT_T = (t: number) => String(t);

type Props = {
  route: Route;
  theme: Theme;
  cursorT: number | null;
  /** The real map left the window. */
  visible: boolean;
  /** Bottom of the sticky header, in px. */
  headerBottom: number;
};

/**
 * Small map fixed at the top right, so the chart hover can be followed while the main map is
 * off screen. Mounted on the first need, then only hidden (avoids reloading tiles).
 */
export function Minimap({ route, theme, cursorT, visible, headerBottom }: Props) {
  const [mounted, setMounted] = useState(visible);
  if (visible && !mounted) setMounted(true);
  if (!mounted) return null;
  return (
    <div
      aria-hidden="true"
      className={`pointer-events-none fixed right-4 z-20 h-[180px] overflow-hidden rounded-lg w-[260px] phone:h-[120px] phone:w-[170px] print:hidden ${
        visible ? "" : "invisible"
      }`}
      style={{ top: headerBottom + 12 }}
    >
      <RouteMap
        compact
        route={route}
        events={NO_EVENTS}
        stops={NO_STOPS}
        theme={theme}
        cursorT={cursorT}
        onHoverT={NOOP}
        focus={null}
        formatT={FORMAT_T}
      />
    </div>
  );
}
