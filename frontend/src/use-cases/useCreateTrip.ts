import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createTrip } from "@/datasources/tripDatasource";
import type {
  CreateTripRequest,
  CreateTripResponse,
  FuelId,
  TripStatusResponse,
} from "@/domain/trip/types";
import { tripStatusKey } from "./useTripStatus";

/** The status the trip page shows until its first poll answers, from what the `POST` just said. */
function initialStatus(req: CreateTripRequest, res: CreateTripResponse): TripStatusResponse {
  return {
    viagem_id: res.viagem_id,
    status: res.status,
    provisoria: false,
    motorista: req.motorista,
    categoria_viagem: req.categoria_viagem,
    combustivel: req.combustivel as FuelId,
    entrada: req.entrada,
    criada_em: new Date().toISOString(),
    calculada_em: null,
  };
}

/**
 * The `POST` never repeats by itself: no retry, even on a network failure.
 * A trip that is still being processed seeds the status cache, so the trip page opens on
 * "processing" instead of a skeleton; its first poll then replaces the seed.
 */
export function useCreateTrip() {
  const queryClient = useQueryClient();
  return useMutation<CreateTripResponse, Error, CreateTripRequest>({
    mutationFn: createTrip,
    retry: false,
    networkMode: "always",
    onSuccess: (res, req) => {
      // The list has a new trip at the top.
      void queryClient.invalidateQueries({ queryKey: ["trips"] });
      if (res.status === "pendente" || res.status === "aguardando_dados") {
        queryClient.setQueryData(tripStatusKey(res.viagem_id), initialStatus(req, res));
      }
    },
  });
}
