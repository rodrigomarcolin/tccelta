# PIDs candidatos para melhorar a estimativa de consumo

Nenhum dos PIDs desta página é coletado hoje, e a recomendação **não é adicioná-los sem antes medir**. O documento reúne o que cada um acrescentaria, quanto custaria no barramento e em que condição valeria a pena.

Referências `[n] (ano)` seguem a numeração de `docs/referencias-telemetria-pids.md`. Fontes sem número têm o ano ao lado do nome. O que não vem de fonte está marcado como "inferência do projeto". Números de PID e fatores de decodificação vêm de memória de SAE J1979 e estão marcados "a conferir". O "PDF de referência" é `Análise de Viagem - volante.pdf`, a análise de uma viagem de teste.

Base do consumo hoje: combustível (g/s) = MAF (`0x10`) ÷ relação ar/combustível estequiométrica; litros = g ÷ densidade. É o método de [11] (2022) quando não há taxa de combustível, sem os trims.

## 1. Trims de combustível

PIDs (a conferir em SAE J1979): `0x06` STFT banco 1, `0x07` LTFT banco 1; `0x08` e `0x09` para o banco 2 em motores em V.

**O que são.** Correções percentuais que a ECU aplica à injeção com base na sonda lambda. Valor positivo adiciona combustível; negativo retira. Decodificação (a conferir): `raw × 100/128 − 100`, em %.

**Fórmula do VED [11] (2022).** `correction = (1 + STFT/100 + LTFT/100) / AFR`. Ou seja, o combustível estimado é o MAF multiplicado por essa correção.

**Ganho.** Não há estudo que quantifique o ganho isolado dos trims para este caso. Um limite simples: trims que somem +5% mudam a estimativa em 5%.

Evidências encontradas, com os limites de cada uma:

- **Patente US 7774130 (2010), inventor Gary Thomas Pepper.** Descreve o cálculo de consumo com e sem trims. Na Tabela 1, um ensaio em dinamômetro no ciclo US06 dá 8,76 L/100 km em laboratório, 8,51 sem trims (−2,91%) e 8,71 com trims (−0,56%), segundo o texto da patente. Recalculando a partir da tabela, os erros dão −2,85% e −0,57%, então a diferença é de arredondamento. A patente conclui que os trims têm papel importante na precisão. É uma patente, sem revisão por pares, com um único ensaio: não vale como validação geral. Lido no Google Patents por ferramenta de resumo; conferir no original antes de citar.
- **Estudo CSME 2022 da Universidade de Alberta (Ansari et al., 2022).** Estima consumo instantâneo com aprendizado de máquina e dados OBD (Ford Escape PHEV e F-350, rota de 100 km). Calcula a relação de mistura pelos trims, em vez do valor comandado pela ECU. No texto, a estimativa só pelo MAF é "aceitável" na estequiometria e "imprecisa" fora dela, com base em DeFries et al. (2014), que compararam a estimativa por MAF com balanço de carbono. Leitura parcial (resumo, introdução, trecho do cálculo e conclusões).
- O socket do BeamNG (`esp32-firmware/beamng/socket/README.md`) tem trims modelados (±3% de oscilação no `0x06`, +1,5% no `0x07`). Portanto **não serve para validar o ganho**: o valor é parâmetro da simulação, não medição.

**Quando é útil.** Quando há desvio persistente do MAF. É o principal ganho do LTFT.

**O que não resolve (inferência do projeto):**

- corte de combustível na desaceleração;
- enriquecimento em malha aberta (aceleração forte), em que a ECU deixa de corrigir pela sonda.

**Intervalo adequado (inferência do projeto; nenhuma fonte define essa taxa).**

| Trim | Intervalo | Motivo |
|---|---|---|
| `0x06` (STFT) | 600 ms | É o período do MAF, que ele corrige; permite corrigir também o consumo instantâneo |
| `0x07` (LTFT) | 10 s | Muda devagar; uma leitura mais frequente acrescenta pouco |

Espera-se que o `0x07` corrija o desvio persistente e que o `0x06` melhore mais o valor instantâneo que o total da viagem (inferência do projeto). O custo no barramento está na seção 3.

**Disponibilidade.** Nenhum estudo com percentuais de suporte foi encontrado. Tendem a existir em veículos a gasolina e flex e a não existir em diesel (conhecimento do padrão, não medido).

## 2. Relação ar/combustível comandada

PID (a conferir): `0x44`, lambda comandado pela ECU. Decodificação (a conferir): `raw × 2/65536`.

**O que é.** O λ que a ECU **comanda**, não o medido. Correção: `combustível = MAF ÷ (AFR estequiométrico × λ)`.

**O que não acrescenta.** Mapa, distância, eventos, marchas, rotação, pedal e qualquer uso em seguro. Só agrega no consumo em aceleração forte.

**Tamanho do efeito.** Com λ de 0,85 a 0,91, o combustível real é de 10% a 18% maior nesses momentos (faixa do arXiv 2009.07872, Ard et al., 2020; visto apenas em trecho de busca, não confirmado no texto). Cálculo: 1/0,91 − 1 = 9,9% e 1/0,85 − 1 = 17,6%.

Exemplos **hipotéticos**, com λ = 0,88 (fator 1/0,88 = 1,136), em que uma fração do combustível é queimada em mistura rica:

| Parte do combustível em mistura rica | Subestimativa do combustível total sem o `0x44` |
|---|---|
| 5% | menos de 1% (cerca de 0,7%) |
| 20% | cerca de 3% (2,7%) |
| 60% | cerca de 8% (7,6%) |

Não medido no projeto. No PDF de referência o pedal fica 23% do tempo em 80–100%, mas pedal alto não garante enriquecimento.

**Limites.**

- O suporte é incerto. Não se sabe se o valor comandado representa o corte de combustível, e isso pode variar por ECU. No corte, o erro absoluto tende a ser pequeno, porque passa pouco ar pelo MAF; o enriquecimento deve pesar mais (inferência do projeto).

## 3. Orçamento de barramento

O escalonador exige `U = Σ Tmax/Pᵢ ≤ 0,8` (`kMaxUtilization`, teste `<=`). `AT ST hh` altera só `tmaxMs` (passo de 4 ms, piso de 20 ms) e não reescala os períodos. Valores recalculados por script, com `Tmax` = 50 ms:

| Cenário | U | Cabe? |
|---|---|---|
| Configuração atual (7 PIDs) | 0,7817 (o log mostra `u=782`) | sim |
| + `0x44` a 600 ms | 0,865 | não |
| + `0x44` a 1 s | 0,832 | não |
| + `0x44` a 2,75 s | 0,7999 | sim, no limite (período mínimo viável cerca de 2,73 s) |
| + `0x44` a 5 s | 0,792 | sim |
| + `0x44` a 10 s | 0,787 | sim |
| + `0x07` a 10 s | 0,787 | sim |
| + `0x06` a 4 s e `0x07` a 10 s | 0,799 | sim |
| + `0x06` a 2 s e `0x07` a 10 s | 0,812 | não |
| + `0x06` a 1 s e `0x07` a 10 s | 0,837 | não |
| + `0x06` a 600 ms | 0,865 | não |
| + `0x06` a 600 ms e `0x07` a 10 s | 0,870 | não |
| + `0x06` a 250 ms e `0x07` a 10 s | 0,987 | não |
| `0x44` a 600 ms no lugar do `0x04` | 0,782 | sim, mas perde o gráfico de carga |
| + `0x06` a 600 ms e `0x07` a 10 s, com rotação, velocidade e pedal a 285 ms | 0,796 | sim |

`Tmax` máximo com períodos fixos (`Tmax = 1,3 × p99`):

| Cenário | Tmax teórico | Tmax via `AT ST` | U nesse Tmax | p99 até |
|---|---|---|---|---|
| Atual | 51,2 ms | 48 ms | 0,750 | 36,9 ms |
| + `0x44` a 600 ms | 46,2 ms | 44 ms | 0,761 | 33,8 ms |
| + `0x07` a 10 s | 50,8 ms | 48 ms | 0,755 | 36,9 ms |
| + `0x06` a 4 s e `0x07` a 10 s | 50,1 ms | 48 ms | 0,767 | 36,9 ms |
| + `0x06` a 2 s e `0x07` a 10 s | 49,3 ms | 48 ms | 0,779 | 36,9 ms |
| + `0x06` a 1 s e `0x07` a 10 s | 47,8 ms | 44 ms | 0,736 | 33,8 ms |
| **+ `0x06` a 600 ms e `0x07` a 10 s (intervalo adequado)** | **46,0 ms** | **44 ms** | **0,766** | **33,8 ms** |
| + `0x06` a 250 ms e `0x07` a 10 s | 40,5 ms | 40 ms | 0,789 | 30,8 ms |
| + `0x06`, `0x07`, `0x08` e `0x09`, nos mesmos intervalos adequados (motor em V) | 41,7 ms | 40 ms | 0,767 | 30,8 ms |

Com os trims no intervalo adequado, o `U` a 50 ms sobe para 0,870, e o firmware recusa a configuração. Há duas saídas:

- **Reduzir o `Tmax` para 44 ms via `AT ST`:** exige `p99` de até 33,8 ms.
- **Manter 50 ms e levar rotação, velocidade e pedal de 250 ms para 285 ms:** o `U` fica em 0,796. O piso garantido piora um pouco, mas o impacto prático tende a ser pequeno, porque a mediana observada desses PIDs é de cerca de 130 ms, graças às leituras antecipadas (inferência do projeto).

Leituras no barramento com os trims no intervalo adequado: as nominais passam de 15,6 para 17,4 por segundo. O teto com leituras antecipadas passa de 30,97 para 34,50 por segundo, ou cerca de 29 ms por leitura. Se a leitura levar mais que isso, as antecipadas diminuem, mas as agendadas continuam protegidas.

Pontos de atenção:

- Se o `Tmax` medido for cerca de 30 ms, `0x44` a 600 ms dá `U ≈ 0,52`.
- Com `Pᵢ = kᵢ·Tmax`, o U fica constante em 0,782 (soma de 1/kᵢ). Mas o firmware não reescala sozinho; seria preciso reconfigurar via `AT#PID`.
- O U não é um limite rígido com a ECU lenta. Em `esp32-firmware/docs/followups-latencia-ecusim.md` os timeouts chegam a cerca de 99 ms com a ECU acima do `Tmax`, porque frames que não são a resposta esperada renovam o prazo. Então a ocupação real pode passar da planejada.
- `kTmaxSamplesPerPid` está em 15 e deve voltar a 100 antes das medições do TCC.
- Um `0x44` a 2,75 s perde boa parte dos enriquecimentos, que duram menos que isso. O `0x06` a 4 s ajuda no total, não no instantâneo (inferência do projeto).
- O teto de 30,97 leituras/s vem dos intervalos mínimos; no PDF a taxa observada foi de 30,65 leituras/s (8.202 em 267,6 s). Isso sugere folga real no barramento (inferência do projeto).

## 4. Quando cada um é útil

| Cenário | O que fazer |
|---|---|
| Frota com veículos antigos, ou MAF com desvio | `0x07` (LTFT), a 10 s |
| Folga medida no barramento (`p99` de até 33,8 ms) | `0x06` a 600 ms e `0x07` a 10 s, com `Tmax` de 44 ms |
| Sem essa folga, mas com necessidade de corrigir o consumo instantâneo | `0x06` a 600 ms e `0x07` a 10 s, com `Tmax` de 50 ms e os PIDs rápidos a 285 ms |
| Condução muito agressiva, ou validação experimental | `0x44`, em coleta temporária |
| `U` já alto, ou ECU lenta com timeouts frequentes | Não adicionar; medir primeiro |
| Veículo diesel | Não adicionar trims (tendem a não existir) |
| Só mapa, eventos, marchas, rotação, pedal | Nenhum dos três: não ajudam |

## 5. Protocolo de decisão

1. Checar o suporte pelos PIDs suportados: `01 00` para `0x06` e `0x07`; `01 40` para `0x44`.
2. Começar só com o `0x07` a 10 s, se houver suporte: quase não custa nada, e o `U` a 50 ms fica em 0,787.
3. Rodar `AT#TMAX` no carro real, com `kTmaxSamplesPerPid` de volta a 100.
4. Para testar o `0x06` a 600 ms: com `p99` de até 33,8 ms, usar `Tmax` de 44 ms; acima disso, manter 50 ms e levar os PIDs rápidos a 285 ms.
5. Validar tanque cheio a tanque cheio, com e sem correção, para ter um erro medido. Se o ganho do `0x06` no total for pequeno, ele só se justifica se o consumo instantâneo precisar dessa precisão.
6. Coletar o `0x44` temporariamente por algumas viagens. Se a diferença ficar em poucos por cento, isso justifica não coletá-lo.

## 6. Revisão da recomendação

A recomendação mudou durante a análise e fica registrada:

- Primeiro recomendou-se adicionar `0x06` a 4 s e `0x07` a 10 s.
- Depois se concluiu que `0x06` a 4 s **não compensa** (ajuda no total, não no instantâneo, e consome orçamento) e que `0x07` **vale testar**.
- Para um ganho relevante também no instantâneo, o intervalo adequado do `0x06` é de 600 ms, o que exige reduzir o `Tmax` ou ajustar os PIDs rápidos (seção 3).
- O percentual de 39% de freio-motor no PDF exagera a importância do corte de combustível: é 39% do tempo em movimento, e nesse regime o fluxo de combustível tende a ser pequeno (inferência do projeto).
- Achado posterior: a patente US 7774130 (2010) traz um número com trims (−0,56%) além do resultado sem trims (−2,91%). Nesse único ensaio, o ganho foi de cerca de 2,4 pontos percentuais. Isso sustenta testar o `0x07`, mas não mede o ganho em carros reais (seção 1).

## Apêndice: outros candidatos

Números de PID a conferir em SAE J1979. A numeração não vem das fontes; as fontes citam sinais.

| PID | Sinal | Observação |
|---|---|---|
| `0x5E` | Taxa de combustível | Usar onde houver suporte. O VED [11] (2022) a indica como indisponível na maioria dos veículos a combustão |
| `0x43` | Carga absoluta | Alternativa em veículo sem MAF. O `0x04` não substitui diretamente o `AbsLoad` da fórmula do VED [11] (2022) |
| `0x0B` | Pressão do coletor (MAP) | Idem, para estimar o fluxo de ar em veículo sem MAF. [9] (2025) usa MAP e MAF |
| `0x0F` | Temperatura do ar de admissão | Não adicionar como padrão |
| `0x11` | Posição da borboleta | Não adicionar como padrão; o pedal (`0x49`) já está coletado |
| `0x52` | Percentual de etanol | Para carro flex; **fora das fontes** consultadas |

Todo PID novo precisa de uma linha na tabela `pid` do backend (`backend/internal/database/migrations/00005_pid.sql`), com nome, unidade, bytes, fator, deslocamento e faixa válida. Sem a linha, o servidor rejeita o bloco com o motivo `unknown_pid` (ação de aguardar; `backend/internal/domain/ingestion/apply.go`), e o envio daquele bloco fica parado até o PID ser cadastrado.
