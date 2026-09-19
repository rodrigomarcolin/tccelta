# test-scripts

Scripts Python de teste hardware-in-the-loop (HIL) para este projeto —
rodam num computador com acesso físico (serial/BLE) aos dispositivos, fora
do firmware/app propriamente ditos.

## Setup

Ambiente conda `tccelta` (ou qualquer venv Python 3.10+):

```bash
pip install pyserial bleak
```

## Scripts

### `hil_dongle_dtc.py`

Valida a leitura de DTC (Modo 03/07/0A) ponta-a-ponta: injeta cenários de
DTC no `simulador/ECUSim` via canal serial de controle
(`DTCUpdateSerialControl.ino`) e consulta o dongle real via BLE (Nordic
UART Service), como um app faria.

Pré-requisitos de hardware:
- Arduino rodando `simulador/ECUSim`, conectado por CAN ao ESP32, numa
  porta serial (`--com`, default `COM8`).
- ESP32 com o firmware `esp32-firmware` gravado no profile `-e twai`
  (**sem** `_psk`/`_handshake` — este script fala BLE em texto puro; para
  testar contra um profile cifrado seria necessário implementar o wire
  protocol de `SecurePskBleConnectivity` no lado do script, o que não foi
  feito aqui).

```bash
python hil_dongle_dtc.py --com COM8 --scenario limpo
python hil_dongle_dtc.py --com COM8 --scenario falha_unica
python hil_dongle_dtc.py --com COM8 --scenario multiframe
python hil_dongle_dtc.py --com COM8 --scenario multi_ecu
python hil_dongle_dtc.py --ble-only   # reconsulta o dongle sem reinjetar DTC
```

Ver o docstring do próprio arquivo para o detalhe do protocolo serial e o
significado de cada cenário.
