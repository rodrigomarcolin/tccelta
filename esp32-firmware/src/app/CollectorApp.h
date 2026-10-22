#pragma once
#include "collector/CollectorConfig.h"
#include "connectivity/ble/BleConnectivity.h"
#include "obd2/IObd2.h"

namespace app {

/** Cria o backend OBD (CAN real ou mock) para o protocolo da config. */
using BusFactory = IObd2& (*)(const collector::CollectorConfig& config);

/**
 * Monta e inicia a coleta na ordem do plano (§4.8): NVS e boot_id, log e
 * `seq`, barramento, filas, objetos, BLE e tarefas. Os objetos vivem até o
 * Dongle reiniciar. Devolve false se algo essencial não subiu.
 */
bool startCollector(BusFactory makeBus, BleConnectivity& ble);

}  // namespace app
