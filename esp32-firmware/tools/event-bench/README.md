# Benchmark com e sem segurança

Protocolo da spec `fase-1/06` §7. Os três perfis de segurança têm um env cada
(`collector_mock_bench_{none,hmac,asym}`, `collector_twai_bench_*` para o carro):

```
pio run -e collector_mock_bench_hmac -t upload
python capture.py --port COM5 --out logs/collector_mock_bench_hmac-regime-<data>.log
python bench_report.py logs/*.log --out-dir report
```

`capture.py` guarda só as linhas `EV ...` com o carimbo do PC; `bench_report.py`
gera `bench.csv` e `bench.md` (média, p50, p95, máx. e n de cada métrica B1 a B12
que depende só do log). Dependências: `pyserial` (captura); o relatório usa só a
biblioteca padrão. Os cenários (regime, estresse, handshake, vazão) estão no
protocolo da spec; registrar o modelo do celular e o MTU (`SYNC_MTU`).
Roteiro passo a passo: `docs/roteiro-benchmark-seguranca.md`.

## Contagem de blocos no log da flash

`contar_blocos.py` reconstrói, a partir de um log do `capture.py`, quantos blocos
o log da flash guarda e a ocupação em relação ao limite N (`BOOT log_max`), pelos
eventos `SEG_NEW`, `FLUSH ok=1`, `SEG_DROP` e `SYNC_FREE`. Comece a captura logo
depois de um wipe. Sai com código 1 se um `SEG_DROP` não bater com a contagem.
Roteiro: `docs/roteiro-teste-limite-blocos.md`.

```
python contar_blocos.py logs/limite.log --a-cada 10
```
