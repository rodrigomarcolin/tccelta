# Armazenamento dos dados de telemetria do Dongle no servidor

Oct 20, 2026 · @Davi

## Objetivo

A proposta é armazenar a telemetria em PostgreSQL com TimescaleDB e PostGIS, organizada em quatro camadas: blocos brutos imutáveis, contexto (boots e âncoras de tempo), leituras decodificadas e dados prontos para análise por motorista. O objetivo é permitir análises de condução comparáveis entre motoristas sem perder a verificabilidade criptográfica dos dados.

O documento se baseia em três documentos do projeto: *Estratégia da Coleta do TCCELTA* (Investigação-3), *Segurança da comunicação entre dongle e servidor* e os diagramas do coletor (*collector\_diagrams*).

## Fluxo de dados fim a fim

O fluxo é offline-first: o servidor recebe blocos autenticados, nunca leituras soltas, e confirma o maior seq contíguo.

&#91;embedded content: fluxo fim a fim · do CAN ao banco\]

O SessionTask, em paralelo, troca o desafio com o servidor, que gera a âncora de tempo do boot e abre uma nova época. Coleta e gravação na flash continuam sem o app conectado.

| Estrutura | Campos (bytes) | Tamanho |
| --- | --- | --- |
| Cabeçalho, um por bloco | seq (4), boot\_id (2), t\_ms inicial (4), tamanho (2) | 12 B |
| Leitura de PID | tipo (1), Δt (2), valor (2) | 5 B |
| Posição GPS (tipo 0xFF) | tipo (1), Δt (2), lat, lon, alt (12) | 15 B |

Formato da seção 2.6.4 da Investigação-3. Cada bloco tem até 1 KB e é fechado ao encher ou após 10 s.

## Consequências para o modelo de dados

O formato dos blocos impõe quatro decisões ao banco.

1. **A chave natural de uma leitura é (dongle, boot\_id, t\_ms).** O horário absoluto é derivado: pode chegar depois do bloco, nunca chegar, e sempre carrega uma incerteza.
2. **As leituras são assíncronas.** Cada PID tem seu próprio instante, então o dado bruto é longo (uma linha por leitura), não uma linha com uma coluna por PID.
3. **O seq é por bloco.** O servidor precisa guardar o maior seq contíguo, enviado como ACK. Os buracos na sequência são derivados dos próprios blocos recebidos.
4. **Dongle não é motorista.** O dongle mede um veículo. Vincular viagens a motoristas é responsabilidade de outro sistema (cadastro da frota ou da seguradora) e fica fora deste modelo.

## Tecnologia e volume

Para o TCC, um único PostgreSQL com TimescaleDB e PostGIS cobre tudo. A parte relacional, as séries temporais com compressão e as consultas geográficas ficam no mesmo banco e se cruzam com joins em SQL.

Os blocos brutos ficam guardados de forma imutável, numa tabela `bytea` ou num object storage (S3, MinIO). Eles são a evidência reverificável pela assinatura do dongle e permitem reprocessar tudo se a decodificação ou a segmentação de viagens mudar.

| Cenário | Leituras/s | Linhas por hora de condução | Linhas por dia |
| --- | --- | --- | --- |
| 1 veículo, Tmax = 200 ms | 3,9 a 7,7 | 14 a 28 mil | depende do uso |
| 1 veículo, Tmax = 50 ms | 15,6 a 31 | 56 a 112 mil | depende do uso |
| 1000 veículos, 2 h/dia, Tmax = 50 ms | — | — | 110 a 220 milhões |

As taxas vêm da Tabela 14 da Investigação-3 e não incluem GPS. Até alguns dispositivos, o PostgreSQL sobra. Na escala de frota, a camada analítica pode migrar para ClickHouse com o mesmo modelo, ordenado por (dongle\_id, boot\_id, t\_ms).

## Organização em camadas

Os dados passam por quatro camadas, da evidência bruta às métricas por motorista. Cada camada pode ser reconstruída a partir da anterior.

### Bronze: o que foi recebido, como foi recebido

```sql
CREATE TABLE bloco_bruto (
  dongle_id    int,
  seq          bigint,
  boot_id      int,
  t0_ms        bigint,
  epoca        int,             -- época em que o bloco foi assinado
  payload      bytea,           -- bloco exatamente como veio
  assinatura   bytea,
  recebido_em  timestamptz DEFAULT now(),
  PRIMARY KEY (dongle_id, seq)  -- idempotência: replay vira no-op
);

-- Estado operacional de cada dongle visto pelo servidor.
CREATE TABLE dongle_estado (
  dongle_id          int PRIMARY KEY,
  seq_contiguo       bigint,            -- ACK_seq: enviado na confirmação
  maior_seq_visto    bigint,
  head_seq           bigint,            -- HEAD_seq do último handshake, ou maior seq recebido
  tail_seq           bigint,            -- TAIL_seq do último handshake
  head_relatado_em   timestamptz,       -- S2 do handshake que relatou head e tail
  ultimo_c_s         bigint NOT NULL DEFAULT 0  -- último c_S emitido (uint32)
);
```

A chave primária `(dongle_id, seq)` neutraliza a repetição (A3.5) e a reordenação (A3.6) no próprio banco. As lacunas não ficam numa tabela própria: quando uma análise precisa delas, saem de uma consulta sobre `bloco_bruto` que procura saltos na sequência de seq. Uma viagem com lacuna é marcada como incompleta, não analisada como inteira.

Com `head_seq` e `tail_seq`, o servidor conhece a faixa inteira que o dongle ainda guarda na flash, \[TAIL\_seq, HEAD\_seq\], e classifica cada seq:

| Faixa de seq | Situação |
| --- | --- |
| até `seq_contiguo` | recebido sem buracos |
| buracos entre `seq_contiguo` e `maior_seq_visto` | faltando no meio (A3.6, A3.7) |
| de `maior_seq_visto + 1` até `head_seq` | gerado e ainda não enviado (A3.8) |
| abaixo de `tail_seq` e acima de `seq_contiguo` | descartado pelo dongle com a flash cheia |

Os dois valores só mudam num handshake aceito, porque só a Resposta 0x02 os traz autenticados com Kauth e amarrados ao nonce N\_S. `head_seq` nunca diminui: um relato menor que o salvo é rejeitado. Ele também sobe sem handshake quando chega um bloco com seq maior que o salvo, mas aí `head_relatado_em` não muda, porque o aumento é deduzido dos blocos, não relatado pelo dongle. Um `head_relatado_em` antigo indica um dongle que parou de sincronizar (A3.9).

### Contexto: boots e tempo

```sql
-- Uma linha por boot. As colunas de tempo são derivadas: o worker
-- as recalcula sempre que o boot ganha uma âncora nova.
CREATE TABLE boot (
  dongle_id int, boot_id int,
  config jsonb,                 -- protocolo, Tmax, PIDs e períodos
  a_ms double precision,        -- hora real (ms desde a época Unix) em t_ms = 0
  b double precision,           -- deriva medida; 0 com um só grupo de âncoras
  incerteza_base_ms int,        -- meia largura do grupo mais estreito
  origem_tempo text,            -- 'ancoras' ou 'vizinhos' (boot sem âncora)
  tempo_atualizado_em timestamptz,
  PRIMARY KEY (dongle_id, boot_id)
);

-- Uma linha por desafio emitido: o rastro de cada c_S e N_S.
CREATE TABLE desafio (
  dongle_id      int NOT NULL,
  c_s            bigint NOT NULL,       -- contador do desafio (uint32), crescente por dongle
  nonce          bytea NOT NULL UNIQUE, -- N_S, 16 bytes aleatórios
  s1             timestamptz NOT NULL,  -- servidor devolveu o desafio
  expira_em      timestamptz NOT NULL,
  -- preenchidos com a Resposta 0x02
  respondido_em  timestamptz,           -- S2; NULL enquanto pendente
  boot_id        int,
  t_ms           bigint,
  head_seq       bigint,
  tail_seq       bigint,
  resultado      text,                  -- 'aceito', 'aceito_sem_ancora' (S2 - S1 > 2 s),
                                        -- 'tag_invalida', 'inconsistente' ou 'expirado'
  -- efeitos de um handshake aceito
  ack_seq        bigint,                -- ACK_seq devolvido no ACK 0x04
  perda_ini      bigint,                -- faixa descartada com a flash cheia, se houve
  perda_fim      bigint,
  token_jti      uuid,                  -- identificador do JWT emitido
  PRIMARY KEY (dongle_id, c_s)
);

-- Uma linha por handshake com S2 - S1 <= 2 s: a medida crua.
-- O deslocamento do boot está em [s1 - t_ms, s2 - t_ms].
CREATE TABLE ancora_tempo (
  dongle_id int, boot_id int,
  t_ms bigint,                  -- t_ms que o dongle mandou na resposta
  nonce bytea NOT NULL REFERENCES desafio (nonce),
  s1 timestamptz,               -- cópia de desafio.s1
  s2 timestamptz,               -- cópia de desafio.respondido_em
  PRIMARY KEY (dongle_id, boot_id, t_ms)
);
```

A configuração fica por boot porque toda troca de protocolo ou de PIDs reinicia o dongle. O boot é, portanto, a unidade em que a configuração é constante.

As colunas de tempo de `boot` ficam nulas até a primeira estimativa. Os grupos de âncoras usados no cálculo não são guardados: o worker os remonta em memória a partir de `ancora_tempo` a cada recálculo.

A tabela `desafio` guarda todo desafio emitido, respondido ou não, com o que a Resposta 0x02 trouxe e o que o handshake decidiu. No `POST /desafio`, a API obtém o próximo c\_S incrementando `ultimo_c_s` em `dongle_estado` e insere a linha com c\_S, N\_S, `s1` e o prazo. No `POST /desafio/resposta`, ela consome o desafio numa única instrução, que já confere se ele é daquele dongle, está pendente e dentro do prazo, e impede duas respostas simultâneas ao mesmo desafio:

```sql
UPDATE desafio SET respondido_em = $4   -- S2, anotado pela API na chegada
WHERE dongle_id = $1 AND c_s = $2 AND nonce = $3
  AND respondido_em IS NULL AND expira_em > $4
RETURNING s1;
```

Se nenhuma linha voltar, a resposta é recusada com `409`. Se voltar, a API grava os campos da resposta, o `resultado` e os efeitos do handshake e, quando a latência permite, a âncora em `ancora_tempo`, apontando para o nonce. Desafios nunca respondidos ficam com `respondido_em` nulo e são marcados como expirados por um job periódico.

O histórico permite auditar o protocolo: quantos desafios cada dongle recebeu, quantos o app nunca entregou (A3.3), quantos ficaram sem âncora por latência e quais blocos o dongle declarou descartados com a flash cheia, com a faixa exata em `perda_ini` e `perda_fim`.

### Silver: leituras decodificadas

```sql
-- Fonte da verdade dos PIDs: o que cada um mede e como decodificar.
-- valor = (bruto * fator) + deslocamento, onde bruto = A (1 byte) ou 256A + B (2 bytes)
CREATE TABLE pid (
  pid           smallint PRIMARY KEY,   -- código do modo 01, ex.: 0x0C
  nome          text NOT NULL,
  unidade       text NOT NULL,
  n_bytes       smallint NOT NULL CHECK (n_bytes IN (1, 2)),
  fator         double precision NOT NULL,
  deslocamento  double precision NOT NULL DEFAULT 0,
  valor_min     double precision,       -- faixa válida, para descartar lixo
  valor_max     double precision
);

CREATE TABLE leitura (
  dongle_id int, boot_id int,
  t_ms bigint,                   -- t0_ms do bloco + Δt
  pid smallint NOT NULL REFERENCES pid (pid),
  valor_bruto int,
  valor real,                    -- calculado com fator e deslocamento de pid
  seq_bloco bigint,
  recebido_em timestamptz
);
SELECT create_hypertable('leitura', 'recebido_em');
-- compressão: segmentby = dongle_id, pid; orderby = boot_id, t_ms
-- índice: (dongle_id, boot_id, pid, t_ms)

CREATE TABLE posicao (
  dongle_id int, boot_id int, t_ms bigint,
  geom geography(PointZ),
  origem text DEFAULT 'app',
  seq_bloco bigint,
  recebido_em timestamptz
);
SELECT create_hypertable('posicao', 'recebido_em');
-- compressão: segmentby = dongle_id; orderby = boot_id, t_ms
-- índice: (dongle_id, boot_id, t_ms)
```

Os dois hypertables são particionados por `recebido_em` porque o horário absoluto pode ser nulo. O particionamento só organiza o armazenamento; consultas por viagem usam o índice em `(dongle_id, boot_id, t_ms)`.

`posicao` não tem índice espacial. Nos chunks comprimidos, não conte com um índice GiST sobre `geom`, e uma consulta por região acabaria varrendo os dados antigos. As consultas espaciais usam o trajeto de cada viagem, na camada gold, e só depois buscam os pontos em `posicao` pelo índice comum.

A tabela `pid` começa com os PIDs coletados hoje:

```sql
INSERT INTO pid (pid, nome, unidade, n_bytes, fator, deslocamento, valor_min, valor_max) VALUES
  (x'04'::int, 'Carga do motor',          '%',    1, 100.0/255, 0,   0,   100),
  (x'05'::int, 'Temperatura do líquido',  '°C',   1, 1,         -40, -40, 215),
  (x'0C'::int, 'Rotação',                 'RPM',  2, 0.25,      0,   0,   16383.75),
  (x'0D'::int, 'Velocidade',              'km/h', 1, 1,         0,   0,   255),
  (x'10'::int, 'MAF',                     'g/s',  2, 0.01,      0,   0,   655.35),
  (x'2F'::int, 'Nível de combustível',    '%',    1, 100.0/255, 0,   0,   100),
  (x'49'::int, 'Pedal do acelerador',     '%',    1, 100.0/255, 0,   0,   100);
```

A API carrega essa tabela na memória ao iniciar e decodifica cada leitura com ela, em vez de ter as fórmulas fixas no código. Um PID novo passa a ser aceito com um `INSERT`, sem novo deploy. Um bloco com um PID que não está na tabela é rejeitado inteiro e registrado, porque a chave estrangeira faria o `COPY` do lote falhar de qualquer forma. O tipo 0xFF do bloco é o registro de GPS, não um PID, e por isso não entra na tabela.

A chave estrangeira de um hypertable para uma tabela comum é suportada pelo TimescaleDB, e a verificação é barata porque `pid` tem poucas linhas. Como `valor_bruto` fica guardado, uma correção de fator ou deslocamento pode ser aplicada às leituras antigas pelo reprocessamento.

### Gold: o que a análise de motoristas consome

- **`viagem`**: segmentada a partir dos boots e quebrada em paradas longas com velocidade e RPM zerados. Guarda início absoluto e incerteza, cobertura de blocos, taxa efetiva de amostragem, distância e o trajeto simplificado (`geography(LineString)`, com índice GiST), que atende às consultas por região.
- **`amostra_alinhada`**: grade regular por viagem (padrão de 250 ms), uma coluna por PID e, para cada valor, a idade da leitura de origem. Instantes com idades muito diferentes entre velocidade e RPM são descartados no cálculo de marcha.
- **`viagem_metricas`**: distribuições de RPM, velocidade e pedal, acelerações e frenagens bruscas por 100 km, tempo em lugging, consumo estimado via MAF e histograma de marchas. Os agregados por motorista saem daqui, como continuous aggregates.

## Tempo absoluto

**A hora real de cada leitura sai de uma reta por boot, guardada na própria tabela boot e recalculada pelo worker sempre que chega uma âncora nova.** Nenhum horário é gravado nas leituras: uma âncora que chega depois dos dados melhora o passado sem reescrever chunks comprimidos. Tudo isso é do servidor; o dongle só manda o próprio t\_ms na resposta ao desafio.

### Uma âncora

O servidor anota S1 ao devolver o desafio e S2 ao receber a resposta, e o dongle leu t\_ms entre os dois instantes. Então o deslocamento do boot (hora real menos t\_ms) fica limitado por:

```latex
S_1 - t_{ms} \;\le\; o_{boot} \;\le\; S_2 - t_{ms}
```

Só entram âncoras com S2 − S1 de até 2 s; acima disso o servidor descarta a âncora e pede outra no ACK, como define o documento do protocolo.

### Cruzar âncoras próximas

Âncoras a menos de 10 min umas das outras formam um grupo. O deslocamento do grupo é a interseção dos intervalos: o maior dos limites inferiores e o menor dos superiores. Em 10 min, a deriva do cristal fica abaixo de 24 ms, desprezível diante de janelas de centenas de milissegundos.

| Âncora | S1 | S2 | t\_ms | Deslocamento | Incerteza |
| --- | --- | --- | --- | --- | --- |
| A | 10:00:00,000 | 10:00:00,800 | 600.000 | 09:50:00,000 a 09:50:00,800 | ±400 ms |
| B | 10:05:00,400 | 10:05:01,000 | 900.000 | 09:50:00,400 a 09:50:01,000 | ±300 ms |
| A ∩ B |  |  |  | 09:50:00,400 a 09:50:00,800 | ±200 ms |

Uma interseção vazia indica deriva maior que a prevista ou um salto no relógio do servidor (por exemplo, um ajuste do NTP). Nesse caso o worker não cruza: mantém as âncoras em grupos separados e registra um alerta.

### Reta entre grupos

Grupos distantes não podem ser cruzados, porque a deriva de até 40 ppm move o deslocamento em até 144 ms por hora. Entre grupos, o worker ajusta uma reta por mínimos quadrados sobre os centros, com peso inversamente proporcional ao quadrado da meia largura de cada grupo:

```latex
o(t_{ms}) = a + b \cdot t_{ms} \qquad T = t_{ms} + a + b \cdot t_{ms}
```

Com um só grupo, b = 0 e a é o centro do grupo. Um |b| acima de 40 ppm também gera alerta. O valor de 40 ppm é o mesmo dos documentos do protocolo; vale medir o cristal da placa e ajustá-lo.

### Hora e incerteza de cada leitura

A hora sai de um join por boot, e não de uma busca de âncora para cada leitura:

```sql
SELECT l.*,
       to_timestamp((bo.a_ms + (1 + bo.b) * l.t_ms) / 1000.0) AS ts,
       bo.origem_tempo
FROM leitura l
JOIN boot bo USING (dongle_id, boot_id)
WHERE bo.a_ms IS NOT NULL;
```

A incerteza de uma leitura é a meia largura do grupo mais próximo mais 40 ppm vezes a distância até ele:

```latex
u(t_{ms}) = \tfrac{1}{2}(o_{max} - o_{min})_{grupo} + 40 \times 10^{-6} \cdot |t_{ms} - t_{centro}|
```

Como os grupos não são guardados, o worker calcula essa incerteza no mesmo passo em que monta os grupos, e grava o resultado por viagem (início e fim), não em cada consulta.

### Boot sem âncora

Um boot que terminou sem handshake não tem âncora e não pode ganhar uma depois. Como o seq ordena os boots, ele fica entre o último registro ancorado do boot anterior e o primeiro do boot seguinte. O worker grava essa janela nas colunas de tempo de boot, com origem\_tempo 'vizinhos', o centro em a\_ms, b = 0 e a meia largura (que pode ser de horas) em incerteza\_base\_ms. As viagens desse boot aparecem com a flag de hora incerta, em vez de sumir das análises.

### Quando recalcular

1. No POST response, a API grava a âncora em ancora\_tempo e marca o boot como pendente.
2. O worker remonta em memória os grupos de âncoras desse boot, atualiza as colunas de tempo em boot e, em seguida, a camada gold dele.
3. Um boot sem âncora é recalculado quando um dos vizinhos ganha âncora.

## Comparação entre motoristas

As métricas só são comparáveis se forem normalizadas e filtradas por qualidade.

- **Amostragem diferente por veículo.** Com Tmax = 200 ms o RPM sai a 1 Hz; com 50 ms, a 4 Hz. Use métricas por distância ou por tempo em movimento e prefira percentis a contagens brutas.
- **Efeito do veículo.** Faixa de RPM e relação de marchas dependem do carro. Compare motoristas dentro do mesmo veículo ou modelo sempre que possível.
- **Flags de qualidade até a camada gold.** Lacunas de seq, períodos em modo diagnóstico (A3.2), viagem sem âncora e GPS suspeito permitem excluir ou ponderar viagens.
- **GPS vem do app.** O dongle assina a posição recebida por `AT#GPS`, mas não a mede. Valide a distância do GPS contra a integral da velocidade do PID 0D.

## Implementação e implantação

O servidor é um serviço em Go, sem estado, com várias réplicas na frente de um único PostgreSQL com TimescaleDB. Verificar assinaturas e decodificar blocos é barato; o limite de escala é a escrita no banco, e o desenho abaixo otimiza justamente isso.

### Onde está o custo

Cada dongle dirigindo gera de 0,1 a 0,17 bloco por segundo, com cerca de 200 registros por bloco (seção 2.6.6 da Investigação-3).

| Dongles dirigindo ao mesmo tempo | Blocos/s | Linhas/s no banco | CPU para as assinaturas |
| --- | --- | --- | --- |
| 100 | 17 | 3,4 mil | desprezível |
| 1.000 | 170 | 34 mil | desprezível |
| 10.000 | 1.700 | 340 mil | menos de 10% de um núcleo |

Verificar a assinatura de um bloco de 1 KB custa poucos microssegundos com HMAC-SHA256 e algumas dezenas com Ed25519 (valores aproximados, a confirmar em benchmark). A estimativa de CPU da tabela usa o caso mais caro. Já 340 mil linhas por segundo exigem `COPY` em lote, compressão e uma máquina de banco dedicada. Após um período offline, o dongle envia o acumulado mais rápido que o tempo real, então a capacidade deve ser dimensionada pelos picos de reconexão, não pela média.

### Linguagem

Go atende ao requisito de desempenho e é mais rápido de desenvolver para o prazo do TCC. Goroutines tratam milhares de conexões simultâneas com pouco custo, e a biblioteca padrão já traz `crypto/hmac`, `crypto/sha256`, `encoding/binary` e um servidor HTTP com roteamento. O driver `pgx` faz `COPY` binário, que é o caminho mais rápido para inserir no PostgreSQL.

Rust (tokio, axum, tokio-postgres) seria a alternativa se a latência de cauda ou o uso de memória virassem problema. Para este volume, a diferença de vazão entre as duas linguagens fica abaixo do gargalo do banco.

O decodificador de blocos fica num pacote isolado, testado contra blocos gerados pelo próprio firmware. O mesmo pacote alimenta o simulador de carga.

### Arquitetura

&#91;embedded content: implantação · do dongle ao banco\]

A API e o worker só se encontram no banco: a API marca boots como pendentes, e o worker os processa.

### Endpoints

Os endpoints seguem as mensagens do documento de segurança. No sentido dongle → servidor, tudo é assinado pelo dongle: blocos e respostas a desafios. No sentido servidor → dongle, tudo é assinado pelo servidor: desafio, confirmação de recebimento e token de diagnóstico. O app só transporta bytes; a autenticação do app no servidor serve para identificar o usuário e limitar abusos, nunca para dar confiança aos dados.

| Endpoint | Quando o app chama | Entrada | Saída |
| --- | --- | --- | --- |
| `POST /v1/dongles/{id}/desafio` | Ao conectar com internet; o dongle pede o desafio | DevId, informado pelo app (sem autenticação) | Desafio 0x01: c\_S, N\_S e tag Kauth |
| `POST /v1/dongles/{id}/desafio/resposta` | Depois que o dongle responde ao desafio | Resposta 0x02: c\_S, N\_S, TAIL\_seq, HEAD\_seq, `boot_id`, `t_ms` e tag Kauth | ACK 0x04 com ACK\_seq e o token JWT |
| `POST /v1/dongles/{id}/blocos` | Em lotes, a partir da última confirmação | Lote de blocos assinados | Confirmação assinada com `seq_contiguo` |
| `GET /v1/dongles/{id}/confirmacao` | Ao reconectar, antes de enviar | — | A confirmação assinada mais recente |
| `POST /v1/dongles/{id}/token-diagnostico` | Antes de entrar no modo diagnóstico | Pedido assinado com a duração desejada | Token assinado com duração limitada, ou recusa |

Corpos em binário (`application/octet-stream`), repassados pelo app sem conversão. Converter para JSON no app abriria espaço para alterações e custaria CPU no servidor.

**Desafio e âncora de tempo.** O servidor anota S1 no momento em que devolve o desafio e guarda o nonce junto. Na resposta, confere que o nonce está pendente, no prazo e não foi usado, e anota S2. Se S2 − S1 for de até 2 s, grava a âncora (boot\_id, t\_ms, S1, S2) em ancora\_tempo e marca o boot como pendente para o worker, que recalcula a hora real do boot (Tempo absoluto). Um nonce de uso único impede que um app reaproveite uma troca antiga para fabricar uma âncora.

**Confirmação.** A confirmação assinada impede que um app malicioso forje um ACK e faça o dongle apagar blocos nunca recebidos. Como o app pode omitir confirmações (A3.3), o `GET` permite pedi-la de novo a qualquer momento; o dongle só descarta blocos cobertos por uma confirmação válida.

**Token de diagnóstico.** Cada token emitido é registrado. Assim, um intervalo sem dados coberto por um token vira um período legítimo de diagnóstico; sem token, vira uma lacuna suspeita (A3.2).

**Respostas de erro.** `400` para lote malformado, `422` para assinatura inválida (o bloco vai para a tabela de rejeitados), `409` para nonce já usado ou expirado e `503` com `Retry-After` sob sobrecarga.

### Ingestão de um lote

1. Ler o lote e separar os blocos.
2. Verificar as assinaturas em paralelo, com comparação em tempo constante quando for MAC. Blocos inválidos vão para a tabela de rejeitados.
3. Abrir uma transação e tomar `pg_advisory_xact_lock(dongle_id)`. Lotes do mesmo dongle ficam em série; dongles diferentes seguem em paralelo.
4. Copiar os blocos para uma tabela temporária e inserir em `bloco_bruto` com `ON CONFLICT DO NOTHING`.
5. Decodificar só os blocos novos e gravar `leitura` e `posicao` com `COPY`.
6. Atualizar `dongle_estado` (`seq_contiguo`, `maior_seq_visto` e `head_seq`, se um bloco novo passar dele) e marcar o boot como pendente para a camada gold.
7. Fazer o commit e devolver a confirmação assinada.

Um lote por transação, em vez de um bloco, divide o custo do commit por dezenas de blocos. Sob sobrecarga, a API limita as requisições em andamento e responde `503` com `Retry-After`. Como a ingestão é idempotente e o dongle reenvia a partir do ACK, recusar um lote nunca perde dados.

### Handshake

O handshake são duas requisições do app, e o servidor guarda o estado entre elas na tabela `desafio`. Ele serve para quatro coisas: provar que a resposta do dongle é nova (c\_S e N\_S), medir a âncora de tempo do boot (S1, S2 e o `t_ms` do dongle), receber HEAD\_seq e TAIL\_seq atestados pelo dongle e emitir o token JWT que autoriza o envio de blocos. Desafio, resposta e ACK são autenticados com Kauth.

No `POST /v1/dongles/{id}/desafio`:

1. Ler o DevId informado pelo app. O pedido não é autenticado, porque o JWT ainda não existe. Qualquer um pode pedir um desafio para qualquer dongle, e isso é seguro: só o dongle com Kauth consegue responder.
2. Limitar a taxa por DevId: se ele já pediu desafios demais no último minuto, responder `429`. Sem isso, um app malicioso encheria a tabela `desafio`.
3. Obter c\_S com `UPDATE dongle_estado SET ultimo_c_s = ultimo_c_s + 1 ... RETURNING ultimo_c_s`. O incremento atômico mantém c\_S estritamente crescente mesmo com pedidos simultâneos, e o dongle recusa um desafio cujo c\_S não seja maior que o último que aceitou.
4. Gerar N\_S com 16 bytes aleatórios (`crypto/rand`).
5. Anotar S1, inserir a linha em `desafio` e responder o Desafio 0x01 (c\_S, N\_S e tag Kauth).

No `POST /v1/dongles/{id}/desafio/resposta`:

1. Anotar S2 no instante em que a requisição chega, antes de qualquer outro processamento.
2. Ler a Resposta 0x02 (c\_S, N\_S, TAIL\_seq, HEAD\_seq, `boot_id` e `t_ms`) e verificar a tag com a Kauth do dongle. Se for inválida, marcar o desafio como `tag_invalida` e responder `422`.
3. Consumir o desafio com o `UPDATE` mostrado em Contexto, passando S2. Se nenhuma linha voltar, responder `409`.
4. Conferir a consistência: TAIL\_seq de até HEAD\_seq + 1 e HEAD\_seq não menor que o `head_seq` salvo. Se falhar, marcar como `inconsistente` e responder `422`.
5. Conferir se S2 − S1 é de até 2 s. Se for maior, o handshake segue, mas sem âncora: o desafio fica como `aceito_sem_ancora`, e o ACK pede um novo desafio.
6. Abrir uma transação e tomar `pg_advisory_xact_lock(dongle_id)`. Aplicar a regra do TAIL\_seq: se TAIL\_seq > `seq_contiguo` + 1, os blocos de `seq_contiguo` + 1 a TAIL\_seq − 1 foram descartados pelo dongle com a flash cheia. A API grava essa faixa em `perda_ini` e `perda_fim` e avança `seq_contiguo` para TAIL\_seq − 1. Em seguida, atualiza `head_seq`, `tail_seq` e `head_relatado_em` em `dongle_estado`, grava a âncora quando houver, cria a linha de `boot` se ainda não existir e marca o boot como pendente para o worker.
7. Fazer o commit, avisar o worker com `NOTIFY`, emitir o JWT (`sub` = DevId, com o `jti` gravado em `desafio`) e responder o ACK 0x04, com o ACK\_seq resultante e a tag Kauth, junto com o token.

S1 e S2 vêm do relógio da API (`time.Now()`), sincronizado por NTP, e não do `now()` do PostgreSQL. O `now()` do banco devolve o início da transação, que pode estar atrasado em relação ao momento real. S1 é anotado antes do `INSERT`, um pouco antes de a resposta sair. Isso só alarga o intervalo \[S1, S2\], então a âncora fica mais conservadora, mas nunca errada. S2 é anotado na chegada pelo mesmo motivo: o tempo gasto verificando a assinatura não pode entrar na medida.

O pedido de desafio não toma o lock do dongle, mas o passo 6 da resposta toma. A regra do TAIL\_seq pode avançar `seq_contiguo`, e essa mudança não pode se misturar com um lote do mesmo dongle sendo gravado ao mesmo tempo. O TAIL\_seq é autenticado com Kauth e amarrado a N\_S, então o app não consegue forjar uma perda. Ele pode provocá-la, deixando de pedir lotes (A3.2), mas a perda chega ao servidor declarada, com a faixa exata.

### Worker da camada gold

O mesmo binário roda em modo worker. Ele busca boots pendentes com `FOR UPDATE SKIP LOCKED`, recalcula viagens, amostras alinhadas e métricas, e libera o boot. Várias instâncias podem rodar sem coordenação, porque cada boot é travado por quem o pegou.

### Implantação

Para o TCC, um Docker Compose numa única VM resolve:

| Serviço | Imagem | Papel |
| --- | --- | --- |
| proxy | Caddy | TLS automático e balanceamento entre réplicas |
| api | binário Go | Desafios, ingestão, confirmações e tokens; 2 ou mais réplicas |
| worker | mesmo binário Go | Camada gold |
| banco | timescale/timescaledb-ha | PostgreSQL com TimescaleDB e PostGIS |
| painel (opcional) | Grafana | Visualização das análises |

Backups do banco com pgBackRest, e métricas da API (blocos/s, latência da confirmação, assinaturas rejeitadas) expostas para Prometheus. A chave de assinatura do servidor fica fora do banco, num cofre de segredos, para que um invasor do banco (adversário 5) não consiga emitir confirmações ou tokens.

Para crescer, a API escala horizontalmente sem mudanças, porque não guarda estado. O banco escala primeiro verticalmente; depois, a tabela `leitura` pode migrar para ClickHouse, mantendo bronze e contexto no PostgreSQL.

### Teste de carga

Um simulador em Go gera blocos assinados de milhares de dongles virtuais, incluindo reconexões com acumulado offline. Ele mede blocos por segundo aceitos, latência do ACK e o ponto em que a API começa a responder `503`. Esses números substituem as estimativas da tabela acima no texto final do TCC.

## Segurança e premissas

Contra o invasor do servidor (adversário 5), `bloco_bruto` é append-only para o usuário da aplicação. Como reforço, os blocos aceitos podem ser encadeados por hash, com o hash publicado periodicamente fora do banco. Blocos com assinatura inválida vão para uma tabela de rejeitados, nunca para as camadas seguintes.

Premissas adotadas:

- O formato em blocos da seção 2.6.4 é o formato final, com seq por bloco.
- Cada bloco é assinado pelo dongle, e o servidor verifica a assinatura com a chave daquele dongle. Não há chave de sessão entre dongle e servidor; a sessão com chave derivada existe só no enlace BLE entre app e dongle.
- Os formatos das mensagens seguem a versão atual do documento de segurança: Desafio 0x01, Resposta 0x02 (50 bytes, com TAIL\_seq), Bloco 0x03 e ACK 0x04, com Kauth para o handshake e Krec para os blocos.
- A configuração de cada boot chega ao servidor. Se o protocolo atual não a envia, ela deve ser incluída na resposta ao desafio.
- A atribuição de viagens a motoristas vem de fora (frota ou seguradora), não do dongle, e não faz parte deste modelo.
- A grade da `amostra_alinhada` é de 250 ms, que corresponde ao menor período previsto (5 Tmax com Tmax = 50 ms).
