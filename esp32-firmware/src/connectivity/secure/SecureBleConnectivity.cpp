#include <cstring>
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include "SecureBleConnectivity.h"

/**
 * Decorador para IConnectivity.
 *
 * Encapsula qualquer IConnectivity*, interceptando os métodos sendResponse() e
 * setOnCommandReceivedCallback() para aplicar autenticação ou encriptação
 * de maneira transparente.
 *
 * Intercepta a mensagem recebida, colocando-a na fila para desencriptação antes de repassar
 * para o callback EML327 registrado.
 * 
 */

SecureBleConnectivity::SecureBleConnectivity(IConnectivity* inner) : _inner(inner) {
    _rawQueue = xQueueCreate(kSecureQueueDepth, sizeof(RawMessage));
    configASSERT(_rawQueue);
}

/**
 * Chama o begin() da classe interna, então determina o callback
 * que colocará o comando na fila para ser desencriptado, e cria 
 * a task de desencriptação.
 */
bool SecureBleConnectivity::begin() {
    if (!_inner->begin()) return false;

    // callback lightweight; copia dados raw e coloca na fila.
    _inner->setOnCommandReceivedCallback(
        [this](const uint8_t* data, size_t len) {
            RawMessage msg;
            msg.len = (len <= kSecureMaxMsgLen) ? len : kSecureMaxMsgLen;
            memcpy(msg.buf, data, msg.len);
            BaseType_t woken = pdFALSE;
            xQueueSendFromISR(_rawQueue, &msg, &woken);
            portYIELD_FROM_ISR(woken);
        }
    );

    xTaskCreate(
        secureTaskEntry,
        "SecureTask",
        kSecureTaskStack,
        this,
        kSecureTaskPriority,
        &_taskHandle
    );
    configASSERT(_taskHandle);
    return true;
}

/**
 * Armazena o callback registrado (ex. o do Elm327Task).
 * A task de desencriptar chamará o _userCallback com um plaintext.
 */
void SecureBleConnectivity::setOnCommandReceivedCallback(CommandCallback cb) {
    _userCallback = cb;
}

/**
 * Encripta/assina dados antes de encaminhar para o transporte interno
 * É chamado no contexto de uma task, então pode conter "heavy work"
 */
void SecureBleConnectivity::sendResponse(const uint8_t* data, size_t len) {
    // TODO: encrypt / sign data, write result into encBuf / encLen
    const uint8_t* encBuf = data;
    size_t         encLen = len;
    _inner->sendResponse(encBuf, encLen);
}

void* SecureBleConnectivity::getSessionContext() {
    // TODO: return a populated SessionContext with auth token / session key
    return nullptr;
}


void SecureBleConnectivity::secureTaskEntry(void* arg) {
    static_cast<SecureBleConnectivity*>(arg)->secureTask();
}

void SecureBleConnectivity::secureTask() {
    // Tarefa que reagirá ao enfileiramento de um comando recebido pelo Dongle.
    // TODO: Renomear e implementar.
    RawMessage msg;
    while (true) {
        if (xQueueReceive(_rawQueue, &msg, portMAX_DELAY) == pdTRUE) {
            // TODO: decrypt msg.buf / msg.len → plain / plainLen
            const uint8_t* plain    = msg.buf;
            size_t         plainLen = msg.len;

            if (_userCallback) {
                // Após ter o plaintext e verificar autenticidade, repassa-o para o userCallback registrado
                // que, no nosso caso, colocará o plaintext na fila para ser processado pelo ELM327 =)
                _userCallback(plain, plainLen);
            }
        }
    }
}

SecureBleConnectivity::~SecureBleConnectivity() {
    if (_taskHandle) {
        vTaskDelete(_taskHandle);
    }

    if (_rawQueue) {
        vQueueDelete(_rawQueue);
    }
}