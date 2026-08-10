#include <cstring>
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include "SecureBleConnectivity.h"

// ── Constructor / Destructor ──────────────────────────────────────────────────

SecureBleConnectivity::SecureBleConnectivity(IConnectivity* inner) : _inner(inner) {
    _rawQueue = xQueueCreate(kSecureQueueDepth, sizeof(RawMessage));
    configASSERT(_rawQueue);
}

SecureBleConnectivity::~SecureBleConnectivity() {
    if (_taskHandle) {
        vTaskDelete(_taskHandle);
        _taskHandle = nullptr;
    }
    if (_rawQueue) {
        vQueueDelete(_rawQueue);
        _rawQueue = nullptr;
    }
}

// ── IConnectivity implementation 

/**
 * Calls begin() on the inner transport, then installs a lightweight BLE
 * callback that enqueues every raw incoming frame (ISR-safe), and spawns the
 * processing task that will drain the queue and invoke onRawFrameReceived().
 */
bool SecureBleConnectivity::begin() {
    if (!_inner->begin()) return false;

    // Install inner callback — runs in the BLE stack context (ISR-like).
    // Must be fast: only copies bytes and enqueues; no heavy work here.
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
 * Stores the user callback (e.g. registered by Elm327Task).
 * deliverPlaintext() will invoke it once a frame is successfully decrypted.
 */
void SecureBleConnectivity::setOnCommandReceivedCallback(CommandCallback cb) {
    _userCallback = cb;
}

/**
 * Encrypts/frames plaintext via the subclass hook sendSecured(), which in turn
 * calls sendRaw() to push the result to the inner transport.
 */
void SecureBleConnectivity::sendResponse(const uint8_t* data, size_t len) {
    sendSecured(data, len);
}

void* SecureBleConnectivity::getSessionContext() {
    // Overridable by subclasses that maintain session state.
    return nullptr;
}

// ── Protected helpers 

/**
 * Forwards authenticated plaintext to the user-registered callback.
 * Called by subclass implementations of onRawFrameReceived() after successful
 * decryption/authentication.
 */
void SecureBleConnectivity::deliverPlaintext(const uint8_t* plain, size_t len) {
    if (_userCallback) {
        _userCallback(plain, len);
    }
}

/**
 * Pushes already-encrypted bytes to the inner transport (BleConnectivity).
 * Called by subclass implementations of sendSecured().
 */
void SecureBleConnectivity::sendRaw(const uint8_t* data, size_t len) {
    _inner->sendResponse(data, len);
}

// ── FreeRTOS task 

void SecureBleConnectivity::secureTaskEntry(void* arg) {
    static_cast<SecureBleConnectivity*>(arg)->secureTask();
}

/**
 * Processing task: blocks on _rawQueue and dispatches each frame to the
 * concrete subclass via onRawFrameReceived(). The subclass performs decryption
 * and calls deliverPlaintext() on success; it silently drops the frame on any
 * authentication failure.
 */
void SecureBleConnectivity::secureTask() {
    RawMessage msg;
    while (true) {
        if (xQueueReceive(_rawQueue, &msg, portMAX_DELAY) == pdTRUE) {
            onRawFrameReceived(msg.buf, msg.len);
        }
    }
}