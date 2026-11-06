// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { TripAnalysis, CreateTripResponse, TripStatusResponse } from "@/domain/trip/types";
import { isProblem } from "@/domain/trip/types";
import { contractViolations } from "@/tests/contract";
import { createApiDatasource } from "./apiDatasource";
import { createTokenManager } from "./token";

/**
 * Integration with the real API (opt-in). Requires the backend stack up with
 * the dev data (see INT1, "Subir localmente") and the `next-dev` client:
 *
 *   ANALISE_INTEGRACAO_API_URL=http://127.0.0.1:8080 ANALISE_INTEGRACAO_CLIENT_SECRET=$(cat ../backend/deploy/segredos/analise_cliente_secret) ANALISE_INTEGRACAO_DONGLE_ID=1 [ANALISE_INTEGRACAO_BOOT_ID=<boot with data>] npx vitest run apiDatasource.integration
 *
 * Without the variable, the tests are skipped and the suite makes no network call.
 */
const API_URL = process.env.ANALISE_INTEGRACAO_API_URL;
const CLIENT_ID = process.env.ANALISE_INTEGRACAO_CLIENT_ID ?? "next-dev";
const CLIENT_SECRET = process.env.ANALISE_INTEGRACAO_CLIENT_SECRET ?? "";
const DONGLE = Number(process.env.ANALISE_INTEGRACAO_DONGLE_ID ?? "1");
const BOOT = Number(process.env.ANALISE_INTEGRACAO_BOOT_ID ?? "4242");

describe.skipIf(!API_URL)("real trips API (INT1)", () => {
  const tokens = createTokenManager({
    baseUrl: API_URL ?? "",
    clientId: CLIENT_ID,
    clientSecret: CLIENT_SECRET.trim(),
  });
  const client = createApiDatasource({ baseUrl: API_URL ?? "", timeoutMs: 5000, tokens });
  const suffix = Date.now().toString(36);
  const requestBody = (name: string, dongle = DONGLE) => ({
    motorista: { nome: name },
    categoria_viagem: `int1-${suffix}`,
    combustivel: "gasolina_e30",
    entrada: { tipo: "boot", dongle_id: dongle, boot_id: BOOT },
  });

  it("creates, follows until ready and reads the analysis against the contract", async () => {
    const c = await client.create(requestBody("Integração"));
    expect(c.status).toBe(202);
    const { viagem_id: id } = c.body as CreateTripResponse;

    // Before it is ready, the analysis does not exist yet.
    const early = await client.analysis(id);
    if (early.status === 409) {
      expect(early.body).toMatchObject({ codigo: "analise_indisponivel" });
    }

    let st: TripStatusResponse | null = null;
    for (let i = 0; i < 60; i++) {
      const r = await client.status(id);
      expect(r.status).toBe(200);
      st = r.body as TripStatusResponse;
      if (st.status === "pronta" || st.status === "falhou") break;
      await new Promise((ok) => setTimeout(ok, 1000));
    }
    expect(st?.status).toBe("pronta");
    expect(st?.motorista.nome).toBe("Integração");
    expect(st?.calculada_em).not.toBeNull();

    const a = await client.analysis(id);
    expect(a.status).toBe(200);
    const analysis = a.body as TripAnalysis;
    expect(analysis.versao_calculo).toBeTruthy();
    expect(analysis.viagem.entrada.dongle_id).toBe(DONGLE);
    expect(contractViolations(analysis)).toEqual([]);
  }, 90_000);

  it("repeated driver in the category: 409 motorista_repetido", async () => {
    const name = `Repetido ${suffix}`;
    expect((await client.create(requestBody(name))).status).toBe(202);
    const r = await client.create(requestBody(name.toUpperCase()));
    expect(r.status).toBe(409);
    expect(isProblem(r.body) && r.body.codigo).toBe("motorista_repetido");
  });

  it("nonexistent dongle: 404 dongle_desconhecido", async () => {
    const r = await client.create(requestBody(`Sem dongle ${suffix}`, 999_999));
    expect(r.status).toBe(404);
    expect(isProblem(r.body) && r.body.codigo).toBe("dongle_desconhecido");
  });

  it("invalid input: 400 entrada_invalida", async () => {
    const r = await client.create({ ...requestBody("Inválida"), combustivel: "agua" });
    expect(r.status).toBe(400);
    expect(isProblem(r.body) && r.body.codigo).toBe("entrada_invalida");
  });

  it("unknown trip: 404", async () => {
    const r = await client.status("00000000-0000-4000-8000-000000000000");
    expect(r.status).toBe(404);
    expect(isProblem(r.body) && r.body.codigo).toBe("viagem_desconhecida");
  });
});

describe.skipIf(!API_URL)("real API: authentication (INT2)", () => {
  const id = "00000000-0000-4000-8000-000000000000";

  it("without Bearer, the trip routes answer 401", async () => {
    const r = await fetch(`${API_URL}/v1/viagens/${id}`);
    expect(r.status).toBe(401);
    const p = (await r.json()) as { codigo: string };
    expect(p.codigo).toBe("token_ausente");
    const post = await fetch(`${API_URL}/v1/viagens`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    expect(post.status).toBe(401);
  });

  it("the manager token opens the route (trip 404, not 401)", async () => {
    const tokens = createTokenManager({
      baseUrl: API_URL ?? "",
      clientId: CLIENT_ID,
      clientSecret: CLIENT_SECRET.trim(),
    });
    const t = await tokens.getToken();
    expect(t.ok).toBe(true);
    const client = createApiDatasource({ baseUrl: API_URL ?? "", timeoutMs: 5000, tokens });
    expect((await client.status(id)).status).toBe(404);
  });
});
