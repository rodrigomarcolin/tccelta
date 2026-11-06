import type { ReactNode } from "react";

/** Title of a chart or card, with its unit in `Unit` next to it. */
export function ChartTitle({ children }: { children: ReactNode }) {
  return <h3 className="mb-2 type-label">{children}</h3>;
}
