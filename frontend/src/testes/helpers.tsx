import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import type { Analise } from "@/contrato/tipos";
import type { Tema } from "@/config";
import { TemaProvider } from "@/componentes/ui/Tema";
import { construirAnalise } from "@/servidor/viagens/mock/analises";
import type { CenarioId } from "@/servidor/viagens/mock/cenarios";

export function renderComTema(ui: ReactElement, tema: Tema = "escuro") {
  return render(<TemaProvider inicial={tema}>{ui}</TemaProvider>);
}

const cache = new Map<string, Analise>();

/** Análise de um cenário do mock, para os testes de tela. */
export function analiseDe(cenario: CenarioId, completa = false, boot = 8): Analise {
  const chave = `${cenario}:${completa}:${boot}`;
  let a = cache.get(chave);
  if (!a) {
    a = construirAnalise(
      cenario,
      {
        entrada:
          cenario === "intervalo"
            ? {
                tipo: "intervalo",
                dongle_id: 1,
                t0: "2026-10-28T11:00:00.000Z",
                t1: "2026-10-28T21:00:00.000Z",
              }
            : { tipo: "boot", dongle_id: 1, boot_id: boot },
        motorista: { id: "11111111-1111-4111-8111-111111111111", nome: "Maria" },
        categoria: "feira",
        combustivel: "gasolina_e30",
      },
      completa,
    );
    cache.set(chave, a);
  }
  // Cópia profunda: os testes podem alterar o objeto.
  return structuredClone(a);
}
