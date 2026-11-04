# 02 — Cálculos e contrato JSON da análise de viagem

> **Status: especificação de algo a desenvolver.** Este documento define como calcular cada elemento da tela de [`01-tela-mapa-velocidade.md`](01-tela-mapa-velocidade.md) e como expor o resultado em JSON. Não define rotas, autenticação, organização do código, tabelas, consultas nem onde o cálculo é armazenado.

## Princípios

- **Uma só fonte de cálculo:** a API entrega os valores prontos. A tela não repete regras de negócio.
- **Ausência é `null`:** falta de dado nunca vira `0`.
- **Sem arredondar:** o arredondamento é decisão da tela.
- **Reprodutível:** a resposta traz `versao_calculo` e os parâmetros usados.
- **Limiares são do projeto:** os valores padrão são heurísticas e não vêm da literatura.

## Entradas lógicas

| Entrada | Conteúdo |
|---|---|
| Definição da viagem | Seção "Definição da viagem" |
| Leituras | Por boot e PID: `t_ms` no relógio do boot e valor decodificado |
| Posições | Por boot: `t_ms` no relógio do boot, latitude e longitude |
| Hora | Por boot, conversão opcional de `t_ms` para hora absoluta, com incerteza |
| Parâmetros | Seção "Parâmetros" |

PIDs usados: `0x04` carga, `0x05` líquido de arrefecimento, `0x0C` rotação, `0x0D` velocidade, `0x10` MAF, `0x2F` nível do tanque, `0x49` pedal.

## Definição da viagem

A requisição define a viagem por exatamente uma destas entradas:

| `tipo` | Campos | Dados considerados |
|---|---|---|
| `boot` | `dongle_id`, `boot_generation`, `boot_id` | Todas as leituras e posições desse boot |
| `intervalo` | `dongle_id`, `t0`, `t1` (ISO 8601 com fuso; `t1 > t0`) | Leituras e posições do dongle com `t0 ≤ hora absoluta < t1`, de qualquer boot |

### Partes da viagem

Uma **parte** reúne os dados contínuos de um boot dentro da viagem. A entrada `boot` gera uma parte. A entrada `intervalo` gera uma parte para cada boot com dados no intervalo, em ordem cronológica.

Regras da entrada `intervalo`:

- Um boot que começa ou termina fora do intervalo contribui só com os dados de dentro dele.
- Um boot sem conversão para hora absoluta não pode ser posicionado no tempo: fica de fora e gera o aviso `boot_sem_hora`.
- Se a incerteza da conversão fizer duas partes se sobreporem, a parte anterior termina no início da seguinte, e a resposta traz o aviso `partes_sobrepostas`.

Sem nenhum dado, os valores calculados vêm `null`, e a resposta traz o aviso `sem_leituras`.

### Eixo de tempo

Todos os campos `t_ms` da resposta, `de_t_ms` e `ate_t_ms` usam o mesmo eixo:

| `tipo` | Significado de `t_ms` | `relogio` |
|---|---|---|
| `boot` | Relógio do boot | Conversão do boot; `null` se não houver |
| `intervalo` | Milissegundos desde `t0` | `deslocamento_ms` é `t0` em milissegundos desde a época Unix; `incerteza_ms` é a maior incerteza entre as partes |

Em ambos os casos, `hora absoluta = deslocamento_ms + t_ms`.

## Parâmetros

Todos são opcionais na requisição e voltam em `parametros` na resposta.

| Parâmetro | Padrão | Uso |
|---|---|---|
| `combustivel` | `gasolina_e30` | Preset; `personalizado` exige `afr` e `densidade_g_l` |
| `afr`, `densidade_g_l` | do preset | Estimativa de combustível |
| `marcha_lenta_rpm` | automático | Rotação de marcha lenta; se informado, substitui a estimativa |
| `marcha_lenta_margem_rpm` | 150 | Margem acima da marcha lenta (ponto morto ou embreagem) |
| `marcha_lenta_reserva_rpm` | 750 | Usado se não houver amostras para estimar a marcha lenta |
| `motor_ligado_rpm` | 300 | Motor ligado |
| `movimento_kmh` | 3 | Separa parado de em movimento |
| `gps_falha_s` | 5 | Intervalo entre pontos a partir do qual há falha |
| `velocidade_idade_max_ms` | 2000 | Idade máxima da velocidade em um segmento sem leitura |
| `acel_brusca_ms2` | 3,0 | Aceleração brusca |
| `fren_brusca_ms2` | −3,5 | Frenagem brusca |
| `evento_min_ms` | 500 | Duração mínima de evento brusco |
| `marcha_lenta_longa_ms` | 60000 | Evento de marcha lenta longa |
| `parada_min_s` | 5 | Duração mínima para marcador de parada |
| `taxa_min_km` | 20 | Distância mínima para taxas por 100 km |
| `pedal_alto_pct` | 80 | Início da faixa de pedal alto |
| `pedal_alto_aviso_fracao` | 0,10 | Fração do tempo com motor ligado que dispara o aviso |
| `freio_motor_pedal_pct`, `freio_motor_rpm`, `freio_motor_kmh` | 3, 1200, 15 | Freio-motor |
| `aquecido_c`, `frio_c`, `frio_rpm`, `frio_pedal_pct` | 80, 60, 3000, 60 | Aquecimento e esforço com motor frio |
| `serie_max_pontos` | 5000 | Limite de pontos por série exposta |
| `de_t_ms`, `ate_t_ms` | ausentes | Trecho selecionado (ver "Resumo de um trecho") |

### Presets de combustível

| `combustivel` | Nome | AFR | Densidade (g/L) |
|---|---|---|---|
| `gasolina_e30` | Gasolina E30 | 13,0 | 750 |
| `gasolina_e32` | Gasolina E32 | 13,0 | 750 |
| `gasolina_e27` | Gasolina E27 | 13,2 | 754 |
| `gasolina_pura` | Gasolina pura | 14,7 | 745 |
| `etanol_hidratado` | Etanol hidratado | 9,0 | 809 |
| `diesel` | Diesel | 14,5 | 832 |

Para E30 e E32, a densidade é o ponto médio da faixa típica de 735 a 765 g/L a 20 °C.

## Pré-processamento

1. **Séries:** em cada parte, ordenar por `t_ms` e manter a primeira leitura de cada `t_ms` por PID.
2. **Dados da parte:** do primeiro ao último instante com leitura, entre todos os PIDs.
3. **Passo da grade:** único para a viagem. É a mediana dos intervalos medianos de rotação, velocidade e pedal, arredondada para múltiplos de 50 ms, limitada a 100–1000 ms. Sem esses PIDs, 1000 ms. Os intervalos são medidos dentro de cada parte.
4. **Grade:** uma por parte, com um ponto a cada `passo` desde o início dos dados da parte. Não há pontos entre partes.
5. **Reamostragem:** cada ponto recebe a última leitura da parte em ou antes dele, se a idade não passar de `max(3 × intervalo mediano do PID, 2 × passo)`. Caso contrário, `null`.
6. **Motor ligado:** `rpm ≥ motor_ligado_rpm`.
7. **Janela da parte:** do primeiro ao último ponto com motor ligado. Uma parte sem motor ligado fica fora da análise.
8. **Janela da viagem:** a união das janelas das partes. Se nenhuma parte tiver motor ligado, usa todos os dados e adiciona o aviso `motor_nunca_ligado`.

Salvo indicação em contrário, os cálculos usam os pontos da grade dentro da janela da viagem, e `dt = passo / 1000`.

Cálculos que dependem de pontos vizinhos ou de sequências contínuas (aceleração, amostras estáveis, trechos de marcha, trocas, eventos, paradas e variação do pedal) não cruzam partes. A rotação de marcha lenta e as marchas detectadas são únicas para toda a viagem.

## Cálculos

### Resumo

| Elemento | Campo | Cálculo |
|---|---|---|
| Distância | `resumo.distancia_km` | `Σ velocidade × dt / 3600` |
| Duração | `resumo.duracao_s` | Soma das durações das janelas das partes |
| Tempo sem dados | `resumo.sem_dados_s` | Fim da última janela menos início da primeira, menos `duracao_s` |
| Tempo em movimento | `resumo.em_movimento_s` | Tempo com `velocidade ≥ movimento_kmh` |
| Tempo parado | `resumo.parado_s` | Tempo com `velocidade < movimento_kmh` |
| Velocidade média | `resumo.velocidade_media_kmh` | Média da velocidade nos pontos em movimento |
| Velocidade máxima | `resumo.velocidade_maxima_kmh` | Máximo das leituras de velocidade na janela |
| Motor ligado | `resumo.motor_ligado_s` | Tempo com motor ligado |
| Marcha lenta | `resumo.marcha_lenta_s` | Tempo parado com motor ligado |
| % de marcha lenta | `resumo.marcha_lenta_pct` | `marcha_lenta_s / motor_ligado_s × 100` |
| Litros | `resumo.combustivel_l` | Ver "Combustível" |
| km/L | `resumo.km_por_l` | `distancia_km / combustivel_l`; `null` se `combustivel_l ≤ 0,02` |
| Eventos | `resumo.n_acel_bruscas`, `resumo.n_fren_bruscas` | Contagem de eventos |
| Taxas | `resumo.acel_por_100km`, `resumo.fren_por_100km` | Contagem × 100 / distância; `null` se `distancia_km < taxa_min_km` |
| Horário | `viagem.inicio_em`, `viagem.fim_em` | Hora absoluta do início da primeira janela e do fim da última; `null` sem conversão |

### Faixas de velocidade

| `id` | Intervalo (km/h) |
|---|---|
| `parado` | `v < 3` (`movimento_kmh`) |
| `ate_30` | `3 ≤ v < 30` |
| `30_60` | `30 ≤ v < 60` |
| `60_80` | `60 ≤ v < 80` |
| `80_110` | `80 ≤ v < 110` |
| `acima_110` | `v ≥ 110` |

A distribuição é calculada por ponto da grade, então existe mesmo sem GPS. Cada faixa soma tempo e distância. A % de distância usa como total só as faixas em movimento; `parado` traz `distancia_pct` igual a `null`.

### Rota

1. Considerar as posições dentro da janela da viagem, em ordem de `t_ms`.
2. Para cada par consecutivo `i` e `i+1`, formar um segmento:
   - Se os pontos forem de partes diferentes ou `t[i+1] − t[i] > gps_falha_s`, a faixa é `falha_gps` e a velocidade é `null`.
   - Senão, a velocidade é a **média das leituras brutas de `0x0D`** em `[t[i], t[i+1])`.
   - Sem leitura no intervalo, usa a última leitura se tiver até `velocidade_idade_max_ms` de idade.
   - Sem nenhuma das duas, a faixa é `sem_velocidade` e a velocidade é `null`.
3. Definir a faixa pela velocidade do segmento. A cor nunca usa a velocidade do GPS.

Limite conhecido: o `t_ms` da posição é marcado na chegada ao dongle, não na leitura no celular. A 130 km/h, 1 s de atraso equivale a cerca de 36 m. Esta versão não corrige esse atraso.

### Paradas

Trechos contínuos com `velocidade < movimento_kmh` de pelo menos `parada_min_s`. A posição é a mais próxima do início, com diferença de até `gps_falha_s`; caso contrário, `null`.

### Aceleração e eventos

**Aceleração:** diferença central de ±0,5 s na grade, com `h = max(1, round(500 / passo))`:

`a[i] = (v[i+h] − v[i−h]) / 3,6 / (2 × h × dt)`

| Evento | Condição | Mínimo | Pico |
|---|---|---|---|
| `aceleracao_brusca` | `a ≥ acel_brusca_ms2` | `evento_min_ms` | Máximo de `a` |
| `frenagem_brusca` | `a ≤ fren_brusca_ms2` | `evento_min_ms` | Mínimo de `a` |
| `marcha_lenta_longa` | Parado com motor ligado | `marcha_lenta_longa_ms` | Duração em segundos |

- O evento é uma sequência contínua de pontos que atendem à condição.
- A duração é `t_fim − t_inicio + passo`.
- `velocidade_inicial_kmh` e `velocidade_final_kmh` vêm do primeiro e do último ponto.
- A posição segue a mesma regra das paradas.
- O pico é exposto como calculado, sem filtro por valor máximo.

### Marcha lenta, ponto morto e marcha

**Rotação de marcha lenta:** se `marcha_lenta_rpm` foi informado, usa esse valor (`origem: "informada"`). Senão, é a mediana da rotação nos pontos parados com motor ligado (`origem: "estimada"`). Com menos de 10 amostras, usa `marcha_lenta_reserva_rpm` (`origem: "reserva"`).

**Ponto morto ou embreagem:** um ponto com `velocidade ≥ 8 km/h` e `rpm ≤ marcha_lenta + marcha_lenta_margem_rpm` recebe `marcha = 0`. Esses pontos ficam fora da detecção das marchas. Uma marcha real pode cair nessa regra em rotação muito baixa, por isso a margem é um parâmetro.

**Detecção das marchas**, nos demais pontos com `velocidade ≥ 8 km/h` e `rpm ≥ 500`:

1. Calcular `ln k = ln(rpm / velocidade)`.
2. **Amostra estável:** todos os pontos na vizinhança de ±0,5 s têm `ln k` definido e a diferença para o ponto central é de até `0,04 + 1/velocidade`. Com menos de 20 amostras estáveis, não há marchas.
3. Montar o histograma de `ln k` estável, com bins de 0,005, suavizado por gaussiana de σ = 3 bins e cortada em ±3σ.
4. **Candidatos:** máximos locais com altura de pelo menos 0,2% do maior pico e pelo menos `round(1500 / passo)` amostras a até 0,05 do bin.
5. Percorrer os candidatos do mais alto ao mais baixo, mantendo só os que estão a pelo menos 0,09 dos já mantidos.
6. O centro é a mediana de `ln k` das amostras próximas. A marcha 1 é a de maior `k`, e `rpm_por_kmh = exp(centro)`.
7. **Atribuição:** cada ponto recebe a marcha mais próxima se a distância for até `0,06 + 1/(2 × velocidade)`.
8. **Limpeza:** descartar trechos com menos de 1000 ms. Depois, preencher lacunas `null` de até 1500 ms entre trechos da mesma marcha. Lacunas com `marcha = 0` não são preenchidas.

A numeração é relativa ao que foi observado, não às marchas reais do veículo. A razão rotação/velocidade sozinha não basta para identificar a marcha ([9] (2025)).

**Trocas:** dois trechos consecutivos de marchas diferentes, ignorando `null` e `0`, com até 4000 ms entre o fim de um e o início do outro. O instante é o fim do primeiro trecho, a direção é `subida` ou `reducao`, e `rpm_antes`, `rpm_depois` e a velocidade vêm dos pontos de cada lado.

**Indicadores:** mediana e quartis de `rpm_antes` nas subidas; mediana de `rpm_antes` nas reduções.

### Pedal e rotação

| Elemento | Campo | Cálculo |
|---|---|---|
| Faixas de rotação | `distribuicoes.rotacao` | Tempo com motor ligado em 0–1000, 1000–1500, 1500–2000, 2000–2500, 2500–3000, 3000–3500, 3500–4000, 4000–5000 e ≥ 5000 rpm |
| Posição do pedal | `distribuicoes.pedal` | Tempo com motor ligado em 0–5 (solto), 5–20, 20–40, 40–60, 60–80 e 80–100% |
| Rotação × pedal | `distribuicoes.rotacao_pedal` | Tempo com motor ligado em células de 500 rpm × 10% de pedal |
| Pedal médio | `condutor.pedal_medio_pct` | Média do pedal em movimento |
| Tempo de pedal alto | `condutor.pedal_alto_s` | Tempo com motor ligado e `pedal ≥ pedal_alto_pct` |
| Variação do pedal | `condutor.pedal_variacao_p95_pct_s` | p95 de `|Δpedal| / Δt` entre leituras brutas consecutivas, com `Δt` até 3 × intervalo mediano |
| Freio-motor | `condutor.freio_motor_pct` | Tempo com motor ligado, `pedal ≤ freio_motor_pedal_pct`, `rpm ≥ freio_motor_rpm` e `velocidade ≥ freio_motor_kmh`, sobre o tempo em movimento |

Quantis usam interpolação linear entre amostras ordenadas.

### Combustível

`L/h = MAF (g/s) / AFR / densidade (g/L) × 3600`

- **Litros:** `Σ L/h × dt / 3600`, nos pontos com MAF.
- **Acumulados:** litros e distância desde o início da viagem.
- **Cobertura:** fração dos pontos com MAF.
- **Limites:** a estimativa assume mistura estequiométrica e não considera trims, corte de combustível nem enriquecimento (`docs/pids-candidatos-consumo.md`).

### Motor e temperatura

| Elemento | Campo | Cálculo |
|---|---|---|
| Aquecimento | `motor.aquecimento_s` | Tempo do início da viagem até a primeira leitura com `temperatura ≥ aquecido_c`; `null` se não houver |
| Temperatura máxima | `motor.temperatura_maxima_c` | Máximo das leituras na janela |
| Esforço com motor frio | `motor.esforco_frio_s` | Tempo com motor ligado, `temperatura < frio_c` e `rpm > frio_rpm` ou `pedal > frio_pedal_pct` |
| Nível do tanque | `series_lentas.nivel_tanque` | Leituras brutas e mediana móvel de ±3 leituras |

### Qualidade dos dados

| Campo | Cálculo |
|---|---|
| `leituras_total`, `leituras_por_s` | Número de leituras e taxa nos dados das partes |
| `periodos_mediana_ms` | Intervalo mediano por PID |
| `gps.periodo_mediana_s` | Intervalo mediano entre posições |
| `gps.cobertura_pct` | Tempo em intervalos até `gps_falha_s`, sobre a duração da viagem |
| `gps.n_falhas`, `gps.maior_falha_s` | Intervalos acima de `gps_falha_s` e o maior deles |
| `gps.distancia_km` | Soma por fórmula de haversine (R = 6371 km) dos segmentos sem falha |
| `gps.diferenca_distancia_pct` | `(distancia_gps − distancia_obd) / distancia_obd × 100` |
| `combustivel_cobertura_pct` | Cobertura do MAF |

### Avisos

| `codigo` | Condição |
|---|---|
| `sem_gps` | Nenhuma posição na janela da viagem |
| `sem_hora_absoluta` | Entrada `boot` sem conversão para hora absoluta |
| `boot_sem_hora` | Entrada `intervalo` com boot ignorado por não ter conversão; `detalhe` identifica o boot |
| `partes_sobrepostas` | Duas partes se sobrepõem no tempo |
| `sem_leituras` | Nenhuma leitura na viagem |
| `motor_nunca_ligado` | Nenhum ponto com motor ligado |
| `cobertura_maf_baixa` | Cobertura do MAF abaixo de 90% |
| `consumo_pode_estar_subestimado` | `pedal_alto_s / motor_ligado_s > pedal_alto_aviso_fracao` |
| `marcha_lenta_reserva` | Marcha lenta veio do valor de reserva |

### Resumo de um trecho

Quando a requisição traz `de_t_ms` e `ate_t_ms`, a resposta contém só `viagem`, `parametros`, `trecho`, `resumo` e `distribuicoes`.

- Os campos são recalculados com os pontos da grade dentro do trecho.
- Marcha lenta, marchas detectadas e aceleração continuam vindo da viagem inteira, sem nova detecção.
- Um evento conta quando seu início está dentro do trecho.

## Formato JSON

Os valores abaixo são ilustrativos, e os arrays foram encurtados. As séries usam arrays paralelos para reduzir o tamanho da resposta. O exemplo usa a entrada `boot`.

```json
{
  "versao_calculo": "1",
  "viagem": {
    "entrada": { "tipo": "boot", "dongle_id": 1, "boot_generation": 0, "boot_id": 8 },
    "inicio_t_ms": 33481,
    "fim_t_ms": 268453,
    "inicio_em": "2026-10-28T14:03:22-03:00",
    "fim_em": "2026-10-28T14:07:17-03:00",
    "relogio": { "deslocamento_ms": 1793206969000, "incerteza_ms": 180 },
    "partes": [
      {
        "boot_generation": 0,
        "boot_id": 8,
        "dados_inicio_t_ms": 881,
        "dados_fim_t_ms": 268453,
        "inicio_t_ms": 33481,
        "fim_t_ms": 268453
      }
    ]
  },
  "parametros": {
    "combustivel": "gasolina_e30",
    "afr": 13.0,
    "densidade_g_l": 750,
    "marcha_lenta_margem_rpm": 150,
    "gps_falha_s": 5,
    "...": "demais parâmetros efetivos"
  },
  "avisos": [
    { "codigo": "consumo_pode_estar_subestimado" }
  ],
  "resumo": {
    "distancia_km": 3.5,
    "duracao_s": 235.0,
    "sem_dados_s": 0.0,
    "em_movimento_s": 201.0,
    "parado_s": 23.0,
    "velocidade_media_kmh": 63.0,
    "velocidade_maxima_kmh": 131,
    "motor_ligado_s": 230.0,
    "marcha_lenta_s": 23.0,
    "marcha_lenta_pct": 10.0,
    "combustivel_l": 0.83,
    "km_por_l": 4.2,
    "n_acel_bruscas": 2,
    "n_fren_bruscas": 6,
    "acel_por_100km": null,
    "fren_por_100km": null
  },
  "condutor": {
    "pedal_medio_pct": 35.0,
    "pedal_alto_s": 51.0,
    "pedal_variacao_p95_pct_s": 118.0,
    "freio_motor_pct": 39.0
  },
  "motor": {
    "aquecimento_s": 67.0,
    "temperatura_maxima_c": 90,
    "esforco_frio_s": 0.0
  },
  "faixas_velocidade": [
    { "id": "parado", "de_kmh": 0, "ate_kmh": 3, "tempo_s": 23.0, "distancia_km": 0.0, "distancia_pct": null },
    { "id": "ate_30", "de_kmh": 3, "ate_kmh": 30, "tempo_s": 40.0, "distancia_km": 0.35, "distancia_pct": 10.0 },
    { "id": "acima_110", "de_kmh": 110, "ate_kmh": null, "tempo_s": 11.0, "distancia_km": 0.35, "distancia_pct": 10.0 }
  ],
  "rota": {
    "t_ms": [33481, 34480, 35479],
    "lat": [-25.4284, -25.4285, -25.4287],
    "lon": [-49.2733, -49.2735, -49.2738],
    "segmentos": {
      "velocidade_kmh": [24.5, null],
      "faixa": ["ate_30", "falha_gps"]
    }
  },
  "paradas": [
    { "t_inicio_ms": 120000, "duracao_s": 12.0, "motor_ligado": true, "lat": -25.43, "lon": -49.27 }
  ],
  "eventos": [
    {
      "tipo": "frenagem_brusca",
      "t_inicio_ms": 81600,
      "duracao_s": 2.0,
      "pico": -6.8,
      "pico_unidade": "m/s2",
      "velocidade_inicial_kmh": 77,
      "velocidade_final_kmh": 41,
      "lat": -25.43,
      "lon": -49.27
    }
  ],
  "marchas": {
    "marcha_lenta": { "rpm": 640, "origem": "estimada", "margem_rpm": 150 },
    "patamares": [
      { "marcha": 1, "rpm_por_kmh": 66.2, "tempo_s": 21.0, "tempo_pct": 13.0 }
    ],
    "ponto_morto_embreagem_s": 2.0,
    "trocas": [
      {
        "t_ms": 65300,
        "direcao": "subida",
        "de": 1,
        "para": 2,
        "rpm_antes": 2742,
        "rpm_depois": 1760,
        "velocidade_kmh": 41
      }
    ],
    "subida_rpm": { "mediana": 2649, "p25": 2523, "p75": 2750, "n": 14 },
    "reducao_rpm": { "mediana": 1261, "n": 12 }
  },
  "distribuicoes": {
    "rotacao": [
      { "de": 0, "ate": 1000, "tempo_s": 46.0, "pct": 20.0 },
      { "de": 5000, "ate": null, "tempo_s": 0.0, "pct": 0.0 }
    ],
    "pedal": [
      { "de": 0, "ate": 5, "rotulo": "Solto", "tempo_s": 133.0, "pct": 59.0 }
    ],
    "rotacao_pedal": {
      "rpm_bin": 500,
      "pedal_bin": 10,
      "tempo_s": [[0.0, 0.0], [44.0, 2.0]]
    }
  },
  "series": {
    "passo_ms": 150,
    "exposicao_passo_ms": 150,
    "t_ms": [33481, 33631, 33781],
    "velocidade_kmh": [0, 0, null],
    "rpm": [640, 650, 655],
    "rpm_por_kmh": [null, null, null],
    "marcha": [null, null, null],
    "pedal_pct": [0.0, 0.0, 12.5],
    "aceleracao_ms2": [0.0, 0.0, null],
    "carga_pct": [20.0, 20.0, 21.0],
    "consumo_l_h": [1.2, 1.2, 1.3],
    "combustivel_acumulado_l": [0.0, 0.0001, 0.0001],
    "distancia_acumulada_km": [0.0, 0.0, 0.0]
  },
  "series_lentas": {
    "temperatura": { "t_ms": [35000, 40000], "c": [48, 71] },
    "nivel_tanque": { "t_ms": [35000, 45000], "pct": [100.0, 100.0], "mediana_movel_pct": [100.0, 100.0] }
  },
  "qualidade": {
    "leituras_total": 8202,
    "leituras_por_s": 30.65,
    "periodos_mediana_ms": { "0x0C": 130, "0x0D": 130, "0x49": 130, "0x04": 300, "0x10": 300, "0x05": 5000, "0x2F": 10000 },
    "combustivel_cobertura_pct": 99.8,
    "gps": {
      "periodo_mediana_s": 1.0,
      "cobertura_pct": 98.0,
      "n_falhas": 1,
      "maior_falha_s": 8.0,
      "distancia_km": 3.45,
      "diferenca_distancia_pct": -1.4
    }
  }
}
```

Na entrada `intervalo`, muda o bloco `viagem`:

```json
{
  "entrada": {
    "tipo": "intervalo",
    "dongle_id": 1,
    "t0": "2026-10-28T08:00:00-03:00",
    "t1": "2026-10-28T18:00:00-03:00"
  },
  "inicio_t_ms": 21600000,
  "fim_t_ms": 29400000,
  "inicio_em": "2026-10-28T14:00:00-03:00",
  "fim_em": "2026-10-28T16:10:00-03:00",
  "relogio": { "deslocamento_ms": 1793185200000, "incerteza_ms": 240 },
  "partes": [
    {
      "boot_generation": 0,
      "boot_id": 8,
      "dados_inicio_t_ms": 21560000,
      "dados_fim_t_ms": 22800000,
      "inicio_t_ms": 21600000,
      "fim_t_ms": 22800000
    },
    {
      "boot_generation": 0,
      "boot_id": 9,
      "dados_inicio_t_ms": 28150000,
      "dados_fim_t_ms": 29400000,
      "inicio_t_ms": 28200000,
      "fim_t_ms": 29400000
    }
  ]
}
```

### Regras do contrato

- **Avisos:** cada item traz `codigo` e, quando aplicável, `detalhe`.
- **Entre partes:** cada série de `series` e `series_lentas` traz um ponto separador com valores `null` logo após o fim da parte, para o gráfico não ligar partes diferentes. Medianas móveis são calculadas dentro de cada parte.
- **Marcha:** em `series.marcha`, `1..N` é a marcha detectada, `0` é ponto morto ou embreagem e `null` é indefinida.
- **Sem GPS:** `rota` e as posições de `paradas` e `eventos` vêm `null`. `qualidade.gps` traz `cobertura_pct: 0` e os demais campos `null`.
- **Sem hora absoluta:** `inicio_em`, `fim_em` e `relogio` vêm `null`.
- **Sem um PID:** a série correspondente vem preenchida com `null`, e os valores derivados vêm `null`.
- **Limite de pontos:** os cálculos sempre usam a grade completa. Se a grade da viagem passar de `serie_max_pontos`, as séries expostas são reduzidas por amostragem uniforme, e `exposicao_passo_ms` informa o novo passo. Rota, eventos e trocas não são reduzidos.
- **Unidades:** ficam no nome do campo. O campo `pico` usa `pico_unidade`: `m/s2` nos eventos bruscos e `s` em `marcha_lenta_longa`.

## Critérios de aceite

Os testes usam viagens de exemplo com resultado conhecido.

1. **Faixas:** 2,9 km/h cai em `parado`; 29,9 em `ate_30`; 60,0 em `60_80`; 110,0 em `acima_110`.
2. **Distribuição:** as faixas em movimento somam 100% de distância (± arredondamento), e `parado` tem `distancia_pct` igual a `null`.
3. **Cor da rota:** com velocidade do GPS diferente da OBD, a faixa do segmento segue a média OBD.
4. **Falha de GPS:** um intervalo de 8 s gera `falha_gps` com o padrão; com `gps_falha_s = 10`, gera uma faixa de velocidade.
5. **Segmento sem leitura:** usa a última leitura com até 2 s; com mais de 2 s, gera `sem_velocidade`.
6. **Janela da viagem:** leituras com motor desligado antes da partida não entram na duração, mas entram em `viagem.partes[].dados_inicio_t_ms`.
7. **Combustível:** para um total de 0,74 L com gasolina pura, os presets E30 e E32 dão cerca de 0,83 L (0,74 × 14,7 × 745 / (13,0 × 750)). `gasolina_pura` mantém 0,74 L.
8. **Marcha lenta:** a viagem do PDF não gera a falsa "6ª" nem trocas com ela; esse tempo aparece em `ponto_morto_embreagem_s`.
9. **Parâmetro da marcha lenta:** alterar `marcha_lenta_rpm` ou `marcha_lenta_margem_rpm` altera a classificação, e o valor usado volta em `marchas.marcha_lenta`.
10. **Pico sem filtro:** um evento com pico de −10,5 m/s² é exposto com esse valor, sem marcação adicional.
11. **Taxa curta:** com 3,5 km, as taxas por 100 km vêm `null`; com 25 km, vêm preenchidas.
12. **Aviso:** com 23% do tempo em pedal alto, o aviso `consumo_pode_estar_subestimado` aparece.
13. **Trecho:** um trecho recalcula `resumo` e `distribuicoes` sem mudar a numeração das marchas.
14. **Regressão:** com `combustivel = gasolina_pura`, a viagem do PDF mantém 3,5 km, 131 km/h, 0,74 L, 4,7 km/L, 2 acelerações e 6 frenagens bruscas.
15. **Reprodutibilidade:** a mesma entrada, com os mesmos parâmetros e a mesma `versao_calculo`, gera o mesmo resultado.
16. **Entrada inválida:** uma requisição com as duas entradas, sem nenhuma, ou com `t1 ≤ t0` é rejeitada.
17. **Intervalo em um boot:** um intervalo dentro de um boot gera uma parte só com os dados de dentro dele, e `t_ms` conta a partir de `t0`.
18. **Intervalo com dois boots:** gera duas partes em ordem cronológica. A distância e os litros somam as duas, `duracao_s` não inclui o tempo entre elas, e esse tempo aparece em `sem_dados_s`.
19. **Sem cruzar partes:** a aceleração, os eventos e as trocas não usam pontos de partes diferentes, e o segmento de rota entre elas tem faixa `falha_gps`.
20. **Boot sem hora:** na entrada `intervalo`, o boot fica de fora com o aviso `boot_sem_hora`. Na entrada `boot`, a análise funciona normalmente com o aviso `sem_hora_absoluta`.
21. **Marchas na viagem:** em uma viagem com duas partes, a mesma marcha tem o mesmo número nas duas.

## Follow-ups

- **Calibrar a marcha lenta:** ajustar o padrão de 150 rpm de `marcha_lenta_margem_rpm` com viagens reais.
