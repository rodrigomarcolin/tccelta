"use client";

import { useMemo } from "react";
import type { Analise } from "@/contrato/tipos";
import type { Eixo } from "@/dominio/eixo";
import { algumValor, temposDaSerie, textoPausa } from "@/dominio/analise";
import { preencherTempos } from "@/dominio/busca";
import { nomeCombustivel } from "@/config";
import { Indicador } from "@/componentes/ui/Indicador";
import { duracao, numero } from "@/formato/numeros";
import { Grafico } from "./Grafico";
import type { Pausa } from "./GraficoUPlot";

type Props = { analise: Analise; eixo: Eixo; formatarX: (x: number) => string };

export function CombustivelMotor({ analise, eixo, formatarX }: Props) {
  const { series: s, series_lentas: l, motor: m, parametros: p } = analise;
  const tempos = useMemo(() => temposDaSerie(analise), [analise]);
  const x = useMemo(() => tempos.map((t) => eixo.para(t)), [tempos, eixo]);
  const xTemp = useMemo(
    () => preencherTempos(l.temperatura.t_ms).map((t) => eixo.para(t)),
    [l.temperatura.t_ms, eixo],
  );
  const xTanque = useMemo(
    () => preencherTempos(l.nivel_tanque.t_ms).map((t) => eixo.para(t)),
    [l.nivel_tanque.t_ms, eixo],
  );
  const pausas: Pausa[] = useMemo(
    () => eixo.pausas.map((q) => ({ x0: q.x0, x1: q.x1, rotulo: textoPausa(q.duracaoMs) })),
    [eixo],
  );
  const combustivel = nomeCombustivel(p.combustivel);
  const aquecido = typeof p.aquecido_c === "number" ? p.aquecido_c : 80;

  const base = { formatarX, pausas, altura: 140 };
  return (
    <div className="secao-conteudo">
      <div className="indicadores-linha">
        <Indicador rotulo="Tempo até aquecer" valor={duracao(m.aquecimento_s)} />
        <Indicador
          rotulo="Temperatura máxima"
          valor={numero(m.temperatura_maxima_c, 0)}
          unidade="°C"
        />
        <Indicador rotulo="Esforço com motor frio" valor={duracao(m.esforco_frio_s)} />
      </div>
      <div className="grade-2">
        <section className="cartao" aria-label="Consumo instantâneo">
          <h3 className="titulo-grafico">
            Consumo instantâneo <span className="unidade">L/h · estimado · {combustivel}</span>
          </h3>
          <Grafico
            {...base}
            x={x}
            titulo="Consumo instantâneo estimado"
            series={[{ rotulo: "Consumo", valores: s.consumo_l_h, cor: "--accent" }]}
            formatarY={(v) => numero(v, 1)}
            semLeituras={!algumValor(s.consumo_l_h)}
          />
        </section>
        <section className="cartao" aria-label="Combustível acumulado">
          <h3 className="titulo-grafico">
            Combustível acumulado <span className="unidade">L · estimado · {combustivel}</span>
          </h3>
          <Grafico
            {...base}
            x={x}
            titulo="Combustível acumulado estimado"
            series={[{ rotulo: "Acumulado", valores: s.combustivel_acumulado_l, cor: "--accent" }]}
            formatarY={(v) => numero(v, 2)}
            semLeituras={!algumValor(s.combustivel_acumulado_l)}
          />
        </section>
        <section className="cartao" aria-label="Carga do motor">
          <h3 className="titulo-grafico">
            Carga do motor <span className="unidade">%</span>
          </h3>
          <Grafico
            {...base}
            x={x}
            titulo="Carga do motor"
            series={[{ rotulo: "Carga", valores: s.carga_pct, cor: "--text-secondary" }]}
            faixaY={[0, 100]}
            formatarY={(v) => numero(v, 0)}
            semLeituras={!algumValor(s.carga_pct)}
          />
        </section>
        <section className="cartao" aria-label="Temperatura do líquido">
          <h3 className="titulo-grafico">
            Temperatura do líquido <span className="unidade">°C</span>
          </h3>
          <Grafico
            {...base}
            x={xTemp}
            titulo="Temperatura do líquido de arrefecimento"
            series={[{ rotulo: "Temperatura", valores: l.temperatura.c, cor: "--text-primary" }]}
            referenciaY={{ y: aquecido, rotulo: `Motor aquecido (${aquecido} °C)` }}
            formatarY={(v) => numero(v, 0)}
            semLeituras={!algumValor(l.temperatura.c)}
          />
        </section>
        <section className="cartao" aria-label="Nível do tanque">
          <h3 className="titulo-grafico">
            Nível do tanque <span className="unidade">% · leitura e mediana móvel</span>
          </h3>
          <Grafico
            {...base}
            x={xTanque}
            titulo="Nível do tanque"
            series={[
              { rotulo: "Leitura", valores: l.nivel_tanque.pct, cor: "--text-tertiary" },
              {
                rotulo: "Mediana móvel",
                valores: l.nivel_tanque.mediana_movel_pct,
                cor: "--accent",
              },
            ]}
            faixaY={[0, 100]}
            formatarY={(v) => numero(v, 0)}
            semLeituras={!algumValor(l.nivel_tanque.pct)}
          />
        </section>
      </div>
    </div>
  );
}
