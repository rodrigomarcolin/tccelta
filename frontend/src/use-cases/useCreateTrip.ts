import { useMutation } from "@tanstack/react-query";
import { createTrip } from "@/datasources/tripDatasource";
import type { CreateTripRequest, CreateTripResponse } from "@/domain/trip/types";

/** The `POST` never repeats by itself: no retry, even on a network failure. */
export function useCreateTrip() {
  return useMutation<CreateTripResponse, Error, CreateTripRequest>({
    mutationFn: createTrip,
    retry: false,
    networkMode: "always",
  });
}
