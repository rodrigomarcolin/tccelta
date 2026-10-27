# sync_bench

Faz o papel de app e de servidor de bancada para validar o servico SYNC do dongle sem o app
(spec fase-1/05 secao 13). Cenarios: handshake, stream, replay, badtag, nosession, badack,
incomplete, corrupt. Veja `python sync_bench.py --help`. Dependencia: bleak (`pip install -r requirements.txt`).
