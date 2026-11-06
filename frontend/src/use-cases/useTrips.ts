import { useEffect, useRef } from "react";
import { keepPreviousData, useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import { LIST_RETRIES, POLL_PROVISIONAL_MS, TRIPS_PAGE_SIZE } from "@/config";
import { listTrips } from "@/datasources/tripDatasource";
import type { TripListResponse } from "@/domain/trip/types";
import { retryDelay, shouldRetry } from "./retryPolicy";

export const tripsKey = (search: string) => ["trips", search] as const;

type Pages = InfiniteData<TripListResponse, string | undefined>;

/** A trip still being processed: the list refreshes until it settles. */
const processing = (data: Pages | undefined): boolean =>
  data?.pages.some((p) =>
    p.viagens.some((v) => v.status === "pendente" || v.status === "aguardando_dados"),
  ) ?? false;

/**
 * The trip list in batches, newest first, filtered by driver (`search`, already trimmed).
 * The previous list stays on screen while a new search loads. While any loaded trip is still
 * `pendente` or `aguardando_dados`, every loaded batch is fetched again each
 * `POLL_PROVISIONAL_MS`; with none, there is no polling.
 */
export function useTrips(search: string) {
  const hasResult = useRef(false);
  const query = useInfiniteQuery({
    queryKey: tripsKey(search),
    queryFn: ({ pageParam, signal }) =>
      listTrips({ limite: TRIPS_PAGE_SIZE, cursor: pageParam, busca: search || undefined }, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.proximo_cursor ?? undefined,
    placeholderData: keepPreviousData,
    // A few attempts, then the failure reaches the screen (a retry button, not an endless wait).
    retry: (failures, error) =>
      failures < LIST_RETRIES && shouldRetry(error, hasResult.current, false),
    retryDelay,
    refetchInterval: (q) => (processing(q.state.data) ? POLL_PROVISIONAL_MS : false),
  });
  useEffect(() => {
    hasResult.current = query.data !== undefined;
  }, [query.data]);
  return query;
}
