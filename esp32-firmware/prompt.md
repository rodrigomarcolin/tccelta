Você está desenvolvendo um dongle OBD2 embarcado para ESP32, usando o framework Arduino com FreeRTOS e PlatformIO. O objetivo é uma arquitetura limpa, desacoplada, baseada em interfaces, filas FreeRTOS e injeção de dependência em tempo de build via PlatformIO.

Visão geral da arquitetura
A arquitetura é organizada em camadas verticais, onde cada camada depende apenas da interface da camada inferior, nunca de implementações concretas. As camadas, de baixo para cima, são:

1. Camada CAN (can/)

Interface abstrata ICanBus com métodos como begin(), sendFrame(), receiveFrame().
Implementação Mcp2515Can — usa o chip externo MCP2515 via SPI.
Implementação TwaiCan — usa o periférico TWAI nativo do ESP32 com transceiver SN65HVD230.
Apenas uma das implementações é compilada por vez, determinada pelo build profile ativo.
Nesta pasta, há um arquivo MCP2515 com uma prova de conceito co mde

2. Camada OBD2 (obd2/)

Interface abstrata IObd2 com métodos como requestPid(), readPid().
Implementação Obd2Mock — retorna dados simulados (usada para desenvolvimento/testes sem hardware CAN real).
Implementação Obd2Can — utiliza ICanBus injetado via construtor para comunicação real com o barramento CAN.
Apenas uma das implementações é compilada por vez.

3. Camada ELM327 (elm327/)

Classe Elm327 recebe strings de comando (ex: "ATZ", "0100") e as interpreta conforme o protocolo ELM327.
Delega requisições OBD2 para IObd2 injetado via construtor.
Retorna strings de resposta formatadas no padrão ELM327.

4. Camada de Conectividade (connectivity/)

Interface abstrata IConnectivity com métodos como begin(), sendResponse(), setOnCommandReceivedCallback(callback).
Implementação BleConnectivity — usa BLE (Bluetooth Low Energy) do ESP32.
Mapeamento de serviços/características BLE ainda a ser definido; use placeholders com comentários // TODO: definir UUIDs.


Comunicação entre camadas: FreeRTOS Queues
Use filas FreeRTOS (QueueHandle_t) para desacoplar os módulos que rodam em tasks separadas. A comunicação deve seguir este fluxo assíncrono:
BleConnectivity → [fila commandQueue] → Elm327Task → IObd2 → ICanBus
                                                          ↓
BleConnectivity ← [fila responseQueue] ←──────────────────

Cada módulo que precisa de comunicação assíncrona deve expor as filas necessárias ou recebê-las via injeção.
Defina structs tipadas para as mensagens das filas (ex: CommandMessage, ResponseMessage).


Injeção de dependência em tempo de build (PlatformIO)
No platformio.ini, defina environments (profiles) que selecionam os arquivos-fonte corretos. Use a diretiva src_filter para incluir apenas os arquivos da implementação escolhida. Exemplo de estrutura esperada:
ini[env:mcp2515_real]
build_flags = -DUSE_MCP2515 -DUSE_OBD2_REAL
src_filter = +<*> -<can/twai/> -<obd2/mock/>

[env:twai_real]
build_flags = -DUSE_TWAI -DUSE_OBD2_REAL
src_filter = +<*> -<can/mcp2515/> -<obd2/mock/>

[env:mock_dev]
build_flags = -DUSE_MOCK -DUSE_OBD2_MOCK
src_filter = +<*> -<can/> -<obd2/real/>
A instanciação das implementações concretas e o wiring das dependências devem ocorrer em um único arquivo de composição (ex: main.cpp ou AppFactory.h), usando #ifdef baseados nas build_flags.

Estrutura de diretórios esperada
src/
  can/
    ICanBus.h
    mcp2515/
      Mcp2515Can.h
      Mcp2515Can.cpp
    twai/
      TwaiCan.h
      TwaiCan.cpp
  obd2/
    IObd2.h
    mock/
      Obd2Mock.h
      Obd2Mock.cpp
    real/
      Obd2Can.h
      Obd2Can.cpp
  elm327/
    Elm327.h
    Elm327.cpp
  connectivity/
    IConnectivity.h
    ble/
      BleConnectivity.h
      BleConnectivity.cpp
  tasks/
    Elm327Task.h
    Elm327Task.cpp
  main.cpp
platformio.ini

Requisitos e restrições técnicas

Linguagem: C++17 (suportado pelo toolchain ESP32 Arduino)
Framework: Arduino + FreeRTOS (já integrado no ESP32 Arduino)
Sem RTTI, sem exceções (-fno-rtti -fno-exceptions são padrão no ESP32)
Use ponteiros crus ou std::unique_ptr para ownership; evite std::shared_ptr em ISRs e contextos de task FreeRTOS por segurança
Interfaces devem usar destrutor virtual
Nenhuma implementação concreta deve ser incluída em cabeçalhos de outras camadas
O firmware final de cada profile deve compilar sem warnings e sem símbolos de implementações não utilizadas


O que deve ser gerado

Todos os arquivos .h e .cpp da estrutura acima com implementações stub funcionais (mas compiláveis)
O platformio.ini completo com os três environments descritos
Um main.cpp com o wiring correto usando #ifdef para cada profile
Comentários // TODO nos pontos onde decisões ainda estão pendentes (ex: UUIDs BLE, PID list OBD2)
Um README.md descrevendo a arquitetura, os profiles disponíveis e como adicionar novas implementações


Comece pela estrutura de diretórios e interfaces, depois as implementações stub, e por fim o platformio.ini e main.cpp.
