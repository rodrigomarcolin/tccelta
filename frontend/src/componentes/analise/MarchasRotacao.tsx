"use client";

import { useMemo, useState } from "react";
import type { Analise, FaixaDistribuicao } from "@/contrato/tipos";
import { Indicador } from "@/componentes/ui/Indicador";
import { algumValor, rotuloMarcha } from "@/dominio/analise";
import { duracao, numero, TRACO } from "@/formato/numeros";
import { BarrasHorizontais, Dispersao, MapaDeCalor, type ItemBarra } from "./graficos-svg";
import { varMarcha } from "./cores";

type Props = {
  analise: Analise;
  formatarT: (t: number) => string;
  onMarcarInstante: (t: number) => void;
};

function rotuloFaixaRpm(f: FaixaDistribuicao): string {
  return f.ate === null ? `≥ ${numero(f.de, 0)} rpm` : `${numero(f.de, 0)}–${numero(f.ate, 0)} rpm`;
}

function rotuloFaixaPedal(f: FaixaDistribuicao): string {
  const faixa = `${f.de}–${f.ate}%`;
  return f.rotulo ? `${f.rotulo} (${faixa})` : faixa;
}

function arredondarAcima(v: number, passo: number): number {
  return Math.max(passo, Math.ceil(v / passo) * passo);
}

export function MarchasRotacao({ analise, formatarT, onMarcarInstante }: Props) {
  const { series: s, marchas: m, condutor: c, distribuicoes: d, resumo: r } = analise;
  const [tabelaAberta, setTabelaAberta] = useState(false);
  const semRpm = !algumValor(s.rpm);

  const dispersao = useMemo(() => {
    const pontos: { x: number; y: number; cor: string }[] = [];
    const passo = Math.max(1, Math.ceil(s.t_ms.length / 1200));
    let vMax = 20;
    let rMax = 1000;
    for (let i = 0; i < s.t_ms.length; i += passo) {
      const v = s.velocidade_kmh[i];
      const rpm = s.rpm[i];
      const g = s.marcha[i];
      if (
        v === null ||
        rpm === null ||
        g === null ||
        g === undefined ||
        v === undefined ||
        rpm === undefined ||
        v < 8
      )
        continue;
      pontos.push({ x: v, y: rpm, cor: varMarcha(g) });
      vMax = Math.max(vMax, v);
      rMax = Math.max(rMax, rpm);
    }
    return { pontos, vMax: arredondarAcima(vMax, 20), rMax: arredondarAcima(rMax, 500) };
  }, [s]);

  const trocas = m.trocas;
  const subidas = trocas.filter((t) => t.direcao === "subida");
  const reducoes = trocas.filter((t) => t.direcao === "reducao");
  const rMaxTrocas = arredondarAcima(Math.max(1000, ...trocas.map((t) => t.rpm_antes)), 500);

  const pontosTroca = (lista: typeof trocas, centro: number) =>
    lista.map((t, i) => ({
      x: centro + (((i * 37) % 21) - 10) / 40,
      y: t.rpm_antes,
      cor: "--accent",
      rotulo: `${t.direcao === "subida" ? "Subida" : "Redução"} de ${t.de}ª para ${t.para}ª às ${formatarT(t.t_ms)}: ${numero(t.rpm_antes, 0)} rpm. Marcar na linha do tempo`,
      onClick: () => onMarcarInstante(t.t_ms),
    }));

  const barrasMarcha: ItemBarra[] = [
    ...m.patamares.map((p) => ({
      rotulo: rotuloMarcha(p.marcha),
      pct: p.tempo_pct,
      tempoS: p.tempo_s,
      cor: varMarcha(p.marcha),
    })),
    ...(m.ponto_morto_embreagem_s !== null && m.ponto_morto_embreagem_s > 0
      ? [
          {
            rotulo: "Ponto morto ou embreagem",
            pct: r.em_movimento_s ? (m.ponto_morto_embreagem_s / r.em_movimento_s) * 100 : null,
            tempoS: m.ponto_morto_embreagem_s,
            cor: "--marcha-0",
          },
        ]
      : []),
  ];

  return (
    <div className="secao-conteudo">
      <div className="indicadores-linha">
        <Indicador
          rotulo="Rotação mediana ao subir"
          valor={numero(m.subida_rpm.mediana, 0)}
          unidade="rpm"
          nota={m.subida_rpm.n ? `${m.subida_rpm.n} trocas` : undefined}
        />
        <Indicador
          rotulo="Rotação mediana ao reduzir"
          valor={numero(m.reducao_rpm.mediana, 0)}
          unidade="rpm"
          nota={m.reducao_rpm.n ? `${m.reducao_rpm.n} trocas` : undefined}
        />
        <Indicador
          rotulo="Pedal médio em movimento"
          valor={numero(c.pedal_medio_pct, 0)}
          unidade="%"
        />
        <Indicador rotulo="Freio-motor" valor={numero(c.freio_motor_pct, 0)} unidade="%" />
        <Indicador
          rotulo="Variação do pedal (p95)"
          valor={numero(c.pedal_variacao_p95_pct_s, 0)}
          unidade="%/s"
        />
      </div>

      <div className="grade-2">
        <section className="cartao" aria-label="Rotação por velocidade">
          <h3 className="titulo-grafico">Rotação × velocidade</h3>
          {semRpm || dispersao.pontos.length === 0 ? (
            <p className="nota">Sem leituras</p>
          ) : (
            <Dispersao
              titulo="Rotação por velocidade, colorida por marcha"
              pontos={dispersao.pontos}
              linhas={m.patamares
                .filter((p) => p.marcha > 0)
                .map((p) => ({
                  x1: 8,
                  y1: p.rpm_por_kmh * 8,
                  x2: dispersao.vMax,
                  y2: Math.min(dispersao.rMax, p.rpm_por_kmh * dispersao.vMax),
                  cor: varMarcha(p.marcha),
                  rotulo: `${p.marcha}ª`,
                }))}
              dominioX={[0, dispersao.vMax]}
              dominioY={[0, dispersao.rMax]}
              ticksX={Array.from({ length: dispersao.vMax / 20 + 1 }, (_, i) => ({
                v: i * 20,
                rotulo: String(i * 20),
              }))}
              ticksY={Array.from({ length: dispersao.rMax / 1000 + 1 }, (_, i) => ({
                v: i * 1000,
                rotulo: numero(i * 1000, 0),
              }))}
              rotuloX="Velocidade (km/h)"
              rotuloY="Rotação (rpm)"
            />
          )}
          <p className="nota">Pontos de ponto morto ou embreagem ficam fora, sem reta.</p>
        </section>

        <section className="cartao" aria-label="Tempo em cada marcha">
          <h3 className="titulo-grafico">Tempo em cada marcha</h3>
          {barrasMarcha.length === 0 ? (
            <p className="nota">Sem leituras</p>
          ) : (
            <BarrasHorizontais titulo="Tempo em cada marcha" itens={barrasMarcha} />
          )}
          <p className="nota">
            A numeração é relativa ao que foi observado, não às marchas reais do veículo.
          </p>
        </section>

        <section className="cartao" aria-label="Rotação nas trocas">
          <h3 className="titulo-grafico">Rotação nas trocas</h3>
          {trocas.length === 0 ? (
            <p className="nota">Sem trocas de marcha identificadas</p>
          ) : (
            <Dispersao
              titulo="Rotação antes de cada troca, separada em subidas e reduções"
              pontos={[...pontosTroca(subidas, 1), ...pontosTroca(reducoes, 3)]}
              tracos={[
                ...(m.subida_rpm.mediana !== null
                  ? [{ x1: 0.6, x2: 1.4, y: m.subida_rpm.mediana }]
                  : []),
                ...(m.reducao_rpm.mediana !== null
                  ? [{ x1: 2.6, x2: 3.4, y: m.reducao_rpm.mediana }]
                  : []),
              ]}
              dominioX={[0, 4]}
              dominioY={[0, rMaxTrocas]}
              ticksX={[
                { v: 1, rotulo: "Subidas" },
                { v: 3, rotulo: "Reduções" },
              ]}
              ticksY={Array.from({ length: rMaxTrocas / 1000 + 1 }, (_, i) => ({
                v: i * 1000,
                rotulo: numero(i * 1000, 0),
              }))}
              rotuloX="Tipo de troca (traço: mediana)"
              rotuloY="Rotação antes (rpm)"
              altura={260}
            />
          )}
          <button
            type="button"
            className="botao botao-link"
            aria-expanded={tabelaAberta}
            onClick={() => setTabelaAberta((v) => !v)}
          >
            {tabelaAberta ? "Ocultar tabela de trocas" : "Ver tabela de trocas"}
          </button>
          {tabelaAberta ? (
            <table className="tabela">
              <caption className="so-leitor">Trocas de marcha</caption>
              <thead>
                <tr>
                  <th scope="col">Hora</th>
                  <th scope="col">Tipo</th>
                  <th scope="col">Marchas</th>
                  <th scope="col" className="num">
                    Rotação antes
                  </th>
                  <th scope="col" className="num">
                    Rotação depois
                  </th>
                  <th scope="col" className="num">
                    Velocidade
                  </th>
                </tr>
              </thead>
              <tbody>
                {trocas.map((t) => (
                  <tr key={t.t_ms}>
                    <td className="dado">{formatarT(t.t_ms)}</td>
                    <td>{t.direcao === "subida" ? "Subida" : "Redução"}</td>
                    <td className="dado">{`${t.de}ª → ${t.para}ª`}</td>
                    <td className="num dado">{numero(t.rpm_antes, 0)}</td>
                    <td className="num dado">{numero(t.rpm_depois, 0)}</td>
                    <td className="num dado">{numero(t.velocidade_kmh, 0)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </section>

        <section className="cartao" aria-label="Rotação por pedal">
          <h3 className="titulo-grafico">Rotação × pedal</h3>
          <MapaDeCalor
            tempoS={d.rotacao_pedal.tempo_s}
            rpmBin={d.rotacao_pedal.rpm_bin}
            pedalBin={d.rotacao_pedal.pedal_bin}
          />
        </section>

        <section className="cartao" aria-label="Posição do pedal">
          <h3 className="titulo-grafico">Posição do pedal</h3>
          {d.pedal.length === 0 ? (
            <p className="nota">Sem leituras</p>
          ) : (
            <BarrasHorizontais
              titulo="Tempo por posição do pedal"
              itens={d.pedal.map((f) => ({
                rotulo: rotuloFaixaPedal(f),
                pct: f.pct,
                tempoS: f.tempo_s,
              }))}
            />
          )}
        </section>

        <section className="cartao" aria-label="Faixas de rotação">
          <h3 className="titulo-grafico">Faixas de rotação</h3>
          {d.rotacao.length === 0 ? (
            <p className="nota">Sem leituras</p>
          ) : (
            <BarrasHorizontais
              titulo="Tempo por faixa de rotação"
              itens={d.rotacao.map((f) => ({
                rotulo: rotuloFaixaRpm(f),
                pct: f.pct,
                tempoS: f.tempo_s,
              }))}
            />
          )}
        </section>
      </div>
      <p className="nota">
        Marcha lenta:{" "}
        {m.marcha_lenta ? `${numero(m.marcha_lenta.rpm, 0)} rpm (${m.marcha_lenta.origem})` : TRACO}
        . Tempo parado: {duracao(r.parado_s)}.
      </p>
    </div>
  );
}
