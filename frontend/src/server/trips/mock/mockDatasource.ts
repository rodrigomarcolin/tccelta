import { randomUUID } from "node:crypto";
import type {
  TripAnalysis,
  FuelId,
  CreateTripResponse,
  TripInput,
  ApiResponse,
  TripStatusResponse,
  TripStatus,
  ListQuery,
  TripListItem,
  TripListResponse,
} from "@/domain/trip/types";
import { INITIAL_DONGLE } from "@/config";
import type { TripsDatasource } from "../datasource";
import { buildAnalysis } from "./analyses";
import {
  AWAITING_UNTIL_MS,
  PROVISIONAL_SCENARIOS,
  scenarioForInput,
  EXAMPLES,
  VARIANTS,
  PENDING_UNTIL_MS,
  PROVISIONAL_MS,
  type ScenarioId,
} from "./scenarios";
import { buildProblem } from "@/domain/trip/problem";
import { normalizeName, validateCreation } from "./validation";
import { previewOf } from "./routePreview";

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
  /** Turns the route in the list summary (seeded entries only). */
  rotationDeg: number;
  /** What the list takes from the analysis, by `complete` (building it is the costly part). */
  summaries?: Map<boolean, ListSummary>;
};

type ListSummary = Pick<
  TripListItem,
  "inicio_em" | "fim_em" | "distancia_km" | "duracao_s" | "rota_resumo"
>;

const DEFAULT_PAGE = 24;
const MAX_PAGE = 50;
const MAX_SEARCH_RUNES = 60;
/** Distance in creation time between the seeded entries. */
const SEEDED_STEP_MS = 20 * 60_000;

/** Opaque cursor: creation time and id of the last item of the page. */
const encodeCursor = (r: StoredTrip) => Buffer.from(`${r.createdAt}|${r.id}`).toString("base64url");

function decodeCursor(cursor: string): { createdAt: number; id: string } | null {
  const [created, id, ...rest] = Buffer.from(cursor, "base64url").toString().split("|");
  const createdAt = Number(created);
  if (rest.length || !id || !Number.isInteger(createdAt)) return null;
  return { createdAt, id };
}

/** Same key the API uses: dongle + generation + boot, or dongle + the exact instants. */
function sameTrip(a: TripInput, b: TripInput): boolean {
  if (a.dongle_id !== b.dongle_id) return false;
  if (a.tipo === "boot" && b.tipo === "boot") {
    return a.boot_id === b.boot_id && (a.boot_generation ?? 0) === (b.boot_generation ?? 0);
  }
  if (a.tipo === "intervalo" && b.tipo === "intervalo") {
    return Date.parse(a.t0) === Date.parse(b.t0) && Date.parse(a.t1) === Date.parse(b.t1);
  }
  return false;
}

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
        rotationDeg: 0,
      });
    }
    // Extra entries so the list passes one batch; older than the examples, in a stable order.
    for (const [i, v] of VARIANTS.entries()) {
      trips.set(v.id, {
        id: v.id,
        driverName: v.driver,
        category: "feira",
        fuel: "gasolina_e30",
        input: v.input,
        scenario: v.scenario,
        createdAt: now - 2 * 60_000 - i * SEEDED_STEP_MS,
        isExample: true,
        firstQuery: null,
        rotationDeg: v.rotationDeg,
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

  /** `touch: false` (the list) does not start the provisional clock of an example. */
  function stateOf(
    r: StoredTrip,
    touch = true,
  ): { status: TripStatus; provisional: boolean; complete: boolean } {
    const nowMs = now();
    if (r.isExample && touch) r.firstQuery ??= nowMs;
    const base = r.isExample ? (r.firstQuery ?? nowMs) : r.createdAt;
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
      // The same trip, for any driver or category, comes before the repeated name.
      for (const r of trips.values()) {
        if (sameTrip(r.input, entrada)) {
          return {
            status: 409,
            body: buildProblem(409, "viagem_repetida", "viagem já processada", {
              viagem_id: r.id,
              motorista: { nome: r.driverName },
            }),
          };
        }
      }
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
        rotationDeg: 0,
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

    async list(query: ListQuery): Promise<ApiResponse<TripListResponse>> {
      const invalid = (detail: string) => ({
        status: 400,
        body: buildProblem(400, "entrada_invalida", detail),
      });
      const limit = query.limite ?? DEFAULT_PAGE;
      if (!Number.isInteger(limit) || limit < 1 || limit > MAX_PAGE) {
        return invalid(`limite: inteiro de 1 a ${MAX_PAGE}`);
      }
      if (query.busca !== undefined && [...query.busca].length > MAX_SEARCH_RUNES) {
        return invalid(`busca: no máximo ${MAX_SEARCH_RUNES} caracteres`);
      }
      const after = query.cursor ? decodeCursor(query.cursor) : null;
      if (query.cursor && !after) return invalid("cursor inválido");

      const search = query.busca ? normalizeName(query.busca) : "";
      const matching = [...getStore(now()).values()]
        .filter((r) => !search || normalizeName(r.driverName).includes(search))
        // Newest first; the id breaks ties (the examples share a creation time).
        .sort((a, b) => b.createdAt - a.createdAt || (a.id < b.id ? 1 : a.id > b.id ? -1 : 0));
      const rest = after
        ? matching.filter(
            (r) =>
              r.createdAt < after.createdAt || (r.createdAt === after.createdAt && r.id < after.id),
          )
        : matching;
      const page = rest.slice(0, limit);
      return {
        status: 200,
        body: {
          viagens: page.map(listItem),
          total: matching.length,
          proximo_cursor: rest.length > limit ? encodeCursor(page[page.length - 1]) : null,
        },
      };
    },
  };

  function listItem(r: StoredTrip): TripListItem {
    const e = stateOf(r, false);
    const base: TripListItem = {
      viagem_id: r.id,
      status: e.status,
      provisoria: e.provisional,
      motorista: { nome: r.driverName },
      categoria_viagem: r.category,
      entrada_tipo: r.input.tipo,
      dongle_id: r.input.dongle_id,
      boot_id: r.input.tipo === "boot" ? r.input.boot_id : null,
      boot_generation: r.input.tipo === "boot" ? (r.input.boot_generation ?? 0) : null,
      inicio_em: r.input.tipo === "intervalo" ? r.input.t0 : null,
      fim_em: r.input.tipo === "intervalo" ? r.input.t1 : null,
      distancia_km: null,
      duracao_s: null,
      criada_em: new Date(r.createdAt).toISOString(),
      rota_resumo: null,
    };
    if (e.status !== "pronta") return base;
    r.summaries ??= new Map();
    let summary = r.summaries.get(e.complete);
    if (!summary) {
      const a = buildAnalysis(
        r.scenario,
        {
          input: r.input,
          driver: { id: r.id, nome: r.driverName },
          category: r.category,
          fuel: r.fuel,
        },
        e.complete,
      );
      summary = {
        inicio_em: a.viagem.inicio_em ?? base.inicio_em,
        fim_em: a.viagem.fim_em ?? base.fim_em,
        distancia_km: a.resumo.distancia_km,
        duracao_s: a.resumo.duracao_s,
        rota_resumo: previewOf(a.rota, r.rotationDeg),
      };
      r.summaries.set(e.complete, summary);
    }
    return { ...base, ...summary };
  }
}
