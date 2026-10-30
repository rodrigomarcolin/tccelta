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
