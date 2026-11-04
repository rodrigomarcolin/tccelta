import { randomUUID } from "node:crypto";
import type {
  Analise,
  CombustivelId,
  CriarViagemResposta,
  Entrada,
  Resposta,
  StatusResposta,
  StatusViagem,
} from "@/contrato/tipos";
import { DONGLE_INICIAL } from "@/config";
import type { ViagensCliente } from "../tipos";
import { construirAnalise } from "./analises";
import {
  AGUARDANDO_ATE_MS,
  CENARIOS_PROVISORIOS,
  cenarioDaEntrada,
  EXEMPLOS,
  PENDENTE_ATE_MS,
  PROVISORIA_MS,
  type CenarioId,
} from "./cenarios";
import { problema } from "@/contrato/problema";
import { normalizarNome, validarCriacao } from "./validacao";

type Registro = {
  id: string;
  nome: string;
  categoria: string;
  combustivel: CombustivelId;
  entrada: Entrada;
  cenario: CenarioId;
  criadaEm: number;
  /** Exemplos já nascem prontos; a fase provisória conta da primeira consulta. */
  exemplo: boolean;
  primeiraConsulta: number | null;
};

type Armazem = Map<string, Registro>;

const CHAVE = "__tccelta_viagens_mock__";

function armazem(agora: number): Armazem {
  const g = globalThis as unknown as Record<string, Armazem | undefined>;
  let mapa = g[CHAVE];
  if (!mapa) {
    mapa = new Map();
    for (const ex of EXEMPLOS) {
      mapa.set(ex.id, {
        id: ex.id,
        nome: `Exemplo — ${ex.rotulo}`,
        categoria: "exemplo",
        combustivel: "gasolina_e30",
        entrada: ex.entrada,
        cenario: ex.cenario,
        criadaEm: agora - 60_000,
        exemplo: true,
        primeiraConsulta: null,
      });
    }
    g[CHAVE] = mapa;
  }
  return mapa;
}

/** Limpa o armazém (uso em testes). */
export function reiniciarMock(): void {
  delete (globalThis as unknown as Record<string, unknown>)[CHAVE];
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function criarMockCliente(agora: () => number = Date.now): ViagensCliente {
  const buscar = (id: string): Registro | undefined => {
    if (!UUID.test(id)) return undefined;
    return armazem(agora()).get(id.toLowerCase());
  };

  function estado(r: Registro): { status: StatusViagem; provisoria: boolean; completa: boolean } {
    const agoraMs = agora();
    if (r.exemplo) r.primeiraConsulta ??= agoraMs;
    const base = r.exemplo ? (r.primeiraConsulta as number) : r.criadaEm;
    const decorrido = agoraMs - r.criadaEm;
    let status: StatusViagem = "pronta";
    if (!r.exemplo) {
      if (decorrido < PENDENTE_ATE_MS) status = "pendente";
      else if (decorrido < AGUARDANDO_ATE_MS) status = "aguardando_dados";
    }
    const completa = agoraMs - base >= PROVISORIA_MS;
    const provisoria = status === "pronta" && CENARIOS_PROVISORIOS.includes(r.cenario) && !completa;
    return { status, provisoria, completa };
  }

  return {
    async criar(corpo: unknown): Promise<Resposta<CriarViagemResposta>> {
      const v = validarCriacao(corpo);
      if (!v.ok) return { status: 400, corpo: v.problema };
      const { motorista, categoria_viagem, combustivel, entrada } = v.valor;
      if (entrada.dongle_id !== DONGLE_INICIAL) {
        return { status: 404, corpo: problema(404, "dongle_desconhecido", "DevId sem cadastro") };
      }
      const mapa = armazem(agora());
      const normalizado = normalizarNome(motorista.nome);
      for (const r of mapa.values()) {
        if (r.categoria === categoria_viagem && normalizarNome(r.nome) === normalizado) {
          return {
            status: 409,
            corpo: problema(409, "motorista_repetido", "nome já usado na categoria"),
          };
        }
      }
      const id = randomUUID();
      mapa.set(id, {
        id,
        nome: motorista.nome,
        categoria: categoria_viagem,
        combustivel,
        entrada,
        cenario: cenarioDaEntrada(entrada),
        criadaEm: agora(),
        exemplo: false,
        primeiraConsulta: null,
      });
      return { status: 202, corpo: { viagem_id: id, status: "pendente" } };
    },

    async status(id: string): Promise<Resposta<StatusResposta>> {
      const r = buscar(id);
      if (!r) return { status: 404, corpo: problema(404, "viagem_desconhecida") };
      const e = estado(r);
      return {
        status: 200,
        corpo: {
          viagem_id: r.id,
          status: e.status,
          provisoria: e.provisoria,
          motorista: { nome: r.nome },
          categoria_viagem: r.categoria,
          combustivel: r.combustivel,
          entrada: r.entrada,
          criada_em: new Date(r.criadaEm).toISOString(),
          // Muda quando a viagem provisória ganha a versão completa (como na API).
          calculada_em:
            e.status === "pronta"
              ? new Date(
                  r.criadaEm +
                    AGUARDANDO_ATE_MS +
                    (e.completa && CENARIOS_PROVISORIOS.includes(r.cenario) ? PROVISORIA_MS : 0),
                ).toISOString()
              : null,
        },
      };
    },

    async analise(id: string): Promise<Resposta<Analise>> {
      const r = buscar(id);
      if (!r) return { status: 404, corpo: problema(404, "viagem_desconhecida") };
      const e = estado(r);
      if (e.status !== "pronta") {
        return {
          status: 409,
          corpo: problema(409, "analise_indisponivel", undefined, { viagem_status: e.status }),
        };
      }
      return {
        status: 200,
        corpo: construirAnalise(
          r.cenario,
          {
            entrada: r.entrada,
            motorista: { id: r.id, nome: r.nome },
            categoria: r.categoria,
            combustivel: r.combustivel,
          },
          e.completa,
        ),
      };
    },
  };
}
