// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Analise, CriarViagemResposta, StatusResposta } from "@/contrato/tipos";
import { ehProblema } from "@/contrato/tipos";
import { violacoesDoContrato } from "@/testes/contrato";
import { criarApiCliente } from "./api-cliente";
import { criarGerenciadorToken } from "./token";

/**
 * Integração com a API real (opt-in). Exige a stack do backend no ar com
 * os dados de dev (ver INT1, "Subir localmente") e o cliente `next-dev`:
 *
 *   ANALISE_INTEGRACAO_API_URL=http://127.0.0.1:8080 ANALISE_INTEGRACAO_CLIENT_SECRET=$(cat ../backend/deploy/segredos/analise_cliente_secret) ANALISE_INTEGRACAO_DONGLE_ID=1 [ANALISE_INTEGRACAO_BOOT_ID=<boot com dados>] npx vitest run api-integracao
 *
 * Sem a variável, os testes são ignorados e a suíte não faz chamada de rede.
 */
const URL_API = process.env.ANALISE_INTEGRACAO_API_URL;
const CLIENT_ID = process.env.ANALISE_INTEGRACAO_CLIENT_ID ?? "next-dev";
const CLIENT_SECRET = process.env.ANALISE_INTEGRACAO_CLIENT_SECRET ?? "";
const DONGLE = Number(process.env.ANALISE_INTEGRACAO_DONGLE_ID ?? "1");
const BOOT = Number(process.env.ANALISE_INTEGRACAO_BOOT_ID ?? "4242");

describe.skipIf(!URL_API)("API real de viagens (INT1)", () => {
  const tokens = criarGerenciadorToken({
    baseUrl: URL_API ?? "",
    clientId: CLIENT_ID,
    clientSecret: CLIENT_SECRET.trim(),
  });
  const cliente = criarApiCliente({ baseUrl: URL_API ?? "", timeoutMs: 5000, tokens });
  const sufixo = Date.now().toString(36);
  const corpo = (nome: string, dongle = DONGLE) => ({
    motorista: { nome },
    categoria_viagem: `int1-${sufixo}`,
    combustivel: "gasolina_e30",
    entrada: { tipo: "boot", dongle_id: dongle, boot_id: BOOT },
  });

  it("cria, acompanha até pronta e lê a análise no contrato", async () => {
    const c = await cliente.criar(corpo("Integração"));
    expect(c.status).toBe(202);
    const { viagem_id: id } = c.corpo as CriarViagemResposta;

    // Antes de pronta, a análise ainda não existe.
    const cedo = await cliente.analise(id);
    if (cedo.status === 409) {
      expect(cedo.corpo).toMatchObject({ codigo: "analise_indisponivel" });
    }

    let st: StatusResposta | null = null;
    for (let i = 0; i < 60; i++) {
      const r = await cliente.status(id);
      expect(r.status).toBe(200);
      st = r.corpo as StatusResposta;
      if (st.status === "pronta" || st.status === "falhou") break;
      await new Promise((ok) => setTimeout(ok, 1000));
    }
    expect(st?.status).toBe("pronta");
    expect(st?.motorista.nome).toBe("Integração");
    expect(st?.calculada_em).not.toBeNull();

    const a = await cliente.analise(id);
    expect(a.status).toBe(200);
    const analise = a.corpo as Analise;
    expect(analise.versao_calculo).toBeTruthy();
    expect(analise.viagem.entrada.dongle_id).toBe(DONGLE);
    expect(violacoesDoContrato(analise)).toEqual([]);
  }, 90_000);

  it("repete motorista na categoria: 409 motorista_repetido", async () => {
    const nome = `Repetido ${sufixo}`;
    expect((await cliente.criar(corpo(nome))).status).toBe(202);
    const r = await cliente.criar(corpo(nome.toUpperCase()));
    expect(r.status).toBe(409);
    expect(ehProblema(r.corpo) && r.corpo.codigo).toBe("motorista_repetido");
  });

  it("dongle inexistente: 404 dongle_desconhecido", async () => {
    const r = await cliente.criar(corpo(`Sem dongle ${sufixo}`, 999_999));
    expect(r.status).toBe(404);
    expect(ehProblema(r.corpo) && r.corpo.codigo).toBe("dongle_desconhecido");
  });

  it("entrada inválida: 400 entrada_invalida", async () => {
    const r = await cliente.criar({ ...corpo("Inválida"), combustivel: "agua" });
    expect(r.status).toBe(400);
    expect(ehProblema(r.corpo) && r.corpo.codigo).toBe("entrada_invalida");
  });

  it("viagem desconhecida: 404", async () => {
    const r = await cliente.status("00000000-0000-4000-8000-000000000000");
    expect(r.status).toBe(404);
    expect(ehProblema(r.corpo) && r.corpo.codigo).toBe("viagem_desconhecida");
  });
});

describe.skipIf(!URL_API)("API real: autenticação (INT2)", () => {
  const id = "00000000-0000-4000-8000-000000000000";

  it("sem Bearer, as rotas de viagem respondem 401", async () => {
    const r = await fetch(`${URL_API}/v1/viagens/${id}`);
    expect(r.status).toBe(401);
    const p = (await r.json()) as { codigo: string };
    expect(p.codigo).toBe("token_ausente");
    const post = await fetch(`${URL_API}/v1/viagens`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    expect(post.status).toBe(401);
  });

  it("o token do gerenciador abre a rota (404 de viagem, não 401)", async () => {
    const tokens = criarGerenciadorToken({
      baseUrl: URL_API ?? "",
      clientId: CLIENT_ID,
      clientSecret: CLIENT_SECRET.trim(),
    });
    const t = await tokens.getToken();
    expect(t.ok).toBe(true);
    const cliente = criarApiCliente({ baseUrl: URL_API ?? "", timeoutMs: 5000, tokens });
    expect((await cliente.status(id)).status).toBe(404);
  });
});
