"use client";

import { useState, type ReactNode } from "react";
import type { Analise } from "@/contrato/tipos";
import { nomeCombustivel } from "@/config";
import { Indicador } from "@/componentes/ui/Indicador";
import { duracao, hora, numero, porcento, TRACO } from "@/formato/numeros";
import { ORDEM_FAIXAS, ROTULO_FAIXA } from "@/formato/faixas";

export function Legenda({ analise }: { analise: Analise }) {
  const [tabela, setTabela] = useState(false);
  const faixas = ORDEM_FAIXAS.map((id) =>
    analise.faixas_velocidade.find((f) => f.id === id),
  ).filter((f): f is NonNullable<typeof f> => f !== undefined);
  if (faixas.length === 0) return null;
  return (
    <div className="legenda">
      <ul className="legenda-lista" aria-label="Faixas de velocidade">
        {faixas.map((f) => (
          <li key={f.id} className="legenda-item">
            <span
              className="amostra"
              style={{ background: `var(--faixa-${f.id})` }}
              aria-hidden="true"
            />
            <span className="legenda-nome">{ROTULO_FAIXA[f.id]}</span>
            <span className="dado legenda-valor">
              {f.id === "parado" ? duracao(f.tempo_s) : porcento(f.distancia_pct, 0)}
            </span>
          </li>
        ))}
      </ul>
      <p className="nota">% da distância percorrida em movimento. Parado mostra o tempo.</p>
      <button
        type="button"
        className="botao botao-link"
        aria-expanded={tabela}
        onClick={() => setTabela((v) => !v)}
      >
        {tabela ? "Ocultar tabela das faixas" : "Ver tabela das faixas"}
      </button>
      {tabela ? (
        <table className="tabela">
          <caption className="so-leitor">Faixas de velocidade</caption>
          <thead>
            <tr>
              <th scope="col">Faixa</th>
              <th scope="col" className="num">
                % da distância
              </th>
              <th scope="col" className="num">
                Tempo
              </th>
            </tr>
          </thead>
          <tbody>
            {faixas.map((f) => (
              <tr key={f.id}>
                <th scope="row">{ROTULO_FAIXA[f.id]}</th>
                <td className="num dado">{porcento(f.distancia_pct, 1)}</td>
                <td className="num dado">{duracao(f.tempo_s)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}

function Grupo({ titulo, children }: { titulo: string; children: ReactNode }) {
  return (
    <section className="grupo" aria-label={titulo}>
      <h3 className="overline">{titulo}</h3>
      <div className="grupo-itens">{children}</div>
    </section>
  );
}

export function Resumo({ analise }: { analise: Analise }) {
  const r = analise.resumo;
  const combustivel = nomeCombustivel(analise.parametros.combustivel);
  const relogio = analise.viagem.relogio;
  const horario =
    relogio && analise.viagem.inicio_t_ms !== null && analise.viagem.fim_t_ms !== null
      ? `${hora(relogio.deslocamento_ms + analise.viagem.inicio_t_ms)}–${hora(relogio.deslocamento_ms + analise.viagem.fim_t_ms)}`
      : null;
  const estimado = `estimado · ${combustivel}`;
  const semConsumo = r.combustivel_l === null;
  const marchaLenta =
    r.marcha_lenta_pct === null
      ? TRACO
      : `${numero(r.marcha_lenta_pct, 0)}% (${duracao(r.marcha_lenta_s)})`;

  return (
    <div className="resumo-grupos">
      <Grupo titulo="Uso">
        <Indicador rotulo="Distância" valor={numero(r.distancia_km, 1)} unidade="km" />
        <Indicador rotulo="Duração" valor={duracao(r.duracao_s)} />
        <Indicador rotulo="Em movimento" valor={duracao(r.em_movimento_s)} />
        {horario ? <Indicador rotulo="Horário" valor={horario} /> : null}
        {r.sem_dados_s !== null && r.sem_dados_s > 0 ? (
          <Indicador rotulo="Sem dados" valor={duracao(r.sem_dados_s)} />
        ) : null}
      </Grupo>
      <Grupo titulo={`Eficiência (${estimado})`}>
        <Indicador
          rotulo="Consumo"
          valor={numero(r.km_por_l, 1)}
          unidade="km/L"
          nota={semConsumo ? "Sem leituras" : estimado}
        />
        <Indicador
          rotulo="Combustível"
          valor={numero(r.combustivel_l, 2)}
          unidade="L"
          nota={semConsumo ? "Sem leituras" : estimado}
        />
        <Indicador rotulo="Marcha lenta" valor={marchaLenta} />
      </Grupo>
      <Grupo titulo="Condução">
        <Indicador
          rotulo="Velocidade média"
          valor={numero(r.velocidade_media_kmh, 0)}
          unidade="km/h"
        />
        <Indicador
          rotulo="Velocidade máxima"
          valor={numero(r.velocidade_maxima_kmh, 0)}
          unidade="km/h"
        />
        <Indicador
          rotulo="Frenagens bruscas"
          valor={numero(r.n_fren_bruscas, 0)}
          nota={r.fren_por_100km === null ? undefined : `${numero(r.fren_por_100km, 1)} por 100 km`}
        />
        <Indicador
          rotulo="Acelerações bruscas"
          valor={numero(r.n_acel_bruscas, 0)}
          nota={r.acel_por_100km === null ? undefined : `${numero(r.acel_por_100km, 1)} por 100 km`}
        />
        <Indicador rotulo="Parado" valor={duracao(r.parado_s)} />
      </Grupo>
    </div>
  );
}
