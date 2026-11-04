"use client";

import dynamic from "next/dynamic";

/** uPlot on the client only: the module touches `window` on load. */
export const Chart = dynamic(() => import("./UPlotChart"), {
  ssr: false,
  loading: () => <div className="chart chart-loading" aria-busy="true" />,
});
