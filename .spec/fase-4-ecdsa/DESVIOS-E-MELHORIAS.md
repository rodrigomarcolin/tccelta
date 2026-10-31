# Fase 4 — desvios e melhorias

Estado da implementação em relação às specs `01`–`05`.

## Desvios

- **01 — teste no alvo de `ecdsa_tag`.** `EcdsaBlockAuth::sign(bloco 0) == ecdsa_tag` não foi
  escrito como teste Unity no alvo: as classes mbedTLS não rodam no `native` e não havia placa à mão.
  O vetor está em `vectors.json`/`vectors.h` (`VEC_BLK_*_ECDSA_TAG`) e o Go o verifica
  (`TestECDSAFirmwareVectors`). **Pendente de bancada:** com a placa `collector_mock_debug_asym`,
  conferir a assinatura de um `FLUSH` com `tools/provision/verify_block.py --record kms_record.json --event -`
  (lê `hex=`/`tag=` do monitor). A igualdade byte a byte do bloco-vetor continua por fazer.
- **01 — `ecdsa_tag` em todos os blocos do vetor** (`boot` e `loss`), não só no bloco 0; `k_sig` e
  `k_sig_public` já eram campos de topo do `vectors.json`.
- **03 — par de chaves por `crypto/ecdh`.** `NewDongle` gera `k_sig` com `ecdh.P256().GenerateKey`
  (escalar de 32 B big-endian e ponto não comprimido de 65 B), em vez de `ecdsa.GenerateKey`; é o
  mesmo par, sem converter `big.Int`. O depguard do KMS continua só com a biblioteca padrão.
- **04 — `rejected_block` conferido por contagem.** O `Inspector` não expõe o motivo da linha; o cenário
  `asym` confere `CountRejected == 1` e o `X-Lote-Motivo: tag_invalida` da resposta (422, sem progresso).
- **04 — handshake com espera em 429.** O cenário `asym` termina em décimos de segundo e bate no
  limite de desafios por IP ao passar de um dongle para o outro; ele repete o handshake respeitando o
  `Retry-After`.
- **04 — sem checagem automática de "zero chamadas a `/v1/blocks/verify`".** O cenário não olha o
  KMS. O critério vale por construção (o despacho do `asym` não chama `VerifyBlocks`, coberto por
  `TestAsymBatchAcceptedWithoutKMSVerify`) e pode ser conferido em `tccelta_kms_calls_total{operation="verify_rec"}`.
- **05 — `IMessageAuth::lastError()`.** Para o campo `rc` do `SIGN_FAIL`, a interface ganhou
  `int lastError() const` (0 por padrão); só o `EcdsaBlockAuth` o preenche.
- **Novo — `prod` e `devsec` aceitam `asym`.** O plano original (`README.md`) só previa `asym` em
  bancada (`*_debug_asym`, `*_bench_asym`); com a ingestão verificando `asym` localmente (02) e sem
  segredo de KMS no caminho, não há razão para restringir produção a `hmac`. `collector_twai_prod_asym`
  (mesmo `sdkconfig.prod.defaults`, só troca `SECURITY_PROFILE`) e o `static_assert` de
  `KeyStore.cpp` (guardado por `CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE`) passam a aceitar
  `hmac` ou `asym`; só `none` continua proibido em flash encryption *release*. `provision.py --profile
  prod --security asym` passa a ser aceito (antes só `hmac`). `collector_mock_devsec_asym` ensaia o
  mesmo perfil na placa descartável (spec 01 §10.2) antes de queimar eFuse de verdade em `prod`.
- **Novo — limite do corpo do lote com a tag do `asym`.** O `limitBody` de `POST /v1/dongles/blocos`
  usava 1059 B por bloco (tag de 32 B do `hmac`): 32 blocos `asym` reais (1022 B de dados, 34 848 B)
  voltavam `413 corpo_grande` na bancada. Agora é `LOTE_MAX_BLOCOS·protocol.MaxBlockTLVSize + 64`, com
  1091 B por bloco. O e2e não pegou porque os blocos do simulador são menores.
- **Novo — `provision.py` ↔ `import-record`.** O envelope do firmware nunca tinha sido importado pelo
  KMS e não importava: no `alg none` o Go lia `key_hex`, campo que o `provision.py` não grava (o
  material vai em `ciphertext_b64`, como na spec `fase-1/01`); com `--kms-wrapping`, o script esperava
  PEMs soltos em vez do `kms_wrapping.json` do `export-wrapping` (`{kid, pem}`) e calculava o `kid` sobre
  o texto do PEM, não sobre o DER. Os dois lados seguem agora a spec, e
  `TestImportRecordFromProvisionPy*` importa envelopes gerados pelo próprio `provision.py`.

## Verificado, mas não em hardware

`pio run` dos ambientes `*_none`/`*_hmac`/`*_asym` não foi concluído nesta máquina: o link do
`collector_mock_debug_none` falha por símbolos `fs::LittleFSFS` (biblioteca LittleFS do Arduino,
sem relação com a assinatura); a compilação das fontes alteradas passa. Os testes `native` do
`test_block_format` e do `test_sync_engine` cobrem a falha de assinatura. O `test_persistence` já
falhava antes (`TmaxMeter` esperando 99 amostras depois do commit que as reduziu para 15).

## Melhorias

- Chave pública guardada ou verificada no KMS em vez da linha `dongle` (D6).
- Verificação ECDSA no servidor de bancada do app (fora do escopo desta fase).
- Rotação de `k_sig`.
- Handshake assimétrico.
