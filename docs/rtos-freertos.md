# RTOS (FreeRTOS) no firmware

Este documento explica como o firmware deste repositório utiliza um **RTOS (Real-Time Operating System)** — mais especificamente o **FreeRTOS** — para organizar a execução do código de forma concorrente, responsiva e desacoplada. Todos os conceitos são ilustrados com trechos reais do código presente em `src/`.

## Sumário

* [O que é um RTOS e por que usá-lo](#o-que-é-um-rtos-e-por-que-usá-lo)
* [O FreeRTOS no ESP32](#o-freertos-no-esp32)
* [Tasks](#tasks)
* [Filas (Queues)](#filas-queues)
* [Como tasks e filas interagem: o padrão produtor-consumidor](#como-tasks-e-filas-interagem-o-padrão-produtor-consumidor)
* [Bloqueio sem busy-wait](#bloqueio-sem-busy-wait)
* [Back-pressure: o que acontece quando a fila enche](#back-pressure-o-que-acontece-quando-a-fila-enche)
* [Prioridades e stack](#prioridades-e-stack)
* [Resumo do fluxo completo](#resumo-do-fluxo-completo)

---

## O que é um RTOS e por que usá-lo

Um **RTOS** é um sistema operacional voltado a sistemas embarcados que precisa executar várias atividades "ao mesmo tempo" de forma previsível. Diferentemente de um laço único (`loop()`) onde tudo é feito em sequência, um RTOS permite dividir o programa em **tasks** independentes, que o **escalonador (scheduler)** alterna de acordo com prioridades e eventos.

No nosso firmware há duas atividades de naturezas bem diferentes que precisam coexistir:

1. **Receber dados via Bluetooth (BLE)** — acontece de forma assíncrona, dirigida por interrupção/callback, e deve responder muito rápido para não atrapalhar a pilha BLE.
2. **Processar o comando ELM327 e consultar o OBD-II** — pode ser mais lento (em hardware real, envolve esperar a resposta da rede CAN do veículo).

Se essas duas atividades fossem feitas no mesmo contexto, o processamento lento bloquearia a recepção BLE. O RTOS resolve isso separando-as em contextos distintos que se comunicam por uma **fila**.

## O FreeRTOS no ESP32

No ESP32 com framework Arduino, o FreeRTOS **já está presente por baixo dos panos**: o próprio `setup()` / `loop()` do Arduino roda dentro de uma task FreeRTOS chamada `loopTask`. Por isso não precisamos "inicializar" o RTOS — basta usar suas primitivas, incluindo os headers correspondentes:

```cpp
// src/tasks/Elm327Task.h:5
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
```

As três primitivas centrais usadas no projeto são:

| Primitiva | Header | Para quê |
|---|---|---|
| **Task** | `freertos/task.h` | Unidade concorrente de execução |
| **Queue** | `freertos/queue.h` | Comunicação/sincronização entre contextos |
| Macros base (`portMAX_DELAY`, `configASSERT`, `UBaseType_t`) | `freertos/FreeRTOS.h` | Tipos e constantes do kernel |

## Tasks

Uma **task** é uma função que roda concorrentemente com as demais, com seu próprio *stack* e prioridade. No projeto, criamos uma task dedicada ao processamento ELM327 em `Elm327Task::start`:

```cpp
// src/tasks/Elm327Task.cpp:22
xTaskCreate(Elm327Task::taskFunc, "Elm327Task",
            stackSize, this, priority, nullptr);
```

Os argumentos de `xTaskCreate` ilustram bem o modelo de task do FreeRTOS:

* `Elm327Task::taskFunc` — a função que o escalonador vai executar.
* `"Elm327Task"` — nome legível (útil em depuração).
* `stackSize` — quantidade de memória de stack reservada para a task (8192 bytes por padrão).
* `this` — o "argumento" passado à task; aqui, o ponteiro do objeto, para que a função estática possa acessar o estado da instância.
* `priority` — prioridade de escalonamento (5 por padrão).

Como `xTaskCreate` exige uma função livre (não um método de instância), usamos um *trampolim* estático que recupera o objeto a partir do argumento `void* arg`:

```cpp
// src/tasks/Elm327Task.cpp:40
void Elm327Task::taskFunc(void* arg) {
    static_cast<Elm327Task*>(arg)->run();
    vTaskDelete(nullptr);  // em teoria, esta linha é inalcançável
}
```

Esse é um padrão idiomático de RTOS em C++: a função estática serve apenas de "porta de entrada" e imediatamente delega para o método `run()`, que contém a lógica real. O `vTaskDelete(nullptr)` libera a task caso `run()` retorne — embora `run()` seja um laço infinito e, na prática, nunca retorne.

### A task como laço infinito

O corpo de uma task de RTOS é, tipicamente, um **laço infinito** que processa eventos. Veja `Elm327Task::run`:

```cpp
// src/tasks/Elm327Task.cpp:45
void Elm327Task::run() {
    CommandMessage msg;
    while (true) {
        if (xQueueReceive(_commandQueue, &msg, portMAX_DELAY) != pdTRUE) continue;

        String cmd(reinterpret_cast<const char*>(msg.data), msg.len);
        String response = _elm327.process(cmd);

        _connectivity->sendResponse(
            reinterpret_cast<const uint8_t*>(response.c_str()),
            response.length());
    }
}
```

Diferente de um `loop()` Arduino que roda "o tempo todo", esta task fica **dormindo** na chamada `xQueueReceive` até que algo chegue na fila (ver [Bloqueio sem busy-wait](#bloqueio-sem-busy-wait)).

> O `loop()` do Arduino, aliás, também é uma task — e neste projeto ele essencialmente cede a CPU para sempre, já que todo o trabalho está na `Elm327Task`:
>
> ```cpp
> // src/main.cpp:63
> void loop() {
>     vTaskDelay(portMAX_DELAY);
> }
> ```

## Filas (Queues)

Uma **fila** é o mecanismo de comunicação *thread-safe* entre contextos do FreeRTOS. Ela transporta cópias de dados de um produtor para um consumidor e, ao mesmo tempo, serve de ponto de sincronização (o consumidor pode dormir esperando a fila).

A fila é criada no construtor da task:

```cpp
// src/tasks/Elm327Task.cpp:8
_commandQueue = xQueueCreate(queueDepth, sizeof(CommandMessage));
configASSERT(_commandQueue);
```

Dois detalhes importantes de RTOS aparecem aqui:

* `xQueueCreate(queueDepth, sizeof(CommandMessage))` — a fila comporta `queueDepth` (8 por padrão) elementos, cada um do tamanho de um `CommandMessage`. A fila guarda **cópias por valor**, não ponteiros — por isso o `CommandMessage` carrega um buffer fixo:

  ```cpp
  // src/tasks/Elm327Task.h:11
  struct CommandMessage {
      uint8_t data[128];
      size_t  len;
  };
  ```

  Copiar por valor evita que o produtor e o consumidor compartilhem memória, eliminando uma classe inteira de bugs de concorrência (ponteiros pendurados, *data races*).

* `configASSERT(_commandQueue)` — verifica que a fila foi de fato alocada; se a memória acabar, o sistema falha de forma controlada em vez de seguir com um ponteiro nulo.

## Como tasks e filas interagem: o padrão produtor-consumidor

Esta é a parte central do uso de RTOS no projeto. Existem **dois contextos de execução distintos** conversando por meio da fila:

* **Produtor** — o callback de recepção BLE, que roda no contexto da pilha BLE (NimBLE).
* **Consumidor** — a `Elm327Task`, que roda na sua própria task.

A fila `_commandQueue` é a fronteira segura entre eles.

```mermaid
flowchart LR
    subgraph Contexto BLE/NimBLE
      A[onWrite RX char] --> B[callback]
      B --> C[pushToQueue]
    end
    C -->|xQueueSend| Q[(_commandQueue<br/>depth = 8)]
    Q -->|xQueueReceive| D
    subgraph Task Elm327Task
      D[run loop] --> E[Elm327::process]
      E --> F[IObd2::readPid]
      F --> G[sendResponse via BLE notify]
    end
```

### Lado produtor (rápido e leve)

Quando o app escreve um comando na característica RX do BLE, o NimBLE chama `onWrite`. Esse contexto **não pode ser bloqueado** por trabalho pesado, então ele apenas registra o evento e o despacha para o callback:

```cpp
// src/connectivity/ble/BleConnectivity.cpp:65
void BleConnectivity::onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) {
    if (!_callback) return;
    std::string val = pChar->getValue();
    _callback(reinterpret_cast<const uint8_t*>(val.data()), val.size());
}
```

O callback registrado pela task apenas **enfileira** o comando — nada mais:

```cpp
// src/tasks/Elm327Task.cpp:15
_connectivity->setOnCommandReceivedCallback([this](const uint8_t* data, size_t len) {
    Serial.print("[ELM327] Command received: ");
    Serial.write(data, len);
    Serial.println();
    this->pushToQueue(data, len);
});
```

Esse princípio — **"o callback deve ser leve e rápido, por isso somente coloca na fila"** — está documentado no próprio código e é uma regra de ouro de programação com RTOS: trabalho demorado nunca deve ser feito no contexto de um callback/interrupção; ele deve ser delegado a uma task via fila.

O enfileiramento de fato:

```cpp
// src/tasks/Elm327Task.cpp:28
void Elm327Task::pushToQueue(const uint8_t* data, size_t len) {
    CommandMessage msg = {};
    msg.len = len < sizeof(msg.data) ? len : sizeof(msg.data) - 1;
    memcpy(msg.data, data, msg.len);
    msg.data[msg.len] = '\0';  // null-terminate for String construction

    xQueueSend(_commandQueue, &msg, 0);  // dropa se a fila estiver cheia
}
```

### Lado consumidor (pode ser lento)

A `Elm327Task` retira o comando da fila, processa (incluindo a consulta OBD-II, que em hardware real pode esperar a rede CAN) e devolve a resposta — tudo no **seu próprio contexto**, sem travar o BLE:

```cpp
// src/tasks/Elm327Task.cpp:48
if (xQueueReceive(_commandQueue, &msg, portMAX_DELAY) != pdTRUE) continue;

String cmd(reinterpret_cast<const char*>(msg.data), msg.len);
String response = _elm327.process(cmd);

_connectivity->sendResponse(/* ... */);
```

O resultado é um **desacoplamento temporal**: o produtor pode receber rajadas de comandos rapidamente, enquanto o consumidor os processa no seu próprio ritmo.

## Bloqueio sem busy-wait

Um dos maiores benefícios do RTOS aparece no argumento `portMAX_DELAY`:

```cpp
// src/tasks/Elm327Task.cpp:48
xQueueReceive(_commandQueue, &msg, portMAX_DELAY)
```

O terceiro parâmetro é o **tempo máximo de espera**. Com `portMAX_DELAY`, a task pede para esperar "para sempre" até que algo chegue. Enquanto a fila está vazia, **o escalonador coloca a task em estado bloqueado e não lhe dá CPU alguma** — ou seja, não há *busy-wait* (não fica um `while` queimando ciclos checando a fila). A CPU fica livre para outras tasks (a pilha BLE, tasks de sistema, etc.) ou pode até entrar em economia de energia.

Quando um item é enfileirado pelo produtor, o kernel automaticamente acorda a task consumidora. Esse é o coração da eficiência de um RTOS dirigido a eventos.

O mesmo mecanismo aparece no `loop()`:

```cpp
// src/main.cpp:63
void loop() { vTaskDelay(portMAX_DELAY); }
```

`vTaskDelay(portMAX_DELAY)` faz a `loopTask` dormir indefinidamente, cedendo a CPU, já que ela não tem trabalho a fazer.

## Back-pressure: o que acontece quando a fila enche

O produtor usa timeout **zero** ao enfileirar:

```cpp
// src/tasks/Elm327Task.cpp:35
xQueueSend(_commandQueue, &msg, 0);  // timeout = 0
```

Isso significa: *"tente enfileirar; se a fila estiver cheia, não espere — descarte"*. Trata-se de uma decisão deliberada de **back-pressure**, comentada no código como *"Dropa se fila cheia (back-pressure, ECU lenta)"*.

A alternativa seria passar um timeout maior que zero, fazendo o produtor **bloquear** até abrir espaço. Mas o produtor roda no contexto do BLE, que não deve bloquear; portanto, descartar o comando excedente é mais seguro do que travar a pilha de comunicação. Esse é um exemplo concreto de *trade-off* de projeto típico de sistemas de tempo real: preferir perder dados a perder responsividade.

## Prioridades e stack

Ao criar a task, dois parâmetros de RTOS são definidos explicitamente:

```cpp
// src/main.cpp:57
task.start(/* priority */ 5, /* stackSize */ 8192);
```

* **Prioridade (5)** — determina a importância relativa da task para o escalonador. Tasks de maior prioridade preemptam as de menor. O valor 5 coloca a `Elm327Task` acima da `loopTask` (prioridade 1), mas abaixo de tasks críticas do sistema/BLE, equilibrando responsividade sem atropelar a comunicação.
* **Stack (8192 bytes)** — cada task tem seu próprio stack. Ele precisa ser grande o suficiente para as chamadas mais profundas da task (aqui, o parsing ELM327 e a manipulação de `String`). Subdimensionar o stack causa *stack overflow*; superdimensionar desperdiça RAM, que é escassa no ESP32.

Esses dois parâmetros têm valores padrão declarados na assinatura, deixando a configuração explícita e ajustável:

```cpp
// src/tasks/Elm327Task.h:44
void start(UBaseType_t priority = 5, uint32_t stackSize = 8192);
```

## Resumo do fluxo completo

Juntando todas as peças, o caminho de um comando, do Bluetooth à resposta, atravessa dois contextos de RTOS unidos por uma fila:

```
 [App BLE]
    │  escreve "010C\r" na característica RX
    ▼
 BleConnectivity::onWrite ─────────────┐  (contexto da pilha BLE — RÁPIDO)
    │  invoca callback                  │
    ▼                                   │
 Elm327Task::pushToQueue               │
    │  xQueueSend(timeout=0)            │
    ▼                                   │
 ╔══════════════════════════╗          │
 ║  _commandQueue (depth 8)  ║  ◄── fronteira thread-safe (cópia por valor)
 ╚══════════════════════════╝          │
    │  xQueueReceive(portMAX_DELAY)     │
    ▼                                   │
 Elm327Task::run ──────────────────────┘  (task própria — pode ser LENTO)
    │  Elm327::process → IObd2::readPid → (CAN/mock)
    ▼
 IConnectivity::sendResponse
    │  notify na característica TX
    ▼
 [App BLE] recebe "41 0C 17 70\r>"
```

Conceitos de RTOS exercitados pelo código:

| Conceito | Onde aparece |
|---|---|
| Criação de task | `xTaskCreate` em `Elm327Task.cpp:22` |
| Trampolim estático → método | `Elm327Task::taskFunc` em `Elm327Task.cpp:40` |
| Task como laço infinito | `Elm327Task::run` em `Elm327Task.cpp:45` |
| Criação de fila | `xQueueCreate` em `Elm327Task.cpp:8` |
| Produtor (enfileira) | `xQueueSend` em `Elm327Task.cpp:35` |
| Consumidor (desenfileira) | `xQueueReceive` em `Elm327Task.cpp:48` |
| Bloqueio sem busy-wait | `portMAX_DELAY` em `Elm327Task.cpp:48` e `main.cpp:63` |
| Back-pressure | `xQueueSend(..., 0)` em `Elm327Task.cpp:35` |
| Prioridade e stack | `task.start(5, 8192)` em `main.cpp:57` |
| Cópia por valor (sem memória compartilhada) | `struct CommandMessage` em `Elm327Task.h:11` |

Em conjunto, esses elementos mostram o uso clássico de um RTOS: **separar responsabilidades em tasks concorrentes e fazê-las cooperar de forma segura e eficiente por meio de filas**, mantendo o sistema responsivo mesmo quando uma parte do trabalho é demorada.
