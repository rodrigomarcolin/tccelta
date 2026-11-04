"use client";

import { useEffect, useState } from "react";
import type { Analise } from "@/contrato/tipos";
import { nomeCombustivel } from "@/config";
import { formatadorDeTempoCompleto } from "@/dominio/analise";
import { duracao, numero, porcento, TRACO } from "@/formato/numeros";

const ORIGEM: Record<string, string> = {
  estimada: "estimada",
  informada: "informada",
  reserva: "valor de reserva",
};

/** Seção 7. Recolhida em tela; aberta na impressão. O conteúdo está sempre no DOM. */
export function Qualidade({ analise }: { analise: Analise }) {
  const [aberta, setAberta] = useState(false);
  const [impressao, setImpressao] = useState(false);
  useEffect(() => {
    const antes = () => setImpressao(true);
    const depois = () => setImpressao(false);
    window.addEventListener("beforeprint", antes);
    window.addEventListener("afterprint", depois);
    return () => {
      window.removeEventListener("beforeprint", antes);
      window.removeEventListener("afterprint", depois);
    };
  }, []);

  const q = analise.qualidade;
  const p = analise.parametros;
  const m = analise.marchas;
  const fmt = formatadorDeTempoCompleto(analise);
  const ignorados = analise.avisos.filter((a) => a.codigo === "boot_sem_hora");
  const sobrepostas = analise.avisos.some((a) => a.codigo === "partes_sobrepostas");
  const limiares = Object.entries(p).filter(
    ([k]) => !["combustivel", "afr", "densidade_g_l"].includes(k),
  );

  return (
    <details
      className="qualidade"
      open={aberta || impressao}
      onToggle={(e) => setAberta((e.currentTarget as HTMLDetailsElement).open)}
    >
      <summary>Qualidade dos dados e metodologia</summary>
      <div className="qualidade-corpo">
        <h3 className="overline">Partes da viagem</h3>
        {analise.viagem.partes.length === 0 ? (
          <p className="nota">Nenhuma parte com dados.</p>
        ) : (
          <table className="tabela">
            <caption className="so-leitor">Partes da viagem</caption>
            <thead>
              <tr>
                <th scope="col">Boot</th>
                <th scope="col">Período com dados</th>
                <th scope="col">Período analisado</th>
              </tr>
            </thead>
            <tbody>
              {analise.viagem.partes.map((pt) => (
                <tr key={`${pt.boot_generation}-${pt.boot_id}`}>
                  <th scope="row">{`boot ${pt.boot_id}`}</th>
                  <td className="dado">{`${fmt(pt.dados_inicio_t_ms)}–${fmt(pt.dados_fim_t_ms)}`}</td>
                  <td className="dado">{`${fmt(pt.inicio_t_ms)}–${fmt(pt.fim_t_ms)}`}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {ignorados.map((a, i) => (
          <p key={i} className="nota">
            Boot ignorado por não ter hora absoluta{a.detalhe ? `: ${a.detalhe}` : ""}.
          </p>
        ))}
        {sobrepostas ? (
          <p className="nota">
            Houve partes sobrepostas; a anterior foi cortada no início da seguinte.
          </p>
        ) : null}

        <h3 className="overline">Leituras</h3>
        <dl className="definicoes">
          <dt>Leituras</dt>
          <dd className="dado">{`${numero(q.leituras_total, 0)} (${numero(q.leituras_por_s, 1)} por segundo)`}</dd>
          {Object.entries(q.periodos_mediana_ms).map(([pid, ms]) => (
            <div key={pid} className="definicao-linha">
              <dt>{`Período mediano do PID ${pid}`}</dt>
              <dd className="dado">{ms === null ? TRACO : `${numero(ms, 0)} ms`}</dd>
            </div>
          ))}
        </dl>

        <h3 className="overline">Razão rotação/velocidade por marcha</h3>
        {m.patamares.length === 0 ? (
          <p className="nota">Nenhuma marcha identificada.</p>
        ) : (
          <ul className="lista-simples">
            {m.patamares.map((pt) => (
              <li key={pt.marcha}>
                <span>{`${pt.marcha}ª`}</span>{" "}
                <span className="dado">{numero(pt.rpm_por_kmh, 1)}</span>
                <span className="unidade">rpm por km/h</span>
              </li>
            ))}
          </ul>
        )}

        <h3 className="overline">GPS</h3>
        <dl className="definicoes">
          <dt>Cobertura</dt>
          <dd className="dado">{porcento(q.gps.cobertura_pct, 0)}</dd>
          <dt>Falhas</dt>
          <dd className="dado">
            {q.gps.n_falhas === null
              ? TRACO
              : `${q.gps.n_falhas} (maior: ${duracao(q.gps.maior_falha_s)})`}
          </dd>
          <dt>Distância pelo OBD</dt>
          <dd className="dado">{`${numero(analise.resumo.distancia_km, 2)} km`}</dd>
          <dt>Distância pelo GPS</dt>
          <dd className="dado">
            {q.gps.distancia_km === null
              ? TRACO
              : `${numero(q.gps.distancia_km, 2)} km (${numero(q.gps.diferenca_distancia_pct, 1)}%)`}
          </dd>
        </dl>

        <h3 className="overline">Combustível e marcha lenta</h3>
        <dl className="definicoes">
          <dt>Combustível usado na estimativa</dt>
          <dd className="dado">{`${nomeCombustivel(p.combustivel)} · AFR ${numero(p.afr, 1)} · ${numero(p.densidade_g_l, 0)} g/L`}</dd>
          <dt>Cobertura do sensor de ar (MAF)</dt>
          <dd className="dado">{porcento(q.combustivel_cobertura_pct, 1)}</dd>
          <dt>Rotação de marcha lenta</dt>
          <dd className="dado">
            {m.marcha_lenta
              ? `${numero(m.marcha_lenta.rpm, 0)} rpm (${ORIGEM[m.marcha_lenta.origem] ?? m.marcha_lenta.origem})`
              : TRACO}
          </dd>
        </dl>

        <h3 className="overline">Parâmetros (limiares do projeto)</h3>
        <p className="nota">
          Valores de projeto, não padrões da literatura. Versão do cálculo: {analise.versao_calculo}
          .
        </p>
        <dl className="definicoes definicoes-compactas">
          {limiares.map(([k, v]) => (
            <div key={k} className="definicao-linha">
              <dt className="dado">{k}</dt>
              <dd className="dado">{String(v)}</dd>
            </div>
          ))}
        </dl>

        <h3 className="overline">Limites do método</h3>
        <ul className="lista-simples">
          <li>
            A estimativa de combustível supõe mistura estequiométrica e não considera correções de
            mistura nem corte de combustível.
          </li>
          <li>
            A posição do GPS é marcada na chegada ao dongle: a 130 km/h, 1 s de atraso equivale a
            cerca de 36 m.
          </li>
          <li>A razão rotação/velocidade sozinha não identifica a marcha real do veículo.</li>
          <li>Os limiares de eventos são heurísticas do projeto.</li>
        </ul>
      </div>
    </details>
  );
}
