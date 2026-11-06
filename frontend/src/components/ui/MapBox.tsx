import type { ReactNode } from "react";

type Props = { children?: ReactNode; loading?: boolean; compact?: boolean };

/** Frame of the route map; the loading placeholder has the same size. */
export function MapBox({ children, loading, compact }: Props) {
  return (
    <div
      className={`relative overflow-hidden rounded-lg border border-hairline bg-sunken print:h-[300px] print:[&_.leaflet-control-zoom]:hidden ${
        compact
          ? "h-full [&_.leaflet-control-attribution]:pointer-events-none [&_.leaflet-control-attribution]:text-[9px]"
          : "h-[420px]"
      }`}
      aria-busy={loading ? "true" : undefined}
    >
      {children}
    </div>
  );
}
