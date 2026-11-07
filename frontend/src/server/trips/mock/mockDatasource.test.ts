import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { TripAnalysis, Problem, TripStatusResponse } from "@/domain/trip/types";
import { isProblem } from "@/domain/trip/types";
import { contractViolations } from "@/tests/contract";
import { EXAMPLES } from "./scenarios";
import { createMockDatasource, resetMock } from "./mockDatasource";

let now = 1_000_000;
const datasource = createMockDatasource(() => now);

const body = (name = "João", extra: Record<string, unknown> = {}) => ({
  motorista: { nome: name },
  categoria_viagem: "Feira",
  combustivel: "gasolina_e30",
  // Boots 8 to 15 belong to the example trips, which would answer viagem_repetida.
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 20 },
  ...extra,
});

beforeEach(() => {
  resetMock();
  now = 1_000_000;
});
afterEach(() => resetMock());

async function create(c: unknown) {
  return datasource.create(c);
}

describe("create", () => {
  it("responds 202 with viagem_id and status pendente", async () => {
    const r = await create(body());
    expect(r.status).toBe(202);
    expect(r.body).toMatchObject({ status: "pendente" });
    expect((r.body as { viagem_id: string }).viagem_id).toMatch(/^[0-9a-f-]{36}$/);
  });

  it("stores the category in lowercase and refuses a repeated name, ignoring case and spaces", async () => {
    expect((await create(body("João"))).status).toBe(202);
    // Another boot: the same trip would answer viagem_repetida first.
    const other = { tipo: "boot", dongle_id: 1, boot_id: 21 };
    const r = await create(body("joão", { categoria_viagem: "FEIRA", entrada: other }));
    expect(r.status).toBe(409);
    expect((r.body as Problem).codigo).toBe("motorista_repetido");
    // Another category frees the name.
    const third = { tipo: "boot", dongle_id: 1, boot_id: 22 };
    expect((await create(body("João", { categoria_viagem: "teste", entrada: third }))).status).toBe(
      202,
    );
  });

  it.each([
    ["empty name", body("")],
    ["name with surrounding spaces", body(" Ana ")],
    ["61-character name", body("a".repeat(61))],
    ["name with control character", body("a\u0007b")],
    ["empty category", body("X", { categoria_viagem: "" })],
    ["category with symbol", body("X", { categoria_viagem: "a/b" })],
    ["long category", body("X", { categoria_viagem: "a".repeat(41) })],
    ["unknown fuel", body("X", { combustivel: "personalizado" })],
    ["no entrada", body("X", { entrada: undefined })],
    ["entrada without tipo", body("X", { entrada: { tipo: "outro" } })],
    ["boot out of range", body("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 70000 } })],
    [
      "invalid generation",
      body("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: -1 } }),
    ],
    [
      "boot with interval field",
      body("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, t0: "x" } }),
    ],
    ["dongle 0", body("X", { entrada: { tipo: "boot", dongle_id: 0, boot_id: 8 } })],
    [
      "t0 without timezone",
      body("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T08:00:00",
          t1: "2026-10-28T09:00:00-03:00",
        },
      }),
    ],
    [
      "t1 before t0",
      body("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T09:00:00-03:00",
          t1: "2026-10-28T09:00:00-03:00",
        },
      }),
    ],
    [
      "8-day interval",
      body("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-01T09:00:00-03:00",
          t1: "2026-10-09T09:00:01-03:00",
        },
      }),
    ],
    [
      "impossible t0",
      body("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-13-45T09:00:00-03:00",
          t1: "2026-10-28T09:00:00-03:00",
        },
      }),
    ],
    [
      "interval with boot_id",
      body("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T09:00:00-03:00",
          t1: "2026-10-28T09:00:00-03:00",
          boot_id: 8,
        },
      }),
    ],
  ])("400 entrada_invalida: %s", async (_name, c) => {
    const r = await create(c);
    expect(r.status).toBe(400);
    expect((r.body as Problem).codigo).toBe("entrada_invalida");
    expect((r.body as Problem).detail).toBeTruthy();
  });

  it("400 corpo_malformado for an unknown field and a body that is not an object", async () => {
    for (const c of [{ ...body(), extra: 1 }, "texto", null, []]) {
      const r = await create(c);
      expect(r.status).toBe(400);
      expect((r.body as Problem).codigo).toBe("corpo_malformado");
    }
    const r = await create({ ...body(), motorista: { nome: "A", idade: 3 } });
    expect((r.body as Problem).codigo).toBe("corpo_malformado");
  });

  it("404 dongle_desconhecido for a dongle other than 1", async () => {
    const r = await create(body("X", { entrada: { tipo: "boot", dongle_id: 2, boot_id: 8 } }));
    expect(r.status).toBe(404);
    expect((r.body as Problem).codigo).toBe("dongle_desconhecido");
  });

  it("accepts a valid interval and optional generation", async () => {
    const i = await create(
      body("I", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T09:00:00-03:00",
          t1: "2026-10-28T19:00:00-03:00",
        },
      }),
    );
    expect(i.status).toBe(202);
    const b = await create(
      body("B", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 20, boot_generation: 3 } }),
    );
    expect(b.status).toBe(202);
  });
});

describe("progression", () => {
  it("goes through pendente, aguardando_dados and pronta; analysis before that gives 409", async () => {
    const id = ((await create(body())).body as { viagem_id: string }).viagem_id;

    const s1 = (await datasource.status(id)).body as TripStatusResponse;
    expect(s1.status).toBe("pendente");
    const a1 = await datasource.analysis(id);
    expect(a1.status).toBe(409);
    expect(a1.body).toMatchObject({ codigo: "analise_indisponivel", viagem_status: "pendente" });

    now += 2000;
    expect(((await datasource.status(id)).body as TripStatusResponse).status).toBe(
      "aguardando_dados",
    );
    expect((await datasource.analysis(id)).body).toMatchObject({
      viagem_status: "aguardando_dados",
    });

    now += 3000;
    const s3 = (await datasource.status(id)).body as TripStatusResponse;
    expect(s3.status).toBe("pronta");
    expect(s3.calculada_em).not.toBeNull();
    const a3 = await datasource.analysis(id);
    expect(a3.status).toBe(200);
    expect((a3.body as TripAnalysis).viagem.motorista).toEqual({ id, nome: "João" });
  });

  it("status carries the API2 fields", async () => {
    const id = ((await create(body())).body as { viagem_id: string }).viagem_id;
    const s = (await datasource.status(id)).body as TripStatusResponse;
    expect(s).toMatchObject({
      viagem_id: id,
      motorista: { nome: "João" },
      categoria_viagem: "feira",
      combustivel: "gasolina_e30",
      provisoria: false,
      calculada_em: null,
    });
    expect(s.criada_em).toMatch(/Z$/);
  });
});

describe("not found", () => {
  it("404 viagem_desconhecida for a missing id or one that is not a UUID", async () => {
    for (const id of ["nao-e-uuid", "00000000-0000-4000-8000-000000000000"]) {
      const s = await datasource.status(id);
      expect(s.status).toBe(404);
      expect((s.body as Problem).codigo).toBe("viagem_desconhecida");
      expect((await datasource.analysis(id)).status).toBe(404);
    }
  });
});

describe("example trips", () => {
  it("there are 9 examples, all ready and valid against the contract", async () => {
    expect(EXAMPLES).toHaveLength(9);
    for (const ex of EXAMPLES) {
      const s = (await datasource.status(ex.id)).body as TripStatusResponse;
      expect(s.status).toBe("pronta");
      expect(s.motorista.nome).toBe(`Exemplo — ${ex.label}`);
      const r = await datasource.analysis(ex.id);
      expect(r.status).toBe(200);
      expect(isProblem(r.body)).toBe(false);
      expect(contractViolations(r.body as TripAnalysis), ex.scenario).toEqual([]);
    }
  });

  it("the scenarios carry the expected state", async () => {
    const byScenario = async (scenario: string) => {
      const ex = EXAMPLES.find((e) => e.scenario === scenario)!;
      return (await datasource.analysis(ex.id)).body as TripAnalysis;
    };
    const codes = (a: TripAnalysis) => a.avisos.map((x) => x.codigo);

    const normal = await byScenario("normal");
    expect(normal.rota?.t_ms.length).toBeGreaterThan(100);
    expect(normal.eventos.map((e) => e.tipo)).toEqual(
      expect.arrayContaining(["aceleracao_brusca", "frenagem_brusca", "marcha_lenta_longa"]),
    );
    expect(normal.viagem.inicio_em).not.toBeNull();
    expect(normal.marchas.trocas.length).toBeGreaterThan(0);
    expect(normal.series.marcha).toContain(0);

    const noGps = await byScenario("sem_gps");
    expect(noGps.rota).toBeNull();
    expect(codes(noGps)).toContain("sem_gps");
    expect(noGps.qualidade.gps.cobertura_pct).toBe(0);

    const gaps = await byScenario("gps_falhas");
    expect(gaps.rota?.segmentos.faixa.filter((f) => f === "falha_gps").length).toBe(2);
    expect(gaps.qualidade.gps.n_falhas).toBe(2);

    const noTime = await byScenario("sem_hora");
    expect(noTime.viagem).toMatchObject({ relogio: null, inicio_em: null, fim_em: null });
    expect(codes(noTime)).toContain("sem_hora_absoluta");

    const empty = await byScenario("sem_leituras");
    expect(codes(empty)).toContain("sem_leituras");
    expect(empty.series.t_ms).toHaveLength(0);
    expect(empty.resumo.distancia_km).toBeNull();

    const pid = await byScenario("pid_ausente");
    expect(codes(pid)).toContain("cobertura_maf_baixa");
    expect(pid.resumo.combustivel_l).toBeNull();
    expect(pid.series.consumo_l_h.every((v) => v === null)).toBe(true);
    expect(pid.series_lentas.nivel_tanque.pct.every((v) => v === null)).toBe(true);

    const interval = await byScenario("intervalo");
    expect(interval.viagem.partes).toHaveLength(2);
    expect(interval.viagem.entrada.tipo).toBe("intervalo");
    expect(interval.resumo.sem_dados_s).toBeGreaterThan(3600);
    expect(interval.rota?.segmentos.faixa).toContain("falha_gps");
    expect(interval.series.rpm.filter((v) => v === null)).toHaveLength(1);
  });

  it("provisional ones become complete 30 s after the first query", async () => {
    const ids = EXAMPLES.filter((e) =>
      ["sem_leituras", "provisoria_aguardando", "dados_parciais"].includes(e.scenario),
    );
    for (const ex of ids) {
      expect(
        ((await datasource.status(ex.id)).body as TripStatusResponse).provisoria,
        ex.scenario,
      ).toBe(true);
    }
    const awaiting = EXAMPLES.find((e) => e.scenario === "provisoria_aguardando")!;
    const partial = EXAMPLES.find((e) => e.scenario === "dados_parciais")!;
    const noReadings = EXAMPLES.find((e) => e.scenario === "sem_leituras")!;

    const before = (await datasource.analysis(awaiting.id)).body as TripAnalysis;
    expect(before.series.t_ms).toHaveLength(0);
    expect(
      ((await datasource.analysis(partial.id)).body as TripAnalysis).avisos.map((a) => a.codigo),
    ).toContain("dados_parciais");

    now += 30_000;
    for (const ex of ids) {
      expect(
        ((await datasource.status(ex.id)).body as TripStatusResponse).provisoria,
        ex.scenario,
      ).toBe(false);
    }
    const after = (await datasource.analysis(awaiting.id)).body as TripAnalysis;
    expect(after.series.t_ms.length).toBeGreaterThan(0);
    expect(contractViolations(after)).toEqual([]);
    const complete = (await datasource.analysis(partial.id)).body as TripAnalysis;
    expect(complete.avisos.map((a) => a.codigo)).not.toContain("dados_parciais");
    // No readings stays without readings, but is no longer provisional.
    expect(
      ((await datasource.analysis(noReadings.id)).body as TripAnalysis).series.t_ms,
    ).toHaveLength(0);
  });

  it("a trip created with each example boot uses the matching scenario", async () => {
    const id = (
      (
        await create(
          body("Gps", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 9, boot_generation: 1 } }),
        )
      ).body as {
        viagem_id: string;
      }
    ).viagem_id;
    now += 5000;
    expect(((await datasource.analysis(id)).body as TripAnalysis).rota).toBeNull();
    const other = (
      (await create(body("Qualquer", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 99 } })))
        .body as { viagem_id: string }
    ).viagem_id;
    now += 5000;
    expect(((await datasource.analysis(other)).body as TripAnalysis).rota).not.toBeNull();
  });
});

describe("fuel", () => {
  it("uses the AFR and density of the chosen preset", async () => {
    const id = (
      (await create(body("Etanol", { combustivel: "etanol_hidratado" }))).body as {
        viagem_id: string;
      }
    ).viagem_id;
    now += 5000;
    const a = (await datasource.analysis(id)).body as TripAnalysis;
    expect(a.parametros).toMatchObject({
      combustivel: "etanol_hidratado",
      afr: 9,
      densidade_g_l: 809,
    });
  });
});

describe("remove", () => {
  it("204; status and analysis then give 404, and a second remove too", async () => {
    const id = ((await create(body())).body as { viagem_id: string }).viagem_id;
    expect(await datasource.remove(id)).toEqual({ status: 204, body: null });
    for (const r of [await datasource.status(id), await datasource.analysis(id)]) {
      expect(r.status).toBe(404);
      expect(isProblem(r.body) && r.body.codigo).toBe("viagem_desconhecida");
    }
    const again = await datasource.remove(id);
    expect(again.status).toBe(404);
    expect(isProblem(again.body) && again.body.codigo).toBe("viagem_desconhecida");
  });

  it("an unknown or malformed id gives 404", async () => {
    for (const id of ["zzz", "7d1f3a10-0001-4c5e-9b1a-0000000000ff"]) {
      expect((await datasource.remove(id)).status).toBe(404);
    }
  });

  it("frees the name and the entry for a new trip", async () => {
    const id = ((await create(body("João"))).body as { viagem_id: string }).viagem_id;
    expect((await create(body("João"))).status).toBe(409);
    await datasource.remove(id);
    // The same name and the same boot, now accepted.
    const again = await create(body("João"));
    expect(again.status).toBe(202);
    expect((again.body as { viagem_id: string }).viagem_id).not.toBe(id);
  });

  it("an example can be removed too", async () => {
    expect((await datasource.remove(EXAMPLES[0].id)).status).toBe(204);
    expect((await datasource.status(EXAMPLES[0].id)).status).toBe(404);
  });
});
