import type { Analise, CriarViagemResposta, Resposta, StatusResposta } from "@/contrato/tipos";

/**
 * Única porta para dados de viagem. Devolve o status HTTP e o corpo que a API
 * terá (`application/problem+json` nos erros).
 */
export interface ViagensCliente {
  /** `POST /v1/viagens`. `corpo` chega sem validação: o cliente valida. */
  criar(corpo: unknown): Promise<Resposta<CriarViagemResposta>>;
  /** `GET /v1/viagens/{id}`. */
  status(id: string): Promise<Resposta<StatusResposta>>;
  /** `GET /v1/viagens/{id}/analise`. */
  analise(id: string): Promise<Resposta<Analise>>;
}
