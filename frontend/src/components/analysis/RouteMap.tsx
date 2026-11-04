"use client";

import dynamic from "next/dynamic";
import { MapBox } from "@/components/ui/MapBox";

/** Leaflet on the client only (`ssr: false`). */
export const RouteMap = dynamic(() => import("./LeafletRouteMap"), {
  ssr: false,
  loading: () => <MapBox loading />,
});
