import { useRef, type RefObject } from "react";
import { useQuery } from "@tanstack/react-query";
import { getTripStatus } from "@/datasources/tripDatasource";
import { retryDelay, shouldRetry } from "./retryPolicy";

export const tripStatusKey = (id: string) => ["trip", id, "status"] as const;

/**
 * Status of the trip. A failure streak (network, 5xx, 429, held attempts) retries inside the
 * same fetch, so `failureCount` counts the streak. When the next poll happens is decided by
 * `useTripMonitor`, only after each fetch ends, so slow responses never overlap.
 * `hasAnalysis` tells that an analysis is on screen, which turns unexpected responses into retries.
 */
export function useTripStatus(id: string, hasAnalysis?: RefObject<boolean>) {
  const own = useRef(false);
  const shown = hasAnalysis ?? own;
  return useQuery({
    queryKey: tripStatusKey(id),
    queryFn: ({ signal }) => getTripStatus(id, signal),
    retry: (_failures, error) => shouldRetry(error, shown.current),
    retryDelay,
  });
}
