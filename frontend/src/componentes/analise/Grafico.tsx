"use client";

import dynamic from "next/dynamic";

/** uPlot só no cliente: o módulo acessa `window` ao carregar. */
export const Grafico = dynamic(() => import("./GraficoUPlot"), {
  ssr: false,
  loading: () => <div className="grafico grafico-carregando" aria-busy="true" />,
});
