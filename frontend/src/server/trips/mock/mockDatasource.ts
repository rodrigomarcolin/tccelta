import { randomUUID } from "node:crypto";
import type {
  TripAnalysis,
  FuelId,
  CreateTripResponse,
  TripInput,
  ApiResponse,
  TripStatusResponse,
  TripStatus,
} from "@/domain/trip/types";
import { INITIAL_DONGLE } from "@/config";
import type { TripsDatasource } from "../datasource";
import { buildAnalysis } from "./analyses";
import {
  AWAITING_UNTIL_MS,
  PROVISIONAL_SCENARIOS,
  scenarioForInput,
  EXAMPLES,
  PENDING_UNTIL_MS,
  PROVISIONAL_MS,
  type ScenarioId,
} from "./scenarios";
import { buildProblem } from "@/domain/trip/problem";
import { normalizeName, validateCreation } from "./validation";

type StoredTrip = {
  id: string;
  driverName: string;
  category: string;
  fuel: FuelId;
  input: TripInput;
  scenario: ScenarioId;
  createdAt: number;
  /** Examples are born ready; the provisional phase counts from the first query. */
  isExample: boolean;
  firstQuery: number | null;
};

type Store = Map<string, StoredTrip>;

const STORE_KEY = "__tccelta_trips_mock__";

function getStore(now: number): Store {
  const g = globalThis as unknown as Record<string, Store | undefined>;
  let trips = g[STORE_KEY];
  if (!trips) {
    trips = new Map();
    for (const ex of EXAMPLES) {
      trips.set(ex.id, {
        id: ex.id,
        driverName: `Exemplo — ${ex.label}`,
        category: "exemplo",
        fuel: "gasolina_e30",
        input: ex.input,
        scenario: ex.scenario,
        createdAt: now - 60_000,
        isExample: true,
        firstQuery: null,
      });
    }
    g[STORE_KEY] = trips;
  }
  return trips;
}

/** Clears the store (for tests). */
export function resetMock(): void {
  delete (globalThis as unknown as Record<string, unknown>)[STORE_KEY];
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function createMockDatasource(now: () => number = Date.now): TripsDatasource {
  const lookup = (id: string): StoredTrip | undefined => {
    if (!UUID.test(id)) return undefined;
    return getStore(now()).get(id.toLowerCase());
  };

  function stateOf(r: StoredTrip): { status: TripStatus; provisional: boolean; complete: boolean } {
    const nowMs = now();
    if (r.isExample) r.firstQuery ??= nowMs;
    const base = r.isExample ? (r.firstQuery as number) : r.createdAt;
    const elapsed = nowMs - r.createdAt;
    let status: TripStatus = "pronta";
    if (!r.isExample) {
      if (elapsed < PENDING_UNTIL_MS) status = "pendente";
      else if (elapsed < AWAITING_UNTIL_MS) status = "aguardando_dados";
    }
    const complete = nowMs - base >= PROVISIONAL_MS;
    const provisional =
      status === "pronta" && PROVISIONAL_SCENARIOS.includes(r.scenario) && !complete;
    return { status, provisional, complete };
  }

  return {
    async create(body: unknown): Promise<ApiResponse<CreateTripResponse>> {
      const v = validateCreation(body);
      if (!v.ok) return { status: 400, body: v.problem };
      const { motorista, categoria_viagem, combustivel, entrada } = v.value;
      if (entrada.dongle_id !== INITIAL_DONGLE) {
        return {
          status: 404,
          body: buildProblem(404, "dongle_desconhecido", "DevId sem cadastro"),
        };
      }
      const trips = getStore(now());
      const normalized = normalizeName(motorista.nome);
      for (const r of trips.values()) {
        if (r.category === categoria_viagem && normalizeName(r.driverName) === normalized) {
          return {
            status: 409,
            body: buildProblem(409, "motorista_repetido", "nome já usado na categoria"),
          };
        }
      }
      const id = randomUUID();
      trips.set(id, {
        id,
        driverName: motorista.nome,
        category: categoria_viagem,
        fuel: combustivel,
        input: entrada,
        scenario: scenarioForInput(entrada),
        createdAt: now(),
        isExample: false,
        firstQuery: null,
      });
      return { status: 202, body: { viagem_id: id, status: "pendente" } };
    },

    async status(id: string): Promise<ApiResponse<TripStatusResponse>> {
      const r = lookup(id);
      if (!r) return { status: 404, body: buildProblem(404, "viagem_desconhecida") };
      const e = stateOf(r);
      return {
        status: 200,
        body: {
          viagem_id: r.id,
          status: e.status,
          provisoria: e.provisional,
          motorista: { nome: r.driverName },
          categoria_viagem: r.category,
          combustivel: r.fuel,
          entrada: r.input,
          criada_em: new Date(r.createdAt).toISOString(),
          // Changes when the provisional trip gets the complete version (as in the API).
          calculada_em:
            e.status === "pronta"
              ? new Date(
                  r.createdAt +
                    AWAITING_UNTIL_MS +
                    (e.complete && PROVISIONAL_SCENARIOS.includes(r.scenario) ? PROVISIONAL_MS : 0),
                ).toISOString()
              : null,
        },
      };
    },

    async analysis(id: string): Promise<ApiResponse<TripAnalysis>> {
      const r = lookup(id);
      if (!r) return { status: 404, body: buildProblem(404, "viagem_desconhecida") };
      const e = stateOf(r);
      if (e.status !== "pronta") {
        return {
          status: 409,
          body: buildProblem(409, "analise_indisponivel", undefined, { viagem_status: e.status }),
        };
      }
      return {
        status: 200,
        body: buildAnalysis(
          r.scenario,
          {
            input: r.input,
            driver: { id: r.id, nome: r.driverName },
            category: r.category,
            fuel: r.fuel,
          },
          e.complete,
        ),
      };
    },
  };
}
