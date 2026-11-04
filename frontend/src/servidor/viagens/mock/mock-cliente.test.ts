import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { Analise, Problema, StatusResposta } from "@/contrato/tipos";
import { ehProblema } from "@/contrato/tipos";
import { violacoesDoContrato } from "@/testes/contrato";
import { EXEMPLOS } from "./cenarios";
import { criarMockCliente, reiniciarMock } from "./mock-cliente";

let agora = 1_000_000;
const cliente = criarMockCliente(() => agora);

const corpo = (nome = "João", extra: Record<string, unknown> = {}) => ({
  motorista: { nome },
  categoria_viagem: "Feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
  ...extra,
});

beforeEach(() => {
  reiniciarMock();
  agora = 1_000_000;
});
afterEach(() => reiniciarMock());

async function criar(c: unknown) {
  return cliente.criar(c);
}

describe("criar", () => {
  it("responde 202 com viagem_id e status pendente", async () => {
    const r = await criar(corpo());
    expect(r.status).toBe(202);
    expect(r.corpo).toMatchObject({ status: "pendente" });
    expect((r.corpo as { viagem_id: string }).viagem_id).toMatch(/^[0-9a-f-]{36}$/);
  });

  it("grava a categoria em minúsculas e recusa nome repetido, sem diferença de caixa e espaços", async () => {
    expect((await criar(corpo("João"))).status).toBe(202);
    const r = await criar(corpo("joão", { categoria_viagem: "FEIRA" }));
    expect(r.status).toBe(409);
    expect((r.corpo as Problema).codigo).toBe("motorista_repetido");
    // Outra categoria libera o nome.
    expect((await criar(corpo("João", { categoria_viagem: "teste" }))).status).toBe(202);
  });

  it.each([
    ["nome vazio", corpo("")],
    ["nome com espaços nas pontas", corpo(" Ana ")],
    ["nome de 61 caracteres", corpo("a".repeat(61))],
    ["nome com controle", corpo("a\u0007b")],
    ["categoria vazia", corpo("X", { categoria_viagem: "" })],
    ["categoria com símbolo", corpo("X", { categoria_viagem: "a/b" })],
    ["categoria longa", corpo("X", { categoria_viagem: "a".repeat(41) })],
    ["combustível desconhecido", corpo("X", { combustivel: "personalizado" })],
    ["sem entrada", corpo("X", { entrada: undefined })],
    ["entrada sem tipo", corpo("X", { entrada: { tipo: "outro" } })],
    ["boot fora da faixa", corpo("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 70000 } })],
    [
      "geração inválida",
      corpo("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: -1 } }),
    ],
    [
      "boot com campo de intervalo",
      corpo("X", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, t0: "x" } }),
    ],
    ["dongle 0", corpo("X", { entrada: { tipo: "boot", dongle_id: 0, boot_id: 8 } })],
    [
      "t0 sem fuso",
      corpo("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T08:00:00",
          t1: "2026-10-28T09:00:00-03:00",
        },
      }),
    ],
    [
      "t1 antes de t0",
      corpo("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T09:00:00-03:00",
          t1: "2026-10-28T08:00:00-03:00",
        },
      }),
    ],
    [
      "intervalo de 8 dias",
      corpo("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-01T09:00:00-03:00",
          t1: "2026-10-09T09:00:01-03:00",
        },
      }),
    ],
    [
      "t0 impossível",
      corpo("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-13-45T09:00:00-03:00",
          t1: "2026-10-28T08:00:00-03:00",
        },
      }),
    ],
    [
      "intervalo com boot_id",
      corpo("X", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T08:00:00-03:00",
          t1: "2026-10-28T09:00:00-03:00",
          boot_id: 8,
        },
      }),
    ],
  ])("400 entrada_invalida: %s", async (_nome, c) => {
    const r = await criar(c);
    expect(r.status).toBe(400);
    expect((r.corpo as Problema).codigo).toBe("entrada_invalida");
    expect((r.corpo as Problema).detail).toBeTruthy();
  });

  it("400 corpo_malformado para campo desconhecido e corpo que não é objeto", async () => {
    for (const c of [{ ...corpo(), extra: 1 }, "texto", null, []]) {
      const r = await criar(c);
      expect(r.status).toBe(400);
      expect((r.corpo as Problema).codigo).toBe("corpo_malformado");
    }
    const r = await criar({ ...corpo(), motorista: { nome: "A", idade: 3 } });
    expect((r.corpo as Problema).codigo).toBe("corpo_malformado");
  });

  it("404 dongle_desconhecido para dongle diferente de 1", async () => {
    const r = await criar(corpo("X", { entrada: { tipo: "boot", dongle_id: 2, boot_id: 8 } }));
    expect(r.status).toBe(404);
    expect((r.corpo as Problema).codigo).toBe("dongle_desconhecido");
  });

  it("aceita intervalo válido e geração opcional", async () => {
    const i = await criar(
      corpo("I", {
        entrada: {
          tipo: "intervalo",
          dongle_id: 1,
          t0: "2026-10-28T08:00:00-03:00",
          t1: "2026-10-28T18:00:00-03:00",
        },
      }),
    );
    expect(i.status).toBe(202);
    const b = await criar(
      corpo("B", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 8, boot_generation: 3 } }),
    );
    expect(b.status).toBe(202);
  });
});

describe("progressão", () => {
  it("passa por pendente, aguardando_dados e pronta; análise antes disso dá 409", async () => {
    const id = ((await criar(corpo())).corpo as { viagem_id: string }).viagem_id;

    const s1 = (await cliente.status(id)).corpo as StatusResposta;
    expect(s1.status).toBe("pendente");
    const a1 = await cliente.analise(id);
    expect(a1.status).toBe(409);
    expect(a1.corpo).toMatchObject({ codigo: "analise_indisponivel", viagem_status: "pendente" });

    agora += 2000;
    expect(((await cliente.status(id)).corpo as StatusResposta).status).toBe("aguardando_dados");
    expect((await cliente.analise(id)).corpo).toMatchObject({ viagem_status: "aguardando_dados" });

    agora += 3000;
    const s3 = (await cliente.status(id)).corpo as StatusResposta;
    expect(s3.status).toBe("pronta");
    expect(s3.calculada_em).not.toBeNull();
    const a3 = await cliente.analise(id);
    expect(a3.status).toBe(200);
    expect((a3.corpo as Analise).viagem.motorista).toEqual({ id, nome: "João" });
  });

  it("status traz os campos da API2", async () => {
    const id = ((await criar(corpo())).corpo as { viagem_id: string }).viagem_id;
    const s = (await cliente.status(id)).corpo as StatusResposta;
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

describe("não encontrada", () => {
  it("404 viagem_desconhecida para id inexistente ou que não é UUID", async () => {
    for (const id of ["nao-e-uuid", "00000000-0000-4000-8000-000000000000"]) {
      const s = await cliente.status(id);
      expect(s.status).toBe(404);
      expect((s.corpo as Problema).codigo).toBe("viagem_desconhecida");
      expect((await cliente.analise(id)).status).toBe(404);
    }
  });
});

describe("viagens de exemplo", () => {
  it("há 9 exemplos, todos prontos e válidos contra o contrato", async () => {
    expect(EXEMPLOS).toHaveLength(9);
    for (const ex of EXEMPLOS) {
      const s = (await cliente.status(ex.id)).corpo as StatusResposta;
      expect(s.status).toBe("pronta");
      expect(s.motorista.nome).toBe(`Exemplo — ${ex.rotulo}`);
      const r = await cliente.analise(ex.id);
      expect(r.status).toBe(200);
      expect(ehProblema(r.corpo)).toBe(false);
      expect(violacoesDoContrato(r.corpo as Analise), ex.cenario).toEqual([]);
    }
  });

  it("os cenários trazem o estado esperado", async () => {
    const por = async (cenario: string) => {
      const ex = EXEMPLOS.find((e) => e.cenario === cenario)!;
      return (await cliente.analise(ex.id)).corpo as Analise;
    };
    const codigos = (a: Analise) => a.avisos.map((x) => x.codigo);

    const normal = await por("normal");
    expect(normal.rota?.t_ms.length).toBeGreaterThan(100);
    expect(normal.eventos.map((e) => e.tipo)).toEqual(
      expect.arrayContaining(["aceleracao_brusca", "frenagem_brusca", "marcha_lenta_longa"]),
    );
    expect(normal.viagem.inicio_em).not.toBeNull();
    expect(normal.marchas.trocas.length).toBeGreaterThan(0);
    expect(normal.series.marcha).toContain(0);

    const semGps = await por("sem_gps");
    expect(semGps.rota).toBeNull();
    expect(codigos(semGps)).toContain("sem_gps");
    expect(semGps.qualidade.gps.cobertura_pct).toBe(0);

    const falhas = await por("gps_falhas");
    expect(falhas.rota?.segmentos.faixa.filter((f) => f === "falha_gps").length).toBe(2);
    expect(falhas.qualidade.gps.n_falhas).toBe(2);

    const semHora = await por("sem_hora");
    expect(semHora.viagem).toMatchObject({ relogio: null, inicio_em: null, fim_em: null });
    expect(codigos(semHora)).toContain("sem_hora_absoluta");

    const vazia = await por("sem_leituras");
    expect(codigos(vazia)).toContain("sem_leituras");
    expect(vazia.series.t_ms).toHaveLength(0);
    expect(vazia.resumo.distancia_km).toBeNull();

    const pid = await por("pid_ausente");
    expect(codigos(pid)).toContain("cobertura_maf_baixa");
    expect(pid.resumo.combustivel_l).toBeNull();
    expect(pid.series.consumo_l_h.every((v) => v === null)).toBe(true);
    expect(pid.series_lentas.nivel_tanque.pct.every((v) => v === null)).toBe(true);

    const intervalo = await por("intervalo");
    expect(intervalo.viagem.partes).toHaveLength(2);
    expect(intervalo.viagem.entrada.tipo).toBe("intervalo");
    expect(intervalo.resumo.sem_dados_s).toBeGreaterThan(3600);
    expect(intervalo.rota?.segmentos.faixa).toContain("falha_gps");
    expect(intervalo.series.rpm.filter((v) => v === null)).toHaveLength(1);
  });

  it("provisórias viram completas depois de 30 s da primeira consulta", async () => {
    const ids = EXEMPLOS.filter((e) =>
      ["sem_leituras", "provisoria_aguardando", "dados_parciais"].includes(e.cenario),
    );
    for (const ex of ids) {
      expect(((await cliente.status(ex.id)).corpo as StatusResposta).provisoria, ex.cenario).toBe(
        true,
      );
    }
    const aguardando = EXEMPLOS.find((e) => e.cenario === "provisoria_aguardando")!;
    const parcial = EXEMPLOS.find((e) => e.cenario === "dados_parciais")!;
    const semLeituras = EXEMPLOS.find((e) => e.cenario === "sem_leituras")!;

    const antes = (await cliente.analise(aguardando.id)).corpo as Analise;
    expect(antes.series.t_ms).toHaveLength(0);
    expect(
      ((await cliente.analise(parcial.id)).corpo as Analise).avisos.map((a) => a.codigo),
    ).toContain("dados_parciais");

    agora += 30_000;
    for (const ex of ids) {
      expect(((await cliente.status(ex.id)).corpo as StatusResposta).provisoria, ex.cenario).toBe(
        false,
      );
    }
    const depois = (await cliente.analise(aguardando.id)).corpo as Analise;
    expect(depois.series.t_ms.length).toBeGreaterThan(0);
    expect(violacoesDoContrato(depois)).toEqual([]);
    const completa = (await cliente.analise(parcial.id)).corpo as Analise;
    expect(completa.avisos.map((a) => a.codigo)).not.toContain("dados_parciais");
    // Sem leituras continua sem leituras, mas já não é provisória.
    expect(((await cliente.analise(semLeituras.id)).corpo as Analise).series.t_ms).toHaveLength(0);
  });

  it("uma viagem criada com cada boot de exemplo usa o cenário correspondente", async () => {
    const id = (
      (await criar(corpo("Gps", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 9 } })))
        .corpo as { viagem_id: string }
    ).viagem_id;
    agora += 5000;
    expect(((await cliente.analise(id)).corpo as Analise).rota).toBeNull();
    const outro = (
      (await criar(corpo("Qualquer", { entrada: { tipo: "boot", dongle_id: 1, boot_id: 99 } })))
        .corpo as { viagem_id: string }
    ).viagem_id;
    agora += 5000;
    expect(((await cliente.analise(outro)).corpo as Analise).rota).not.toBeNull();
  });
});

describe("combustível", () => {
  it("usa AFR e densidade do preset escolhido", async () => {
    const id = (
      (await criar(corpo("Etanol", { combustivel: "etanol_hidratado" }))).corpo as {
        viagem_id: string;
      }
    ).viagem_id;
    agora += 5000;
    const a = (await cliente.analise(id)).corpo as Analise;
    expect(a.parametros).toMatchObject({
      combustivel: "etanol_hidratado",
      afr: 9,
      densidade_g_l: 809,
    });
  });
});
