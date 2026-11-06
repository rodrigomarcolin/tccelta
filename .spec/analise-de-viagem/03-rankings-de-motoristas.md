# 03 — Rankings de motoristas

> **Status: especificação de algo a desenvolver.** Este documento define os rankings, os critérios, os índices e a forma de exibição. Os valores de cada viagem vêm de [`02-calculos-e-contrato-json.md`](02-calculos-e-contrato-json.md). A análise de uma viagem está em [`01-tela-mapa-velocidade.md`](01-tela-mapa-velocidade.md).

## Escopo

A tela compara motoristas em seis rankings, com a mesma estrutura em todos. Cada motorista participa com **uma viagem**, e cada linha mostra o valor dessa viagem e a distância para os demais.

Esta spec não define tecnologia, rotas, autenticação, tabelas nem onde o cálculo é armazenado.

Fora de escopo: nota geral, risco de acidente, limite de velocidade da via, pontos, sequências e desafios.

## Motorista

O projeto ainda não define motorista. Nesta spec, **motorista é um identificador, um nome de exibição e uma viagem**. A forma de criar esse vínculo e de escolher a viagem é um follow-up, e os rankings não dependem dela.

## Premissas

**Combustível único:** o projeto ainda não registra o combustível usado em cada viagem. Por isso, esta versão calcula todas as viagens com o mesmo preset, `ranking_combustivel`, e assume que todos os motoristas usam esse combustível.

- **Quando a premissa vale:** o km/L exibido é a estimativa para o combustível usado.
- **Quando não vale:** o km/L exibido não corresponde ao combustível real do motorista. Por exemplo, 14,2 km/L calculados com gasolina E30 seriam cerca de 10,6 km/L com etanol hidratado, para o mesmo ar admitido.
- **Efeito na posição:** a ordem compara o ar admitido por km, sem considerar o combustível real.

## Princípios

- **Valor real primeiro:** quando existe uma medida única, o ranking mostra essa medida. O índice fica para os rankings que combinam medidas.
- **Maior fica à frente:** nos seis rankings, o maior valor ocupa a melhor posição.
- **Ausência não é zero:** quem não tem dados fica fora do ranking, com o motivo indicado. Nunca aparece no fim da lista com valor zero.
- **Descrição, não avaliação:** o ranking mostra um padrão de uso. Nenhum valor é apresentado como risco ou qualidade do motorista ([2] (2024), [4] (2021)).
- **Limiares são do projeto:** os valores padrão são heurísticas, a calibrar com dados reais.

## Entradas lógicas

| Entrada | Conteúdo |
|---|---|
| Motoristas | `motorista_id`, `nome` e exatamente uma viagem |
| Viagem | Definida como na spec 02, calculada com os parâmetros padrão e a mesma `versao_calculo` |
| Contexto | `todos` (padrão), `cidade` ou `estrada` |

Os rankings não usam GPS nem hora absoluta, então funcionam em viagens sem essas informações. Os Ajustes da tela 01 não alteram os rankings.

## Parâmetros

| Parâmetro | Padrão | Uso |
|---|---|---|
| `ranking_combustivel` | `gasolina_e30` | Preset único para calcular km/L |
| `ranking_cobertura_maf_min_pct` | 90 | Cobertura mínima do MAF na viagem |
| `ranking_estrada_pct` | 50 | % da distância a partir de 80 km/h que classifica a viagem como estrada |
| `ranking_empate_relativo_pct` | 2 | Empate técnico em km/L e trocas/km |
| `ranking_empate_pontos` | 2 | Empate técnico em índices e porcentagens |

## Elegibilidade

**Viagem:** entra nos rankings quando não traz os avisos `sem_leituras` ou `motor_nunca_ligado`. Não há distância mínima.

**Motorista em um ranking:** precisa ter o valor daquele ranking. Caso contrário, fica fora, com o motivo indicado.

**Contexto:** a viagem é `estrada` quando as faixas `80_110` e `acima_110` somam pelo menos `ranking_estrada_pct` da distância em movimento. Caso contrário, é `cidade`. Um motorista cuja viagem é de outro contexto não aparece na lista nem entre os que ficaram fora.

**Limite:** viagens muito curtas podem gerar valores extremos, principalmente nas medidas por distância. Uma frenagem brusca em 500 m, por exemplo, equivale a 200 eventos por 100 km.

## Rankings

| `id` | Nome | Descrição na tela | Valor | Unidade | Destaque de pódio |
|---|---|---|---|---|---|
| `mais_economico` | Mais econômico | Maior distância por litro estimado | km/L | km/L | Sim |
| `cafe_sem_derramar` | Café sem derramar | Pedal e velocidade com menos variações bruscas | Índice | 0–100 | Sim |
| `mais_esportivo` | Mais esportivo | Uso mais intenso do pedal e da rotação | Índice | 0–100 | Não |
| `pe_pesado` | Pé pesado | Maior parte da distância entre 80 e 110 km/h | % da distância | % | Não |
| `piloto_automatico` | Piloto automático | Mais tempo em velocidade constante | % do tempo em movimento | % | Sim |
| `cambio_ligeiro` | Câmbio ligeiro | Mais trocas de marcha por quilômetro | Trocas/km | trocas/km | Não |

### Mais econômico

`valor = resumo.km_por_l`

- **Condições:** `resumo.km_por_l` preenchido e `qualidade.combustivel_cobertura_pct ≥ ranking_cobertura_maf_min_pct`.
- **Combustível:** todas as viagens usam `ranking_combustivel`, conforme a premissa de combustível único.
- **Consumo subestimado:** uma viagem com `consumo_pode_estar_subestimado` fica fora, com o motivo `consumo_subestimado`, porque o km/L dela pode parecer melhor do que foi.
- **Limite:** a estimativa não considera corte de combustível nem enriquecimento (spec 02).

### Café sem derramar

Média dos pontos **invertidos** de:

| Componente | Valor |
|---|---|
| `aceleracao_rms_ms2` | `condutor.aceleracao_rms_ms2` |
| `pedal_variacao_p95_pct_s` | `condutor.pedal_variacao_p95_pct_s` |
| `eventos_bruscos_por_100km` | `(resumo.n_acel_bruscas + resumo.n_fren_bruscas) × 100 / resumo.distancia_km` |

Os eventos por 100 km são calculados aqui porque `resumo.acel_por_100km` e `resumo.fren_por_100km` vêm `null` em viagens abaixo de `taxa_min_km`.

Limite: o trânsito também influencia a suavidade.

### Mais esportivo

Média dos pontos de:

| Componente | Valor |
|---|---|
| `pedal_variacao_p95_pct_s` | `condutor.pedal_variacao_p95_pct_s` |
| `pedal_alto_pct` | `condutor.pedal_alto_s / resumo.motor_ligado_s × 100` |
| `rotacao_alta_pct` | Tempo nas faixas de `distribuicoes.rotacao` a partir de 3500 rpm, sobre `resumo.motor_ligado_s`, × 100 |
| `subida_rpm` | `marchas.subida_rpm.mediana` |
| `acel_bruscas_por_100km` | `resumo.n_acel_bruscas × 100 / resumo.distancia_km` |

O índice não usa velocidade, para não premiar quem anda mais rápido. Picos de aceleração também ficam de fora, porque são expostos sem filtro na spec 02.

### Pé pesado

`valor = distancia_pct da faixa 80_110 em faixas_velocidade`

A faixa `acima_110` não conta no numerador, para o ranking não premiar velocidade mais alta. A tela sempre mostra a descrição do critério ao lado do nome. O filtro de contexto fica desativado neste ranking, porque a classificação de estrada usa a mesma medida.

### Piloto automático

`valor = condutor.cruzeiro_s / resumo.em_movimento_s × 100`

O cálculo e os parâmetros de `condutor.cruzeiro_s` estão na spec 02. O nome do ranking descreve o padrão de condução, e o projeto não detecta o uso de piloto automático no veículo.

### Câmbio ligeiro

`valor = número de marchas.trocas / resumo.distancia_km`

A viagem precisa ter pelo menos dois `marchas.patamares`. Caso contrário, fica fora, com o motivo `sem_marchas`, porque não teria como registrar trocas. A contagem soma subidas e reduções. Mais trocas não significam condução melhor nem pior.

## Índices

### Pontos de cada componente

`pontos = limitar((valor − p5) / (p95 − p5), 0, 1) × 100`

Em "Café sem derramar", `pontos_invertidos = 100 − pontos`.

- O **índice** é a média simples dos pontos, com o mesmo peso para todos os componentes.
- Um componente fica disponível para o motorista quando a viagem tem aquele dado.
- O índice exige pelo menos dois terços dos componentes disponíveis: 2 de 3 em "Café sem derramar" e 4 de 5 em "Mais esportivo". Caso contrário, o motorista fica fora, com o motivo `indice_incompleto`.
- Um componente ausente aparece como "sem dados" no detalhamento.
- Se `p95 = p5` em um componente, ele fica indisponível para todos.

Os percentis puros não são usados para os pontos, porque espalham os valores de forma quase uniforme e escondem a distância real entre os motoristas.

### Referência

`p5` e `p95` vêm dos valores das viagens de todos os motoristas elegíveis, sem filtro de contexto, com interpolação linear (spec 02).

**Limites:**

- **Índice relativo ao grupo:** a entrada ou a saída de um motorista pode mudar o índice dos demais e até a ordem entre eles.
- **Poucos motoristas:** com poucos valores, `p5` e `p95` ficam perto do menor e do maior valor. Com dois motoristas, cada componente vira 0 ou 100 pontos, mesmo que a diferença real seja pequena. Por isso, o detalhamento sempre mostra o valor real de cada componente.
- **Um motorista:** não há referência, e os rankings de índice não o incluem.

## Posição e diferenças

| Campo | Cálculo |
|---|---|
| `posicao` | Ordem decrescente do valor sem arredondar. Valores exatamente iguais dividem a posição, e a seguinte é pulada (1, 2, 2, 4) |
| `diferenca_lider` | `valor − valor do 1º`, a partir do 2º lugar |
| `vantagem_seguinte` | `valor − valor do 2º`, só para o 1º lugar |
| `diferenca_lider_l_100km` | `100 / valor − 100 / valor do 1º`, só em `mais_economico` |
| `empate_tecnico_anterior` | Compara com a linha anterior: até `ranking_empate_relativo_pct` do maior valor em km/L e trocas/km, ou até `ranking_empate_pontos` em índices e porcentagens |

O empate técnico é um rótulo e não muda a posição.

## Exibição

```text
+----------------------------------------------------------------------------------+
| Rankings de motoristas                                             [Contexto ▾]  |
| [Mais econômico] [Café sem derramar] [Mais esportivo] [Pé pesado]                |
| [Piloto automático] [Câmbio ligeiro]                                             |
| Maior distância por litro estimado · gasolina E30 para todos                     |
+----------------------------------------------------------------------------------+
| 1º  Motorista A · 28/10 · 12,4 km      ████████████████████  14,2 km/L estimado  |
|     0,6 km/L à frente do 2º                                                      |
| 2º  Motorista B · 25/10 · 18,0 km      ███████████████████   13,6 km/L estimado  |
|     −0,6 km/L do 1º · 0,3 L/100 km a mais                                        |
| 3º  Motorista C · 21/10 · 9,7 km       ███████████████████   13,5 km/L estimado  |
|     −0,7 km/L do 1º · empate técnico com o 2º                                    |
| 4º  Motorista D · 19/10 · 6,1 km       ███████████████       11,0 km/L estimado  |
|     −3,2 km/L do 1º · 2,0 L/100 km a mais                                        |
+----------------------------------------------------------------------------------+
| 3 motoristas fora deste ranking · ver motivo                                     |
+----------------------------------------------------------------------------------+
```

Os números do wireframe são ilustrativos.

### Estrutura

- **Abas:** há uma aba por ranking, e trocar de aba mantém o contexto.
- **Descrição:** uma frase sob as abas explica o critério do ranking aberto.
- **Linha:** mostra posição, nome, data da viagem (quando disponível), distância, barra, valor e diferença.
- **Barra:** começa em zero e é proporcional ao valor do 1º lugar. Os índices usam a mesma regra na escala de 0 a 100.
- **Cor:** a barra usa um só tom, sem verde, amarelo ou vermelho. O texto nunca depende da cor.
- **Pódio:** os três primeiros recebem destaque visual só nos rankings marcados na tabela. Nos demais, a lista continua ordenada, sem medalha ou texto de vitória.

### Texto da diferença

| Situação | Texto |
|---|---|
| 1º lugar | `0,6 km/L à frente do 2º` |
| Demais lugares | `−0,6 km/L do 1º` |
| `mais_economico` | Acrescenta `0,3 L/100 km a mais` |
| Porcentagens | Usa pontos percentuais: `−12 p.p. do 1º` |
| Índices | Usa pontos: `−8 pontos do 1º` |
| Empate técnico | Acrescenta `empate técnico com o 2º`, citando a posição anterior |

### Detalhe do motorista

Selecionar uma linha expande o detalhe, com:

- valores usados no cálculo, como litros e distância;
- nos índices, cada componente com valor e pontos, por exemplo `Índice 78 = variação do pedal 85 · pedal alto 70 · rotação alta 72 · rotação nas subidas 80 · acelerações bruscas 83`;
- acesso à viagem na tela 01.

### Filtro

**Contexto:** todos, cidade ou estrada. Fica desativado em "Pé pesado", com a explicação do motivo.

### Estados da tela

| Estado | Comportamento |
|---|---|
| Carregando | Mantém abas e filtro, com áreas de espera na lista |
| Erro ao carregar | Mensagem clara e opção de tentar de novo |
| Nenhum motorista elegível | "Nenhum motorista com dados suficientes" e o link dos motivos |
| Um motorista elegível | Mostra a linha sem barra comparativa nem diferença. Nos rankings de índice, mostra "É preciso mais de um motorista para calcular este índice" |
| Veículos diferentes | Aviso: "Este ranking compara veículos diferentes. Consumo, rotação e trocas também dependem do carro" |

## Regras de conteúdo

- **Consumo:** todo km/L ou litro traz "estimado" e o combustível usado.
- **Combustível único:** a descrição de "Mais econômico" deixa claro que o mesmo combustível foi considerado para todos os motoristas.
- **Índice:** sempre "índice", nunca "nota".
- **Linguagem:** nenhum texto usa "nota", "risco", "infração", "excesso", "pior" ou "último".
- **Pé pesado:** o nome nunca aparece sem a descrição do critério.
- **Limiares:** a seção de metodologia os identifica como limiares do projeto.

## Avisos e motivos

| `codigo` | Condição |
|---|---|
| `varios_veiculos` | As viagens dos motoristas exibidos vêm de mais de um `dongle_id` |

| `motivo` | Quem fica fora do ranking |
|---|---|
| `sem_dados` | Viagem sem o valor do ranking, inclusive por distância zero nas medidas por km ou pelos avisos `sem_leituras` e `motor_nunca_ligado` |
| `cobertura_maf_baixa` | Cobertura do MAF abaixo de `ranking_cobertura_maf_min_pct` |
| `consumo_subestimado` | Viagem com o aviso `consumo_pode_estar_subestimado` |
| `sem_marchas` | Viagem com menos de dois patamares de marcha |
| `indice_incompleto` | Componentes disponíveis abaixo do mínimo |

## Formato JSON

Os valores são ilustrativos, e os arrays foram encurtados.

```json
{
  "versao_calculo": "1",
  "contexto": "todos",
  "parametros": {
    "ranking_combustivel": "gasolina_e30",
    "ranking_cobertura_maf_min_pct": 90,
    "...": "demais parâmetros efetivos"
  },
  "avisos": [{ "codigo": "varios_veiculos" }],
  "referencia": {
    "n_viagens": 7,
    "componentes": {
      "aceleracao_rms_ms2": { "p5": 0.4, "p95": 1.3 },
      "pedal_variacao_p95_pct_s": { "p5": 40.0, "p95": 130.0 }
    }
  },
  "rankings": [
    {
      "id": "mais_economico",
      "unidade": "km/L",
      "combustivel": "gasolina_e30",
      "entradas": [
        {
          "posicao": 1,
          "motorista": { "id": "a", "nome": "Motorista A" },
          "viagem": {
            "entrada": { "tipo": "boot", "dongle_id": 1, "boot_generation": 0, "boot_id": 8 },
            "inicio_em": "2026-10-28T14:03:22-03:00"
          },
          "valor": 14.2,
          "distancia_km": 12.4,
          "combustivel_l": 0.87,
          "diferenca_lider": null,
          "vantagem_seguinte": 0.6,
          "diferenca_lider_l_100km": null,
          "empate_tecnico_anterior": false,
          "componentes": null
        }
      ],
      "fora": [
        { "motorista": { "id": "e", "nome": "Motorista E" }, "motivo": "consumo_subestimado" }
      ]
    },
    {
      "id": "mais_esportivo",
      "unidade": "indice",
      "combustivel": null,
      "entradas": [
        {
          "posicao": 1,
          "motorista": { "id": "b", "nome": "Motorista B" },
          "viagem": {
            "entrada": { "tipo": "boot", "dongle_id": 2, "boot_generation": 0, "boot_id": 3 },
            "inicio_em": "2026-10-25T08:10:00-03:00"
          },
          "valor": 78.0,
          "distancia_km": 18.0,
          "diferenca_lider": null,
          "vantagem_seguinte": 8.0,
          "empate_tecnico_anterior": false,
          "componentes": [
            { "id": "pedal_alto_pct", "valor": 14.0, "pontos": 70.0 },
            { "id": "subida_rpm", "valor": null, "pontos": null }
          ]
        }
      ],
      "fora": []
    }
  ]
}
```

### Regras do contrato

- **Sem arredondar:** a API não arredonda valores, e a posição usa o valor completo.
- **Ordem:** `entradas` já vem ordenada, e empates exatos usam `motorista.id` como desempate da ordem de exibição.
- **Hora ausente:** sem conversão para hora absoluta, `viagem.inicio_em` vem `null`, e a linha omite a data.
- **Unidades:** ficam em `unidade`, com `km/L`, `indice`, `pct` ou `trocas/km`.

## Critérios de aceite

1. **Abas:** os seis rankings aparecem, e trocar de aba mantém o contexto.
2. **Uma viagem:** uma entrada com um motorista sem viagem, ou com mais de uma, é rejeitada.
3. **Sem mínimos:** uma viagem de 1 km com todos os dados entra nos rankings.
4. **Combustível:** mudar o combustível nos Ajustes da tela 01 não altera nenhum ranking.
5. **Consumo subestimado:** uma viagem com o aviso fica fora de `mais_economico`, com o motivo `consumo_subestimado`, e continua nos demais rankings.
6. **Pontos:** com `p5 = 2` e `p95 = 6`, o valor 4 dá 50 pontos, e o valor 7 dá 100. Invertido, o valor 3 dá 75.
7. **Referência:** com os valores 10 e 20, `p5 = 10,5` e `p95 = 19,5`, e os pontos são 0 e 100.
8. **Índice incompleto:** em "Mais esportivo", faltar um componente mantém o motorista, e faltarem dois o deixa de fora.
9. **Pé pesado:** com 30 km em `80_110`, 20 km em `acima_110` e 50 km nas demais faixas em movimento, o valor é 30%.
10. **Piloto automático:** com `cruzeiro_s = 60` e `em_movimento_s = 200`, o valor é 30%.
11. **Câmbio:** uma viagem com um só patamar fica fora de `cambio_ligeiro`, com o motivo `sem_marchas`.
12. **Empate técnico:** 13,6 e 13,5 km/L recebem o rótulo. 14,2 e 13,6 km/L não recebem.
13. **Empate exato:** dois valores iguais em 2º lugar geram as posições 1, 2, 2 e 4.
14. **Ausência:** um motorista sem dados aparece em "ver motivo", e nunca na lista com zero.
15. **Barras:** um valor com metade do valor do líder tem metade da largura da barra do líder.
16. **Um motorista:** os rankings de valor real mostram a linha sem comparação, e os de índice mostram a mensagem de falta de motoristas.
17. **Filtro:** o contexto fica desativado em "Pé pesado" e volta ao trocar de aba.
18. **Filtro e índice:** mudar o contexto não altera `p5`, `p95` nem o índice de quem continua na lista.
19. **Vários veículos:** viagens de dois dongles geram o aviso `varios_veiculos`.
20. **Pódio:** "Mais esportivo", "Pé pesado" e "Câmbio ligeiro" não mostram medalha nem texto de vitória.
21. **Linguagem:** nenhum texto usa os termos proibidos em "Regras de conteúdo".
22. **Acessibilidade:** abas, detalhe e "ver motivo" funcionam por teclado, e cada valor existe como texto, além da barra.
23. **Tela estreita:** as abas e a lista cabem sem rolagem horizontal da página.
24. **Reprodutibilidade:** a mesma entrada, com os mesmos parâmetros e a mesma `versao_calculo`, gera o mesmo resultado.
25. **Combustível único:** duas viagens com a mesma distância e o mesmo ar admitido têm o mesmo km/L, mesmo que os combustíveis reais sejam diferentes, e a tela informa que o mesmo combustível foi considerado para todos.

## Follow-ups

- **Definir motorista:** criar o cadastro, o vínculo entre motorista e viagem e a forma de escolher a viagem. A especificação do servidor já indica que dongle não é motorista.
- **Registrar o combustível:** guardar o combustível de cada viagem e separar os rankings quando houver combustíveis diferentes, encerrando a premissa de combustível único.
- **Comparar por veículo:** agrupar por veículo ou modelo, porque rotação e marchas dependem do carro. Até lá, vale o aviso `varios_veiculos`.
- **Estabilizar os índices:** avaliar uma referência fixa, com viagens de fora do ranking, para o índice não depender do grupo comparado.
- **Calibrar limiares:** ajustar o empate técnico, a cobertura do MAF e a classificação de estrada com viagens reais.
