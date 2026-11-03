# 01 — Tela de análise de viagem com mapa colorido por velocidade

> **Status: especificação de algo a desenvolver.** Este documento define somente o design da tela: estrutura, informações exibidas, estilo, interações e estados. Os cálculos e o formato dos dados estão em [`02-calculos-e-contrato-json.md`](02-calculos-e-contrato-json.md).

## Escopo

A tela fará parte de um frontend integrado, publicado no futuro. Esta spec não define tecnologia, bibliotecas, provedor de mapa-base nem forma de publicação. A ferramenta local `tools/trip-analyzer` e o PDF `Análise de Viagem - volante.pdf` servem apenas de referência para os gráficos; a tela não herda o formato nem a forma de entrada de dados dessa ferramenta.

Fora de escopo: limite de velocidade da via, tipo de via, curvas, colorir o mapa por outra métrica, análise de altitude, novos PIDs, nota de motorista e mascaramento de início e fim da viagem ao compartilhar.

Referências `[n] (ano)` seguem `docs/referencias-telemetria-pids.md`.

## Objetivo e público

Mostrar uma viagem do dongle em ordem do mais mastigado ao mais detalhado, para três leitores:

| Leitor | Pergunta | Onde responde |
|---|---|---|
| Quem só quer o resumo | Onde fui, a que velocidade, quanto gastei? | Cabeçalho e Resumo |
| Quem quer entender a condução | Onde freei, acelerei, parei? Que marcha e rotação usei? | Linha do tempo, Marchas e rotação, Eventos |
| Quem audita os dados | Os dados são confiáveis? Quais são os limites do método? | Qualidade dos dados e metodologia |

A tela não avalia o motorista. Ela mostra o que o veículo fez.

## Layout

Ordem fixa, de cima para baixo. Em telas estreitas, o mapa fica acima dos indicadores, e os gráficos lado a lado passam a uma coluna.

```
+----------------------------------------------------------------------------------+
| Análise de viagem   Dongle 1 · boot 8     28/10/2026 14:03–14:07        [Ajustes] |  1 Cabeçalho
+----------------------------------------------------------------------------------+
| +-----------------------------+  USO                                              |
| |                             |  Distância 3,5 km · Duração 3 min 55 s            |
| |           M A P A           |  Em movimento 3 min 21 s · 14:03–14:07            |
| |     (A)-----------(B)       |  EFICIÊNCIA (estimada · gasolina E30)             |
| |   rota colorida por vel.    |  4,2 km/L · 0,83 L · Marcha lenta 10% (23 s)      |
| |                             |  CONDUÇÃO                                         |
| +-----------------------------+  Média 63 km/h · Máxima 131 km/h                  |
|  <30  30-60  60-80  80-110  >110   Frenagens bruscas 6 · Acelerações bruscas 2    |  2 Resumo
|  10%  23%    31%    26%     10%   (% da distância) · Parado: 23 s                 |
+----------------------------------------------------------------------------------+
| Velocidade  ~~~~/\~~~~~~/\/\~~~  (marcadores de evento)                           |
| Rotação     ~~~~~~~~~~~~~~~~~~~  (pontos: subida / redução)                       |  3 Linha do tempo
| Marcha      ___|^^^|___|^^^^^^|  (degraus)                                        |    (mesmo eixo X)
| Pedal       ~~~~~~~~~~~~~~~~~~~                                                   |
+----------------------------------------------------------------------------------+
| Marchas e rotação: rotação x velocidade · tempo por marcha · rotação nas trocas   |
|   rotação x pedal · posição do pedal · faixas de rotação                          |  4
+----------------------------------------------------------------------------------+
| Combustível e motor: consumo instantâneo · combustível acumulado · carga ·        |
|   temperatura · nível do tanque                                                   |  5
+----------------------------------------------------------------------------------+
| Eventos: tipo | hora | duração | pico | velocidade | [ver no mapa]                |  6
+----------------------------------------------------------------------------------+
| > Qualidade dos dados e metodologia (recolhido)                                   |  7
+----------------------------------------------------------------------------------+
```

Os números do wireframe são ilustrativos. As porcentagens por faixa e os horários são exemplos.

### 1. Cabeçalho

Identificação da viagem (dongle e boot), data e hora quando disponíveis, alternância de tema e o botão **Ajustes**.

### 2. Resumo

- **Mapa à esquerda**, com a rota colorida por velocidade (seção "Mapa").
- **Legenda abaixo do mapa, que também é distribuição:** cada faixa em movimento mostra a % da **distância** percorrida nela. "Parado" fica fora da soma e mostra o **tempo** parado.
- **À direita, três grupos:**

| Grupo | Indicadores |
|---|---|
| Uso | Distância, duração, tempo em movimento, horário |
| Eficiência | km/L, litros, % de marcha lenta |
| Condução | Velocidade média e máxima, frenagens e acelerações bruscas |

### 3. Linha do tempo

Painéis empilhados com o **mesmo eixo X** e cursor compartilhado, sem eixo Y duplo:

1. Velocidade, com marcadores de evento (frenagem e aceleração, com formas diferentes).
2. Rotação, com pontos de subida e de redução de marcha.
3. Marcha estimada, em degraus. O ponto morto ou a embreagem têm rótulo próprio, fora da numeração das marchas. Marcha indefinida fica vazia.
4. Pedal.

### 4. Marchas e rotação

- **Indicadores de apoio:** rotação mediana ao subir e ao reduzir marcha, pedal médio em movimento, % de freio-motor e variação do pedal (p95).
- **Rotação × velocidade:** dispersão colorida por marcha, com a reta e o rótulo de cada marcha. Os pontos de ponto morto ou embreagem ficam em cinza e sem reta.
- **Tempo em cada marcha:** barras horizontais com tempo e %, com uma linha separada para ponto morto ou embreagem.
- **Rotação nas trocas:** pontos por tipo de troca, separados em subidas e reduções, com traço na mediana. Clicar em um ponto o localiza na linha do tempo. A tabela de trocas fica recolhida.
- **Rotação × pedal:** mapa de calor de tempo, em tom sequencial de um só matiz.
- **Posição do pedal e faixas de rotação:** barras horizontais com % e tempo.

### 5. Combustível e motor

- **Indicadores de apoio:** tempo até o motor aquecer, temperatura máxima e tempo de esforço com motor frio.
- **Gráficos:** consumo instantâneo, combustível acumulado, carga do motor, temperatura do líquido (com linha de referência de aquecimento) e nível do tanque (leitura e mediana móvel).
- O consumo médio acumulado não aparece em destaque, porque oscila muito no início da viagem.

### 6. Eventos

Tabela de eventos (frenagem brusca, aceleração brusca, marcha lenta longa) com tipo, hora, duração, pico, velocidade inicial e final e a ação **"ver no mapa"**.

### 7. Qualidade dos dados e metodologia

Seção recolhida por padrão, com:

- identificação do boot e duração total desde o boot;
- períodos medianos dos PIDs e leituras por segundo;
- razão rotação/velocidade ao longo do tempo e por marcha identificada;
- cobertura de GPS, número de falhas e maior falha;
- distância pela velocidade OBD versus distância pelo GPS, com a diferença;
- combustível usado na estimativa (AFR e densidade) e cobertura do MAF;
- rotação de marcha lenta usada e sua origem (estimada ou informada);
- parâmetros e limiares usados, identificados como **limiares do projeto**;
- limites do método, incluindo o atraso do GPS.

### Ajustes

Painel (modal ou gaveta) aberto pelo botão **Ajustes**, com:

- **Combustível:** gasolina E30 (padrão), gasolina E32, gasolina E27, gasolina pura, etanol hidratado, diesel ou personalizado (AFR e densidade).
- **Avançado:** rotação de marcha lenta (automática ou informada), margem da marcha lenta, limiar de falha de GPS e limiares de eventos.

Ao alterar um ajuste, a tela recalcula e mostra o valor usado na seção de qualidade. Os padrões estão na spec 02.

## Mapa

### Rota

A cor da rota mostra a velocidade do veículo em cada trecho. O rótulo de cada faixa e sua cor são:

| Faixa | Rótulo | Tema claro | Tema escuro |
|---|---|---|---|
| `parado` | Parado | Cinza neutro | Cinza neutro |
| `ate_30` | < 30 km/h | `#e8934a` | `#8c3326` |
| `30_60` | 30–60 km/h | `#d9703a` | `#b44d2c` |
| `60_80` | 60–80 km/h | `#be5030` | `#d96a31` |
| `80_110` | 80–110 km/h | `#983528` | `#ec8e42` |
| `acima_110` | > 110 km/h | `#6a1f22` | `#f7b878` |

As faixas são fixas, para permitir comparar viagens. Elas **não** representam o limite de velocidade da via, e a interface **não fala em infração**.

### Estilo

- Escala de calor de laranja a vinho, sobre mapa-base em cinza, com **contorno branco** na rota.
- **Proibido verde-amarelo-vermelho:** sugere certo e errado, e a cor mostra velocidade, não infração.
- No tema escuro a ordem inverte: a faixa mais rápida é a mais clara, para continuar sendo a de maior destaque contra o fundo.
- As duas rampas passaram no validador ordinal do skill `dataviz` (um só matiz e contraste mínimo de 2:1) contra as superfícies `#fcfcfb` e `#1a1a19`. O contraste sobre o mapa-base ainda precisa ser verificado quando ele for escolhido.
- **Texto nunca usa a cor da série.** Há uma tabela alternativa (faixa, % da distância e tempo), acessível pela legenda.

### Marcadores

| Elemento | Representação |
|---|---|
| Início e fim | Marcadores **A** e **B**, com texto |
| Parada | Marcador neutro (cinza) com **tamanho conforme a duração** |
| Frenagem brusca | Ícone próprio (por exemplo, triângulo para baixo) |
| Aceleração brusca | Ícone diferente (por exemplo, triângulo para cima) |

Eventos não dependem só da cor: forma e rótulo acessível distinguem os tipos.

### Trechos sem dados

Trechos com falha de GPS ou sem velocidade aparecem como **linha cinza fina**, sem cor de faixa.

## Interações

| Ação do usuário | Resultado |
|---|---|
| Mouse sobre a rota | Cursor compartilhado nos gráficos e **tooltip** com hora, velocidade, rotação, marcha e pedal |
| Mouse sobre um gráfico | O ponto correspondente aparece no mapa |
| Arrastar na linha do tempo | Destaca o trecho no mapa, **esmaece o resto** e **atualiza o resumo** para o trecho; aviso "Trecho m:ss–m:ss · limpar" |
| "Ver no mapa" em um evento | Centraliza o evento no mapa e marca o instante na linha do tempo |
| Duplo clique na linha do tempo | Volta a mostrar a viagem inteira |

A tabela de eventos, o botão "limpar" e os Ajustes também funcionam por teclado.

## Estados da tela

| Estado | Comportamento |
|---|---|
| Carregando | Estrutura da página com áreas de espera; ao atualizar um trecho, o resumo anterior fica esmaecido até o novo chegar |
| Erro ao carregar | Mensagem clara e opção de tentar de novo |
| Sem nenhum GPS | Sem mapa; o Resumo ocupa a largura total, com o aviso "Sem posições de GPS nesta viagem: a rota não pode ser mostrada". As outras seções funcionam normalmente |
| GPS com falhas | Mapa com os trechos em cinza |
| Mapa-base indisponível | A rota aparece sobre fundo liso cinza |
| Sem hora absoluta | O cabeçalho não mostra data nem hora, e o eixo mostra o tempo desde o início da viagem |
| Sem leituras de um sinal | O gráfico mostra "Sem leituras" no lugar de valores zerados |

## Regras de conteúdo

- **Consumo e litros** sempre com o rótulo **"estimado"** e o combustível usado ao lado (por exemplo, "0,83 L estimado · gasolina E30").
- **Eventos em contagem bruta.** A taxa por 100 km só aparece quando a spec 02 a fornece.
- **Sem nota de motorista.** Os escores da literatura de seguro são ajustados com dados de acidentes, que o projeto não tem ([2] (2024), [4] (2021)). Nenhum número da tela é apresentado como risco.
- **Aviso de possível subestimativa** do combustível quando a API sinalizar muito pedal alto: "Consumo pode estar subestimado: houve muito tempo com pedal alto".
- **Limiares de evento** são do projeto. A interface não os apresenta como padrão da literatura.

## Critérios de aceite

1. **Hierarquia:** o mapa e o resumo aparecem antes dos gráficos, sem necessidade de rolagem em uma tela de desktop comum.
2. **Sem GPS:** não há mapa, o Resumo ocupa a largura total, o aviso aparece, e as seções 3 a 7 funcionam.
3. **Com GPS:** a rota aparece com as cores da faixa recebida e os marcadores A e B.
4. **Trechos sem dados:** aparecem como linha cinza fina, sem cor de faixa.
5. **Legenda:** as faixas em movimento somam 100% (± arredondamento), e "Parado" mostra tempo.
6. **Sem hora absoluta:** o cabeçalho omite data e hora, e não há erro.
7. **Rótulos de consumo:** todo valor de litros ou km/L tem "estimado" e o combustível ao lado.
8. **Marcha:** ponto morto ou embreagem aparece com rótulo próprio, sem número de marcha e sem marcador de troca.
9. **Interações:** todas as ações da tabela funcionam, e "limpar" restaura a viagem inteira.
10. **Ajustes:** mudar o combustível atualiza os valores e o rótulo em toda a tela.
11. **Modo escuro:** rota, legenda, contorno e marcadores são legíveis, com as cores da coluna "Tema escuro".
12. **Impressão:** a página usa tema claro, o mapa e o resumo cabem na primeira página, a legenda mostra nomes e porcentagens, e a seção de qualidade sai aberta.
13. **Mapa-base indisponível:** a rota aparece sobre fundo liso, sem mensagem de erro cobrindo a tela.
14. **Linguagem:** nenhum texto usa "infração", "excesso" ou "nota", e a rampa não usa verde nem vermelho puro.
15. **Tela estreita:** o mapa fica acima dos indicadores, sem rolagem horizontal da página.
