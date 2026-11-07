import { useMutation, useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { deleteTrip } from "@/datasources/tripDatasource";
import type { TripListResponse } from "@/domain/trip/types";

type Pages = InfiniteData<TripListResponse, string | undefined>;

/** The cached list without the trip, and one fewer in the total; untouched if it was not there. */
function without(data: Pages | undefined, id: string): Pages | undefined {
  if (!data || !data.pages.some((p) => p.viagens.some((v) => v.viagem_id === id))) return data;
  return {
    ...data,
    pages: data.pages.map((p) => ({
      ...p,
      viagens: p.viagens.filter((v) => v.viagem_id !== id),
      total: Math.max(0, p.total - 1),
    })),
  };
}

/**
 * Deletes a trip by id. Never repeats by itself. Before the request, the polling of that trip is
 * cancelled, so a poll in flight cannot meet the `404` of the deletion. On success the trip leaves
 * the status and analysis caches and every cached list at once, and the lists are refreshed.
 */
export function useDeleteTrip() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: deleteTrip,
    retry: false,
    networkMode: "always",
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: ["trip", id] });
    },
    onSuccess: async (_, id) => {
      await queryClient.cancelQueries({ queryKey: ["trip", id] });
      queryClient.removeQueries({ queryKey: ["trip", id] });
      queryClient.setQueriesData<Pages>({ queryKey: ["trips"] }, (data) => without(data, id));
      void queryClient.invalidateQueries({ queryKey: ["trips"] });
    },
  });
}
