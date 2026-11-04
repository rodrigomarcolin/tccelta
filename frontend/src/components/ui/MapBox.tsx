import type { ReactNode } from "react";

type Props = { children?: ReactNode; loading?: boolean };

/** Frame of the route map; the loading placeholder has the same size. */
export function MapBox({ children, loading }: Props) {
  return (
    <div
      className="relative h-[420px] overflow-hidden rounded-lg border border-hairline bg-sunken print:h-[300px] print:[&_.leaflet-control-zoom]:hidden"
      aria-busy={loading ? "true" : undefined}
    >
      {children}
    </div>
  );
}
