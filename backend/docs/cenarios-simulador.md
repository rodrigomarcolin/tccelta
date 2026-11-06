# Cenários do simulador

O simulador (`tccelta simulator`, código em `internal/tools/simulator/`) testa a API sem hardware e sem celular. Ele
cria **dongles virtuais** e **apps virtuais** em Go, que falam o protocolo real com a API usando o mesmo
pacote `protocol` do servidor. Este documento descreve o que cada cenário faz e o que cada verificação
confere, a partir do código (`scenarios.go`, `dongle.go`, `app.go`, `report.go`).

Como rodar:

```
make scenario C=<nome> [N=3] [ARGS='…']   # stack de desenvolvimento, com dongles novos
make scenario C=asym                     # dongles asym (ECDSA); PROFILE=hmac|asym|none escolhe o perfil
make e2e                                 # stack isolada, cinco cenários em sequência
```

Veja o [README do backend](../README.md) para os alvos do Makefile.

## Resumo

| Cenário | Dongles | O que prova | Falha quando |
|---|---|---|---|
| `e2e` | todos do CSV, um por vez | O fluxo completo, com BOOT, LOSS e OVERFLOW, grava os dados certos e fecha o cursor | Qualquer uma das 6 verificações por dongle falha |
| `negative-protocol` | 2 ou mais (usa os dois primeiros) | Entradas adulteradas, fora de ordem, repetidas ou sem autorização são recusadas sem corromper o estado | Qualquer uma das 11 verificações falha |
| `time` | 1 (o primeiro) | O servidor reconstrói a hora das leituras a partir das âncoras de tempo | Qualquer uma das 4 verificações falha |
| `generation` | 1 (o primeiro) | O `boot_id` de 16 bits volta a 1 e o servidor avança a "geração" do boot; o reprocessamento preserva as leituras | Qualquer uma das 4 verificações falha |
| `asym` | todos do CSV, um por vez (todos `asym`, com `k_sig`) | Blocos assinados com ECDSA P-256 são aceitos; uma tag com 1 bit trocado é rejeitada como `invalid_tag` e o ACK não avança | Qualquer uma das 7 verificações por dongle falha |
| `trip` | 1 (o primeiro) | O worker calcula as viagens da gold (entrada `boot` e entrada `intervalo`) a partir da silver, com o papel `tccelta_worker` | Qualquer uma das 7 verificações falha |
| `trip-http` | 1 (o primeiro) | As rotas de viagem da API (`POST`, listagem, status e análise), a recusa da viagem repetida e a do nome repetido, com o worker calculando. Exige `--client`: autentica como o Next.js (client credentials) | Qualquer verificação falha |
| `client-auth` | não usa o dongle (o CSV é exigido pelo comando) | `POST /v1/auth/token`, JWKS, Bearer das rotas de viagem, isolamento entre o token do app e o de cliente, `invalid_client` uniforme, bloqueio progressivo e métricas `tccelta_auth_*`; exige `--client` | Qualquer verificação falha |
| `client-auth-rotation` | não usa o dongle | Depois de `clientjwt rotate --now`: o token antigo segue válido, o novo sai com `cli-v<n+1>` e o JWKS publica as duas versões | Qualquer verificação falha |
| `reconnection` | `N` do CSV | Um volume grande de blocos acumulados offline é drenado por inteiro | Algum dongle termina com `contiguous_seq` diferente do HEAD |
| `load` | `N` do CSV | Mede latências por rota com muitos dongles em regime | Só se houver alguma resposta 5xx |
| `knee` | `N` do CSV (cicla) | Sobe a carga em degraus até achar o limite | Nunca falha: só reporta a capacidade |

Os cenários com verificação (`e2e`, `negative-protocol`, `time`, `generation`, `asym`, `trip`, `reconnection`) imprimem uma linha
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

**Dongle virtual** (`DongleVirtual`): guarda a identidade e as chaves do CSV (DevId, K_auth, K_rec e, no perfil `asym`, a K_sig), o contador
`seq`, o `boot_id`, o último ACK gravado e uma flash em memória com capacidade configurável (padrão de um
milhão de blocos). Ao ser criado, executa um primeiro boot e gera o bloco BOOT de faixa vazia.

Cada bloco de dados gerado tem **61 registros**: 20 amostras de 3 PIDs (0x0C, 0x0D e 0x04, a 40 ms de
intervalo) mais 1 posição de GPS. São assinados pelo perfil do dongle, no formato real
(`MALCONG ‖ tipo ‖ DevId ‖ dados`): HMAC-SHA256 da K_rec (`hmac`), ECDSA P-256 `r‖s` (`asym`) ou tag vazia (`none`). Por isso, no banco, cada bloco aceito vira **60 linhas em `reading` e 1
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

## `asym`

Todos os dongles do CSV, um por vez; exige que sejam do perfil `asym` e tenham `k_sig` (`make scenario C=asym`
já cria dongles `asym`; no `make e2e` são os do `E2E_NEW_DONGLES,asym,asym`). Faz o handshake, gera e drena 100
blocos assinados com ECDSA e depois envia um bloco com 1 bit da tag trocado. Verificações por dongle:

1. `contiguous_seq` igual ao HEAD do dongle.
2. Há leituras e posições gravadas.
3. `rejected_block` vazio antes da adulteração.
4. A tag adulterada volta `422` com `X-Lote-Acao: reler` e `X-Lote-Motivo: tag_invalida` (sem progresso).
5. O ACK devolvido fica parado no `seq` anterior.
6. `rejected_block` tem exatamente 1 linha.
7. Depois do reenvio do bloco intacto, o `contiguous_seq` chega ao HEAD.

O que o cenário não olha é o KMS: no perfil `asym` a verificação é local, então
`tccelta_kms_calls_total{operation="verify_rec"}` não cresce com esses dongles (confira em `/metricas`).

## `trip`

Prova o worker de viagens (API1) de ponta a ponta, sobre o primeiro dongle do CSV:

1. Faz o handshake, gera e envia 200 blocos (PIDs `0x0C`, `0x0D` e `0x04`, mais GPS) e espera o worker dar hora ao boot.
2. Cria duas viagens em `gold.trip` com o papel dono das tabelas (a API de viagens é coberta pelo cenário `trip-http`): uma com entrada
   `boot` (o boot do dongle) e outra com entrada `intervalo`, de poucos segundos antes do início até 5 s depois do fim.
3. Espera até 30 s que cada uma chegue a `ready` (o `compose.test.yaml` usa `VIAGEM_ESPERA_SEM_LOTE=3s` e `VIAGEM_LIMITE_FINAL=8s`).

Verificações: o boot tem hora absoluta; e, para cada viagem, o estado `ready`, ao menos uma linha em `trip_part`, `distance_km > 0`
e `engine_on_s > 0`. Como o cálculo roda no `worker` (papel `tccelta_worker`), estar `ready` também prova que ele leu `reading` e
`position` sem erro de permissão.

## `trip-http`

Prova as rotas de viagem (API2) de ponta a ponta, sem inserir em `gold.trip`: tudo passa por `POST /v1/viagens`, `GET /v1/viagens`,
`GET /v1/viagens/{id}` e `GET /v1/viagens/{id}/analise`. Com `--client id:arquivo:ok` (o `make e2e` passa; é obrigatório), obtém o token de client credentials e o usa como Bearer, como o servidor do Next.js. As rotas de viagem sem Bearer respondem `401`; não existe mais modo sem autenticação. Usa o primeiro dongle do CSV
e uma categoria nova por execução (o nome do motorista é único por categoria).

0. Gera 200 blocos e, depois de uma queda de energia, mais 40: o dongle fica com dois boots, o primeiro para a viagem 1 e o segundo
   para testar o nome repetido sem repetir a viagem.
1. Erros de contrato: dongle sem estado (`404 dongle_desconhecido`), combustível `personalizado` (`400 entrada_invalida`), id que não é UUID (`404`).
2. Boot já enviado: `202` com `Location`; o mesmo motorista com outra caixa, **em outro boot**, dá `409 motorista_repetido`; o status chega a
   `pronta` (limite de 40 s) e a análise traz `viagem.motorista`, `viagem.categoria_viagem`, distância, séries e rota.
3. A mesma viagem de novo (outro motorista, o mesmo nome com outra caixa, outra categoria e `boot_generation` explícita): `409 viagem_repetida`
   com `viagem_id` igual ao da viagem 1 e `motorista.nome` "Boot enviado". A viagem repetida vence o nome repetido.
4. Listagem: cria uma segunda viagem (no outro boot, motorista "Boot enviado 2") e lê `GET /v1/viagens?limite=1&busca=…` página a página.
   A página 1 tem `total` 2, `Cache-Control: no-store`, a viagem mais nova e um `proximo_cursor`; a página 2, pelo cursor, é a última
   (`proximo_cursor` nulo) e traz a viagem 1 com todos os campos (`pronta`, `entrada_tipo`, `boot_id`, `boot_generation`, `inicio_em`, `fim_em`,
   `distancia_km`, `duracao_s`, `criada_em` e `rota_resumo` com ao menos um trecho). Uma busca sem resultado devolve `viagens: []` e `total` 0;
   `limite=0`, `limite=51`, `limite=abc`, `cursor=lixo` e `busca` com 61 caracteres dão `400 entrada_invalida`; sem Bearer, `401`.
5. Boot inexistente (o próximo ao último e um número absurdo): `404 boot_desconhecido`, sem criar viagem.

## `client-auth` e `client-auth-rotation`

Provam o client credentials (API3) de ponta a ponta, com o Redis, o KMS e o Vault do `compose.test.yaml`. Os clientes vêm de
`--client id:arquivo_do_segredo:ok|invalido` (repetível; o primeiro `ok` é o principal) e o `make e2e` os cria com
`tccelta-kms client create|rotate|revoke`: `e2e-cli` (principal), `e2e-rot` (segredo novo `ok`, segredo antigo `invalido`) e `e2e-rev`
(revogado, `invalido`). A API do teste usa uma progressão curta (`TOKEN_BLOQUEIO_INICIAL=2s`, `TETO=8s`, `ESQUECER=6s`).

`client-auth`:

1. `200` com `access_token`, `Bearer`, `expires_in`, `Cache-Control: no-store` e `Pragma: no-cache`; claims `aud` `tccelta-analise`,
   `sub` do cliente e sem `cs`.
2. O JWKS só tem chaves `cli-v<n>` e o token verifica com a chave do `kid`.
3. Rotas de viagem: `token_ausente`, `token_invalido` e `404 viagem_desconhecida` com token válido.
4. O token de cliente é recusado em `/v1/dongles/blocos` e `/v1/dongles/{id}/confirmacao`.
5. `400` para `grant_type` errado, `scope` e `Content-Type`.
6. Segredo errado, cliente desconhecido, segredo antigo, cliente revogado e Basic ausente dão a mesma resposta `401 invalid_client`.
7. Seis falhas do par: a 6ª é `401` e a seguinte é `429` com `Retry-After`; depois dele, a credencial certa volta a funcionar.
8. As séries `tccelta_auth_*` existem e nenhuma usa `client_id` ou IP como rótulo.

Com `--token-file`, grava o token emitido. `client-auth-rotation` roda depois de `clientjwt rotate --now` (serviço `kms-rotacao` do
`compose.test.yaml`): o token antigo segue válido até `exp`, o token novo sai com a chave nova e é aceito sem reiniciar a API (ela recarrega
as chaves ao ver um `kid` desconhecido, no máximo a cada 30 s) e o JWKS publica as duas versões.

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
reconstrói as imagens de ferramentas e roda **`e2e`, `negative-protocol`, `time`, `generation`, `asym`, `trip` e `trip-http`**, cada um com 3
dongles recém-criados (KMS, depois banco; o `asym` com `--profile asym`). Se um falhar, os seguintes não rodam. No fim, com sucesso ou não,
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
