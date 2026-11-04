"use client";

import dynamic from "next/dynamic";

/** Leaflet só no cliente (`ssr: false`). */
export const Mapa = dynamic(() => import("./MapaRotaLeaflet"), {
  ssr: false,
  loading: () => <div className="mapa mapa-carregando" aria-busy="true" />,
});
