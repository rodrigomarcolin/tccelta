# Cenários do simulador

O simulador (`tccelta simulator`, código em `internal/tools/simulator/`) testa a API sem hardware e sem celular. Ele
cria **dongles virtuais** e **apps virtuais** em Go, que falam o protocolo real com a API usando o mesmo
pacote `protocol` do servidor. Este documento descreve o que cada cenário faz e o que cada verificação
confere, a partir do código (`scenarios.go`, `dongle.go`, `app.go`, `report.go`).

Como rodar:

```
make scenario C=<nome> [N=3] [ARGS='…']   # stack de desenvolvimento, com dongles novos
make e2e                                 # stack isolada, quatro cenários em sequência
```

Veja o [README do backend](../README.md) para os alvos do Makefile.

## Resumo

| Cenário | Dongles | O que prova | Falha quando |
|---|---|---|---|
| `e2e` | todos do CSV, um por vez | O fluxo completo, com BOOT, LOSS e OVERFLOW, grava os dados certos e fecha o cursor | Qualquer uma das 6 verificações por dongle falha |
| `negative-protocol` | 2 ou mais (usa os dois primeiros) | Entradas adulteradas, fora de ordem, repetidas ou sem autorização são recusadas sem corromper o estado | Qualquer uma das 11 verificações falha |
| `time` | 1 (o primeiro) | O servidor reconstrói a hora das leituras a partir das âncoras de tempo | Qualquer uma das 4 verificações falha |
| `generation` | 1 (o primeiro) | O `boot_id` de 16 bits volta a 1 e o servidor avança a "geração" do boot; o reprocessamento preserva as leituras | Qualquer uma das 4 verificações falha |
| `reconnection` | `N` do CSV | Um volume grande de blocos acumulados offline é drenado por inteiro | Algum dongle termina com `contiguous_seq` diferente do HEAD |
| `load` | `N` do CSV | Mede latências por rota com muitos dongles em regime | Só se houver alguma resposta 5xx |
| `knee` | `N` do CSV (cicla) | Sobe a carga em degraus até achar o limite | Nunca falha: só reporta a capacidade |

Os cenários com verificação (`e2e`, `negative-protocol`, `time`, `generation`, `reconnection`) imprimem uma linha
`OK` ou `FALHOU` por verificação e terminam com erro (`N verificação(ões) falharam`) se alguma falhar. `load`
e `knee` não verificam dados; produzem um relatório.

## Pré-requisitos comuns

- A API, o KMS, o worker e o banco no ar, e os dongles do CSV **cadastrados** (KMS e banco). Dongle
  desconhecido dá `404 dongle_desconhecido` já no primeiro desafio.
- **Dongles novos a cada execução.** O dongle virtual sempre começa do seq 1, e o servidor guarda o estado por
  DevId. Reusar um DevId que um cenário anterior avançou dá `422 resposta_inconsistente` no handshake. Por isso
  o `make scenario` e o `make e2e` criam dongles novos.
- O simulador lê o banco (papel `tccelta_owner`, em dev e teste) para conferir o que foi gravado.

## O que o simulador simula

**Dongle virtual** (`DongleVirtual`): guarda a identidade e as chaves do CSV (DevId, K_auth, K_rec), o contador
`seq`, o `boot_id`, o último ACK gravado e uma flash em memória com capacidade configurável (padrão de um
milhão de blocos). Ao ser criado, executa um primeiro boot e gera o bloco BOOT de faixa vazia.

Cada bloco de dados gerado tem **61 registros**: 20 amostras de 3 PIDs (0x0C, 0x0D e 0x04, a 40 ms de
intervalo) mais 1 posição de GPS. São assinados com HMAC-SHA256 da K_rec, no formato real
(`MALCONG ‖ tipo ‖ DevId ‖ dados`). Por isso, no banco, cada bloco aceito vira **60 linhas em `reading` e 1
em `position`**.

Operações usadas pelos cenários:

| Operação | Efeito |
|---|---|
| `GenerateBlocks(n)` | Cria `n` blocos de dados assinados e os põe na flash |
| `LosePower(p)` | Queda de energia: `p` blocos que estavam só na RAM somem e um novo BOOT começa, deixando uma faixa de seq perdida |
| `Corrupt(seq)` | Marca o bloco como ilegível (CRC). No envio ele vira um marcador LOSS. Se for um BOOT, vira LOSS em pedaços |
| `DiscardSegment(n)` | Remove os `n` blocos mais antigos da flash, como o descarte por segmento do firmware |
| flash pequena (`FlashCapacity`) | Ao encher, descarta o bloco mais antigo: gera OVERFLOW |
| `DriftPPM` | Faz o relógio do dongle derivar |
| `InitialBoot` | Define o `boot_id` do primeiro boot |

**App virtual** (`AppVirtual`): reimplementa o app contra o contrato HTTP.

- **Handshake:** pede o desafio, entrega ao dongle, devolve a resposta, repassa o ACK. Repete até 3 vezes se a
  âncora de tempo falhar.
- **Drenar:** envia lotes até o servidor confirmar tudo (ACK igual ao HEAD). Renova o token se faltarem menos
  de 5 minutos. O tamanho do lote `K` começa em 32, dobra (até 32) quando há progresso e cai pela metade em
  `429`/`503`, esperando o `Retry-After` (no máximo 3 s). Um `401` refaz o handshake. Três lotes sem progresso
  viram erro.

## `e2e`

Roda, **para cada dongle do CSV, um depois do outro**, a mesma história com flash de **30 blocos**
(propositalmente pequena):

1. **Handshake.**
2. **Fase 1:** gera e drena 200 blocos (em rodadas de 20).
3. **Queda de energia:** `LosePower(3)` cria um BOOT com uma lacuna de 3 blocos; novo handshake; gera e
   drena 50 blocos.
4. **Bloco corrompido:** gera 10 blocos, corrompe o de seq `HEAD − 5` (o 6º contando do fim) e drena. O bloco
   chega como LOSS.
5. **Offline com flash cheia:** gera 60 blocos de uma vez (a flash de 30 descarta os mais antigos: OVERFLOW),
   faz o handshake e drena.
6. **Apenas no último dongle do CSV**, dois casos extras de reconciliação:
   - **Caso 16 (OVERFLOW que engole o segmento antes do BOOT):** gera 20 blocos, `LosePower(5)`, gera 4
     blocos e descarta os 20 mais antigos da flash; handshake e dreno.
   - **Caso 18 (BOOT corrompido):** `LosePower(3)`, gera 2 blocos, corrompe o BOOT mais recente;
     handshake e dreno. O BOOT corrompido vira LOSS em pedaços.

Depois de cada dongle, as **verificações**, lendo o banco:

| # | Verificação | Mensagem |
|---|---|---|
| 1 | `contiguous_seq` e `head_seq` do servidor são iguais ao HEAD do dongle: nada ficou para trás | `contiguous_seq=… head=…` |
| 2 | A tabela `gap` tem as três causas: `BOOT`, `LOSS` e `OVERFLOW` | `lacunas map[…]` |
| 3 | O número de linhas em `reading` e em `position` bate **exatamente** com o esperado: soma dos registros dos blocos que **não** estão cobertos por uma lacuna | `leituras=n/esperado posicoes=n/esperado` |
| 4 | `rejected_block` está vazio | `rejected_block vazio` |
| 5 | Existe ao menos uma âncora de tempo | `ancoras=…` |
| 6 | Há registros em `boot` e ao menos um já tem tempo calculado (`a_ms` e `time_source`), esperando até 15 s o worker terminar | `boots=… com tempo=…` |

A verificação 3 é a mais forte: confere que nenhum bloco foi perdido sem lacuna e que nenhum foi gravado em
duplicata.

## `negative-protocol`

Usa o primeiro dongle como "atacante" e o segundo só como alvo de um teste cruzado. Começa com um handshake, 10
blocos e um dreno, e anota quantas leituras existem (`l0`). Depois, em ordem:

| Passo | Ação | Verificação |
|---|---|---|
| a | 3 blocos; troca 1 byte do segundo | A API responde 200, **interrompido**, ação `reler`, motivo `tag_invalida`; o bloco adulterado aparece em `rejected_block` (≥ 1) |
| b | Envia só os blocos 2 e 3, pulando o primeiro | Motivo `salto`, ação `reler` |
| c | Drena e relê contagens | Sem verificação própria (só guarda a contagem para o passo final) |
| d | Lote de 33 blocos | `429` com código `lote_grande` (o máximo é 32) |
| e | Token do dongle A em `GET /v1/dongles/<B>/confirmacao` | `403` |
| f | Responde ao desafio corretamente e reenvia a mesma resposta | A primeira dá 200; a repetida dá `409` |
| g | Rajada de 5 pedidos de desafio | O último é `429` (desafios pendentes) |
| h | `POST /blocos` com um token lixo | `401` |
| final | Novo handshake (espera 2 s se houver desafios pendentes), 5 blocos, dreno | `contiguous_seq` igual ao HEAD; e as leituras só cresceram: `l2 > l0` e `l2 ≥ l1`, isto é, nenhum ataque alterou dados já aceitos |

## `time`

Um dongle (o primeiro), com `--drift` opcional em ppm. Faz **4 rodadas**: handshake, 5 blocos, dreno e 1 s de
pausa, produzindo 4 âncoras. Verificações:

1. O boot mais recente tem tempo calculado e `time_source = anchors`.
2. `base_uncertainty_ms ≤ 2000` (o RTT máximo), se o campo existir.
3. A **hora reconstruída** da leitura difere do relógio do computador em menos de 10 s.
4. Há ao menos 4 âncoras.

Este cenário roda em tempo real (cerca de 4 s de pausas). O `--speedup` **não tem efeito** aqui: só os
cenários `load` e `knee` o usam. O `make e2e` o passa ao `time` (`--acelerar 120`), mas ele é ignorado.

## `generation`

Um dongle (o primeiro) que começa com `boot_id = 65534`, perto do limite de 16 bits. Faz um handshake e 4
rodadas de 5 blocos com dreno; entre as três primeiras, `LosePower(0)` e novo handshake. Os boots seguem
65534, 65535, depois o contador dá a volta (0 vira 1) e chega a 2. Verificações:

1. `boot_generation = 1` e `last_boot_id = 2` no estado do dongle.
2. A tabela `boot` tem exatamente os pares (geração, boot): `(0, 65534)`, `(0, 65535)`, `(1, 1)` e `(1, 2)`.
3. O alerta `boot_generation_advanced` foi registrado **uma vez**.
4. **Reprocessamento:** roda `reprocess` primeiro como ensaio e depois com `--apply`; a contagem de
   leituras antes e depois é a mesma e maior que zero.

## `reconnection`

Para cada dongle (`--dongles`, padrão: o tamanho do CSV): gera **`--accumulated` blocos** offline (padrão 500),
espera sua vez numa rampa (`--ramp`, padrão 30 s, distribuída entre os dongles), e drena tudo com limite de
10 min por dongle. Ao fim registra `drain_s` no relatório. Verificação: para cada dongle, `contiguous_seq` do
servidor é igual ao HEAD do dongle. Simula um parque de dongles voltando ao ar depois de ficar sem conexão.

## `load`

Roda `N` dongles "em regime" por `--duration` (padrão 2 min). Cada dongle:

1. Espera um instante da rampa (`i/N × min(rampa, T_lat)`).
2. Faz o handshake. **Se falhar (por exemplo, `429`), o dongle para e não tenta de novo.**
3. Em laço: gera `max(1, 0,17 × T_lat)` blocos (10, com `T_lat` de 60 s), drena, e espera `T_lat / acelerar`.

O relatório traz latências por rota, status, e os campos extras `dongles` e `responses_5xx`. O cenário só
**falha se houver resposta 5xx**; não confere as metas de desempenho.

Limites a ter em mente:

- Todos os dongles saem do mesmo IP. O limite padrão da API é de 120 requisições por minuto por IP
  (`LIMITE_IP_MIN`), e acima disso vêm `429`, o que reduz a carga em vez de medir o servidor.
- Se `--dongles` for maior que o CSV, as identidades **se repetem** (ciclam), e dois dongles virtuais com o
  mesmo DevId se atrapalham. Mantenha o CSV com pelo menos `N` dongles (o `make load-test` já cria `N`).

## `knee`

Executa o mesmo regime da `load` em degraus de **100, 250, 500, 1.000, 2.000 e 4.000** dongles, cada um por
`--duration` e com relatório próprio. Em cada degrau anota o p99 de `POST /blocos` e a fração de `503`. Para no
primeiro degrau em que o p99 passa de 1.000 ms ou os `503` passam de 1 %, e reporta o último degrau que
passou em `dongle_capacity`. Nunca devolve erro. Como a `load`, depende do CSV ter dongles suficientes e
está sujeito ao limite por IP.

## O `make e2e`

É um encadeamento, não um cenário: sobe a stack **isolada** (`compose.test.yaml`, projeto `tccelta-teste`),
reconstrói as imagens de ferramentas e roda **`e2e`, `negative-protocol`, `time` e `generation`**, cada um com 3
dongles recém-criados (KMS, depois banco). Se um falhar, os seguintes não rodam. No fim, com sucesso ou não,
derruba a stack com `down -v` e devolve o código de saída do cenário que falhou. Não cobre `reconnection`, `load`
nem `knee`.

Diferença para o `make scenario`: o `scenario` roda **um** cenário na stack de desenvolvimento, que continua no ar
com os dados; o `e2e` usa um banco e um Vault próprios, que desaparecem.

## Relatório

Cada execução imprime um JSON e, com `--report arquivo`, o grava (o `make load-test` usa
`saida/relatorio.json`):

| Campo | Conteúdo |
|---|---|
| `scenario`, `params` | Nome e opções da execução |
| `duration_s`, `requests_per_s` | Duração total e vazão média |
| `routes.<route>` | `n`, `p50_ms`, `p95_ms`, `p99_ms` e `status` (contagem por código HTTP), para `challenge`, `response`, `blocks`, `confirmation` |
| extras | `dongles` e `responses_5xx` (carga); `drain_s` (reconexão); `step_<n>` e `dongle_capacity` (joelho) |

As latências são agregadas **por rota**, sem separar por status: um `429`, que é barato, mistura-se com os
`200` e puxa o p50 para baixo. Com poucas requisições (dezenas), o p99 é praticamente o valor máximo.

## Limites do simulador

- Usa o mesmo pacote `protocol` que o servidor. Um erro de formato que exista nos dois lados passa
  despercebido; a checagem independente são os vetores gerados pelo firmware (`vetores_firmware.json`).
- Não simula BLE, MTU, fragmentação nem o tempo de gravação da flash. A flash é uma lista em memória.
- Cada dongle virtual nasce do zero, e o firmware real nunca "recomeça do seq 1" sozinho. O `422` por estado
  antigo é um comportamento correto do servidor, mas hoje nenhum cenário o testa de propósito.
- No passo (c) do `negative-protocol`, as variáveis `stale` e `oldHead` são lidas e descartadas: o reenvio de
  lotes antigos não é, de fato, verificado.

## Estado de execução

- `e2e` e a sequência do `make e2e` foram executados pelo autor do projeto depois do refactor. O primeiro
  cenário passou; os resultados completos do `make e2e` não foram registrados.
- `negative-protocol` falhou uma vez com `422` por reaproveitar os dongles do cenário anterior; isso foi
  tratado criando dongles novos por cenário.
- `load` rodou uma vez com 100 dongles: 0 respostas 5xx, mas 60 dos 100 desafios receberam `429` pelo limite
  por IP, e só 40 dongles chegaram a enviar blocos (1,83 req/s). Não serve como medida de capacidade.
- `reconnection` e `knee` não foram executados depois do refactor.
