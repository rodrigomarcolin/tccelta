"use client";

import type { Evento } from "@/contrato/tipos";
import { ROTULO_EVENTO } from "@/dominio/analise";
import { duracao, numero } from "@/formato/numeros";

type Props = {
  eventos: Evento[];
  formatarT: (t: number) => string;
  onVerNoMapa: (e: Evento) => void;
};

export function picoTexto(e: Evento): string {
  return e.pico_unidade === "s" ? duracao(e.pico) : `${numero(e.pico, 1)} m/s²`;
}

export function TabelaEventos({ eventos, formatarT, onVerNoMapa }: Props) {
  if (eventos.length === 0) return <p className="nota">Nenhum evento nesta viagem.</p>;
  return (
    <div className="tabela-rolagem">
      <table className="tabela tabela-eventos">
        <caption className="so-leitor">Eventos da viagem</caption>
        <thead>
          <tr>
            <th scope="col">Tipo</th>
            <th scope="col">Hora</th>
            <th scope="col" className="num">
              Duração
            </th>
            <th scope="col" className="num">
              Pico
            </th>
            <th scope="col" className="num">
              Velocidade
            </th>
            <th scope="col">Ação</th>
          </tr>
        </thead>
        <tbody>
          {eventos.map((e) => {
            const semPosicao = e.lat === null || e.lon === null;
            return (
              <tr key={`${e.tipo}-${e.t_inicio_ms}`}>
                <th scope="row">{ROTULO_EVENTO[e.tipo]}</th>
                <td className="dado">{formatarT(e.t_inicio_ms)}</td>
                <td className="num dado">{duracao(e.duracao_s)}</td>
                <td className="num dado">{picoTexto(e)}</td>
                <td className="num dado">
                  {numero(e.velocidade_inicial_kmh, 0)} → {numero(e.velocidade_final_kmh, 0)} km/h
                </td>
                <td>
                  <span title={semPosicao ? "Sem posição de GPS" : undefined}>
                    <button
                      type="button"
                      className="botao botao-secundario botao-pequeno"
                      disabled={semPosicao}
                      aria-label={`Ver no mapa: ${ROTULO_EVENTO[e.tipo]} às ${formatarT(e.t_inicio_ms)}`}
                      onClick={() => onVerNoMapa(e)}
                    >
                      Ver no mapa
                    </button>
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
