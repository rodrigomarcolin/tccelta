# sync_bench

Faz o papel de app e de servidor de bancada para validar o servico SYNC do dongle sem o app
(spec fase-1/05 secao 13). Cenarios: handshake, stream, replay, badtag, nosession, badack,
incomplete, corrupt. Veja `python sync_bench.py --help`. Dependencia: bleak (`pip install -r requirements.txt`).

## nus_at

`nus_at.py` envia um comando `AT#` de bancada pelo NUS e imprime a resposta (os `AT#` nao
entram pela serial). So funciona no env `collector_mock_debug_open`, sem pareamento.
Roteiro: `docs/roteiro-teste-limite-blocos.md`.

```
python nus_at.py 'AT#SYNC?'
python nus_at.py 'AT#WIPE'
```
