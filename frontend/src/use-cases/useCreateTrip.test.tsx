import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createQueryClient } from "@/app/providers";
import type { CreateTripRequest } from "@/domain/trip/types";
import { useCreateTrip } from "./useCreateTrip";
import { tripStatusKey } from "./useTripStatus";

const REQ: CreateTripRequest = {
  motorista: { nome: "Davi" },
  categoria_viagem: "feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 9 },
};

function stubPost(status: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      status: 202,
      ok: true,
      json: async () => ({ viagem_id: "abc", status }),
      headers: new Headers(),
    })),
  );
}

function setup() {
  const client = createQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, ...renderHook(() => useCreateTrip(), { wrapper }) };
}

afterEach(() => vi.unstubAllGlobals());

describe("useCreateTrip", () => {
  it("seeds the status cache with the pending trip, so the page opens on 'processing'", async () => {
    stubPost("pendente");
    const { client, result } = setup();
    await result.current.mutateAsync(REQ);
    await waitFor(() => expect(client.getQueryData(tripStatusKey("abc"))).toBeDefined());
    expect(client.getQueryData(tripStatusKey("abc"))).toMatchObject({
      viagem_id: "abc",
      status: "pendente",
      motorista: { nome: "Davi" },
      calculada_em: null,
    });
  });

  it("does not seed a trip that is already ready", async () => {
    stubPost("pronta");
    const { client, result } = setup();
    await result.current.mutateAsync(REQ);
    expect(client.getQueryData(tripStatusKey("abc"))).toBeUndefined();
  });
});

describe("useCreateTrip and the list", () => {
  it("invalidates the trip list after creating", async () => {
    stubPost("pendente");
    const { client, result } = setup();
    client.setQueryData(["trips", ""], { pages: [], pageParams: [] });
    await result.current.mutateAsync(REQ);
    await waitFor(() => expect(client.getQueryState(["trips", ""])?.isInvalidated).toBe(true));
  });
});
