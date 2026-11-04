import { useEffect, useRef, type RefObject } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { getTripAnalysis } from "@/datasources/tripDatasource";
import { ApiError } from "@/domain/trip/apiError";
import { retryDelay, shouldRetry } from "./retryPolicy";

export const tripAnalysisKey = (id: string, calculatedAt: string | null) =>
  ["trip", id, "analysis", calculatedAt] as const;

/**
 * Analysis of the trip, one entry per `calculada_em`: when the version changes, the previous
 * one stays on screen (only for the same trip) until the new one arrives.
 * A 409 (not ready yet) tries again after 1 s.
 */
export function useTripAnalysis(
  id: string,
  calculatedAt: string | null,
  enabled: boolean,
  hasAnalysis: RefObject<boolean>,
) {
  const enabledRef = useRef(enabled);
  useEffect(() => {
    enabledRef.current = enabled;
  });
  return useQuery({
    queryKey: tripAnalysisKey(id, calculatedAt),
    queryFn: ({ signal }) => getTripAnalysis(id, signal),
    enabled,
    staleTime: Infinity,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === id ? keepPreviousData(previous) : undefined,
    retry: (_failures, error) =>
      enabledRef.current &&
      ((error instanceof ApiError && error.status === 409) ||
        shouldRetry(error, hasAnalysis.current, false)),
    retryDelay,
  });
}
