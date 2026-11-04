"use client";

import dynamic from "next/dynamic";

/** Leaflet on the client only (`ssr: false`). */
export const RouteMap = dynamic(() => import("./LeafletRouteMap"), {
  ssr: false,
  loading: () => <div className="map map-loading" aria-busy="true" />,
});
