"use client";

import { useEffect, useState, type RefObject } from "react";

/** The map counts as off screen when less than this fraction of its height is visible. */
const MIN_VISIBLE = 0.25;

type State = { offscreen: boolean; headerBottom: number };

/**
 * Tells whether the map has left the window (below the sticky header).
 * With all rects at zero (jsdom) it returns `offscreen: false`.
 */
export function useMapOffscreen(
  map: RefObject<HTMLElement | null>,
  header: RefObject<HTMLElement | null>,
): State {
  const [state, setState] = useState<State>({ offscreen: false, headerBottom: 0 });

  useEffect(() => {
    let frame = 0;
    const measure = () => {
      frame = 0;
      const mapEl = map.current;
      if (!mapEl) return;
      const m = mapEl.getBoundingClientRect();
      const headerBottom = Math.round(header.current?.getBoundingClientRect().bottom ?? 0);
      const top = Math.max(m.top, headerBottom);
      const bottom = Math.min(m.bottom, window.innerHeight);
      const visible = Math.max(0, bottom - top);
      const offscreen = visible < m.height * MIN_VISIBLE;
      setState((prev) =>
        prev.offscreen === offscreen && prev.headerBottom === headerBottom
          ? prev
          : { offscreen, headerBottom },
      );
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(measure);
    };
    measure();
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule, { passive: true });
    return () => {
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      if (frame) cancelAnimationFrame(frame);
    };
  }, [map, header]);

  return state;
}
