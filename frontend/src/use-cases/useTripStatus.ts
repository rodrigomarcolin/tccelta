import type { RefObject } from "react";
import { useQuery } from "@tanstack/react-query";
import { getTripStatus } from "@/datasources/tripDatasource";
import { retryDelay, shouldRetry } from "./retryPolicy";

export const tripStatusKey = (id: string) => ["trip", id, "status"] as const;

/**
 * Status of the trip, polled every `refetchInterval` ms after each response (`false`: stopped;
 * `useTripMonitor` decides it from the last response). A failure streak (network, 5xx, 429,
 * held attempts) retries inside the same fetch, so `failureCount` counts the streak.
 * `hasAnalysis` tells that an analysis is on screen, which turns unexpected responses into retries.
 */
export function useTripStatus(
  id: string,
  hasAnalysis: RefObject<boolean>,
  refetchInterval: number | false,
) {
  return useQuery({
    queryKey: tripStatusKey(id),
    queryFn: ({ signal }) => getTripStatus(id, signal),
    // A 404 on the status route is the end: the trip does not exist.
    retry: (_failures, error) => shouldRetry(error, hasAnalysis.current, true),
    retryDelay,
    refetchInterval,
  });
}
