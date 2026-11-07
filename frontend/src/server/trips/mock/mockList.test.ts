// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { Problem, TripListResponse, TripStatusResponse } from "@/domain/trip/types";
import { EXAMPLES } from "./scenarios";
import { createMockDatasource, resetMock } from "./mockDatasource";
import { PREVIEW_MAX_POINTS } from "./routePreview";

let now = 1_000_000;
const datasource = createMockDatasource(() => now);

// Boots 8 to 15 belong to the example trips, which would answer viagem_repetida.
const body = (name = "João", extra: Record<string, unknown> = {}) => ({
  motorista: { nome: name },
  categoria_viagem: "Feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 20 },
  ...extra,
});

beforeEach(() => {
  resetMock();
  now = 1_000_000;
});
afterEach(() => resetMock());

const create = (c: unknown) => datasource.create(c);

describe("viagem_repetida", () => {
  it("answers 409 with the existing trip and its driver, for any driver or category", async () => {
    const first = ((await create(body("João"))).body as { viagem_id: string }).viagem_id;
    const again = await create(body("Outra pessoa", { categoria_viagem: "teste" }));
    expect(again.status).toBe(409);
    expect(again.body).toMatchObject({
      codigo: "viagem_repetida",
      viagem_id: first,
      motorista: { nome: "João" },
    });
  });

  it("comes before motorista_repetido", async () => {
    await create(body("João"));
    expect(((await create(body("João"))).body as Problem).codigo).toBe("viagem_repetida");
  });

  it("an absent generation is 0, and another generation is another trip", async () => {
    const entrada = (generation?: number | null) => ({
      tipo: "boot",
      dongle_id: 1,
      boot_id: 30,
      ...(generation === undefined ? {} : { boot_generation: generation }),
    });
    expect((await create(body("A", { entrada: entrada() }))).status).toBe(202);
    expect(((await create(body("B", { entrada: entrada(0) }))).body as Problem).codigo).toBe(
      "viagem_repetida",
    );
    expect(((await create(body("C", { entrada: entrada(null) }))).body as Problem).codigo).toBe(
      "viagem_repetida",
    );
    expect((await create(body("D", { entrada: entrada(1) }))).status).toBe(202);
  });

  it("an example trip is also a trip that exists", async () => {
    const r = await create(body("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 } }));
    expect(r.body).toMatchObject({ codigo: "viagem_repetida", viagem_id: EXAMPLES[0].id });
  });

  it("an interval collides only on the same instants, compared as instants", async () => {
    const interval = (t0: string, t1: string) => ({ tipo: "intervalo", dongle_id: 1, t0, t1 });
    const first = await create(
      body("I1", { entrada: interval("2026-10-28T09:00:00-03:00", "2026-10-28T19:00:00-03:00") }),
    );
    expect(first.status).toBe(202);
    // The same instants written in UTC.
    const same = await create(
      body("I2", { entrada: interval("2026-10-28T12:00:00Z", "2026-10-28T22:00:00Z") }),
    );
    expect(same.status).toBe(409);
    expect(same.body).toMatchObject({ codigo: "viagem_repetida" });
    // Overlapping but different: allowed (a known limit of the rule).
    const other = await create(
      body("I3", { entrada: interval("2026-10-28T12:00:00Z", "2026-10-28T22:00:01Z") }),
    );
    expect(other.status).toBe(202);
  });
});

describe("list", { timeout: 30_000 }, () => {
  const list = async (query: Parameters<typeof datasource.list>[0] = {}) => {
    const r = await datasource.list(query);
    return { status: r.status, body: r.body as TripListResponse };
  };
  const all = async () => {
    const ids: string[] = [];
    let cursor: string | undefined;
    do {
      const r = await list({ limite: 7, cursor });
      ids.push(...r.body.viagens.map((v) => v.viagem_id));
      cursor = r.body.proximo_cursor ?? undefined;
    } while (cursor);
    return ids;
  };
  const exampleItem = async (scenario: string) =>
    (await list({ limite: 50 })).body.viagens.find(
      (v) => v.viagem_id === EXAMPLES.find((e) => e.scenario === scenario)!.id,
    )!;

  it("has more than one batch of 24 by default", async () => {
    const r = await list();
    expect(r.status).toBe(200);
    expect(r.body.viagens).toHaveLength(24);
    expect(r.body.total).toBeGreaterThan(48);
    expect(r.body.proximo_cursor).toEqual(expect.any(String));
  });

  it("walks the pages without repeating or skipping, newest first, ties broken by id", async () => {
    const ids = await all();
    const total = (await list()).body.total;
    expect(ids).toHaveLength(total);
    expect(new Set(ids).size).toBe(total);

    const items = (await list({ limite: 50 })).body.viagens;
    // The examples share a creation time: the id orders them, descending.
    const examples = items.filter((v) => EXAMPLES.some((e) => e.id === v.viagem_id));
    expect(examples.map((v) => v.viagem_id)).toEqual(
      EXAMPLES.map((e) => e.id)
        .sort()
        .reverse(),
    );
    for (let i = 1; i < items.length; i++) {
      expect(items[i - 1].criada_em >= items[i].criada_em).toBe(true);
    }
  });

  it("the last page has no cursor", async () => {
    const total = (await list()).body.total;
    let page = await list({ limite: 20 });
    let seen = page.body.viagens.length;
    expect(page.body.proximo_cursor).not.toBeNull();
    while (page.body.proximo_cursor) {
      page = await list({ limite: 20, cursor: page.body.proximo_cursor });
      seen += page.body.viagens.length;
    }
    expect(seen).toBe(total);
  });

  it("a trip created between batches does not repeat or shift the next one", async () => {
    const first = await list({ limite: 10 });
    now += 1000;
    await create(body("Nova", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 40 } }));
    const second = await list({ limite: 10, cursor: first.body.proximo_cursor! });
    const seen = new Set(first.body.viagens.map((v) => v.viagem_id));
    expect(second.body.viagens.some((v) => seen.has(v.viagem_id))).toBe(false);
    // The new trip is on top of a fresh listing.
    expect((await list({ limite: 1 })).body.viagens[0].motorista.nome).toBe("Nova");
  });

  it("filters by driver, normalized like the repeated name check", async () => {
    await create(body("João da Silva"));
    const r = await list({ busca: "  JOÃO " });
    expect(r.body.viagens.map((v) => v.motorista.nome)).toEqual(["João da Silva"]);
    expect(r.body.total).toBe(1);
    expect((await list({ busca: "ninguém" })).body).toMatchObject({
      viagens: [],
      total: 0,
      proximo_cursor: null,
    });
  });

  it("fills the item from the analysis of a ready trip", async () => {
    const normal = await exampleItem("normal");
    expect(normal).toMatchObject({
      status: "pronta",
      provisoria: false,
      entrada_tipo: "boot",
      dongle_id: 1,
      boot_id: 8,
      boot_generation: 0,
      categoria_viagem: "exemplo",
    });
    expect(normal.distancia_km).toBeGreaterThan(0);
    expect(normal.duracao_s).toBeGreaterThan(0);
    expect(normal.inicio_em).not.toBeNull();
    expect(normal.fim_em).not.toBeNull();
    expect(normal.rota_resumo).toHaveLength(1);
    const points = normal.rota_resumo!.flat();
    expect(points.length).toBeGreaterThan(2);
    expect(points.length).toBeLessThanOrEqual(PREVIEW_MAX_POINTS);
  });

  it("no GPS has no route; a GPS gap cuts the route into stretches", async () => {
    expect((await exampleItem("sem_gps")).rota_resumo).toBeNull();
    const gaps = (await exampleItem("gps_falhas")).rota_resumo!;
    expect(gaps).toHaveLength(3);
    expect(gaps.flat().length).toBeLessThanOrEqual(PREVIEW_MAX_POINTS);
    expect((await exampleItem("sem_hora")).inicio_em).toBeNull();
    expect(await exampleItem("sem_leituras")).toMatchObject({
      provisoria: true,
      distancia_km: null,
      rota_resumo: null,
    });
  });

  it("every seeded route stays within the point limit", async () => {
    let cursor: string | undefined;
    do {
      const r = await list({ limite: 50, cursor });
      for (const v of r.body.viagens) {
        expect(v.rota_resumo?.flat().length ?? 0).toBeLessThanOrEqual(PREVIEW_MAX_POINTS);
      }
      cursor = r.body.proximo_cursor ?? undefined;
    } while (cursor);
  });

  it("an interval has no boot", async () => {
    expect(await exampleItem("intervalo")).toMatchObject({
      entrada_tipo: "intervalo",
      boot_id: null,
      boot_generation: null,
    });
  });

  it("a new trip is pending, without numbers or route, then ready", async () => {
    await create(body("Nova"));
    const first = async () => (await list({ busca: "nova" })).body.viagens[0];
    expect(await first()).toMatchObject({
      status: "pendente",
      distancia_km: null,
      rota_resumo: null,
    });
    now += 2000;
    expect((await first()).status).toBe("aguardando_dados");
    now += 3000;
    const ready = await first();
    expect(ready.status).toBe("pronta");
    expect(ready.rota_resumo).not.toBeNull();
  });

  it("listing does not start the provisional clock of the examples", async () => {
    await list({ limite: 50 });
    now += 40_000;
    const partial = EXAMPLES.find((e) => e.scenario === "dados_parciais")!;
    // Still provisional on the first status query: the clock starts there.
    const status = (await datasource.status(partial.id)).body as TripStatusResponse;
    expect(status.provisoria).toBe(true);
  });

  it.each([
    ["limite 0", { limite: 0 }],
    ["limite 51", { limite: 51 }],
    ["limite fractional", { limite: 1.5 }],
    ["busca of 61 characters", { busca: "a".repeat(61) }],
    ["malformed cursor", { cursor: "!!!" }],
  ])("400 entrada_invalida: %s", async (_name, query) => {
    const r = await datasource.list(query);
    expect(r.status).toBe(400);
    expect((r.body as unknown as Problem).codigo).toBe("entrada_invalida");
  });
});

describe("list after remove", () => {
  it("the removed trip leaves the list and the total drops, with or without a search", async () => {
    const id = ((await create(body("Zelia Unica"))).body as { viagem_id: string }).viagem_id;
    const before = (await datasource.list({})).body as TripListResponse;
    const found = (await datasource.list({ busca: "zelia" })).body as TripListResponse;
    expect(found.total).toBe(1);

    expect((await datasource.remove(id)).status).toBe(204);
    const after = (await datasource.list({})).body as TripListResponse;
    expect(after.total).toBe(before.total - 1);
    expect(after.viagens.map((v) => v.viagem_id)).not.toContain(id);
    const none = (await datasource.list({ busca: "zelia" })).body as TripListResponse;
    expect(none).toMatchObject({ total: 0, viagens: [], proximo_cursor: null });
  });

  it("the name and the entry are free again", async () => {
    const id = ((await create(body("Zelia"))).body as { viagem_id: string }).viagem_id;
    await datasource.remove(id);
    const again = await create(body("zelia"));
    expect(again.status).toBe(202);
    const found = (await datasource.list({ busca: "zelia" })).body as TripListResponse;
    expect(found.viagens.map((v) => v.viagem_id)).toEqual([
      (again.body as { viagem_id: string }).viagem_id,
    ]);
  });
});
