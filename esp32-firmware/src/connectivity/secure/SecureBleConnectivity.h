#pragma once
#include <cstring>
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include "connectivity/IConnectivity.h"

/**
 * SecureBleConnectivity — Abstract base class for encrypted BLE transport.
 *
 * Owns all shared FreeRTOS plumbing:
 *   - A raw-message queue (_rawQueue) fed by the inner transport's BLE callback
 *   - A dedicated processing task (_taskHandle) that drains the queue and calls
 *     the protected hook onRawFrameReceived() for each frame
 *
 * Concrete subclasses need only implement two protected virtual hooks:
 *
 *   onRawFrameReceived(data, len)
 *     Called by the processing task for every raw frame arriving from the wire.
 *     The subclass decrypts/authenticates it and, on success, calls
 *     deliverPlaintext() to forward the result to the registered user callback.
 *
 *   sendSecured(plain, len)
 *     Called by sendResponse(). The subclass encrypts/frames the plaintext and
 *     calls sendRaw() to push the result to _inner->sendResponse().
 *
 * Architecture:
 *   IConnectivity
 *   ├── BleConnectivity                    (plain transport)
 *   └── SecureBleConnectivity              (this file — abstract base)
 *       ├── SecurePskBleConnectivity       (AES-256-GCM, static PSK)
 *       └── SecureHandshakeBleConnectivity (PSK handshake — HKDF session key)
 *
 * Internal data flow:
 *   BleConnectivity::onWrite()
 *       └─► _rawQueue          (lightweight ISR-safe enqueue)
 *               └─► [_secureTask]
 *                       └─► onRawFrameReceived()   ← subclass decrypts
 *                               └─► deliverPlaintext() → _userCallback
 *                                                           └─► [Elm327Task]
 */

static constexpr size_t      kSecureMaxMsgLen    = 512;
static constexpr size_t      kSecureQueueDepth   = 8;
static constexpr uint32_t    kSecureTaskStack    = 4096;
static constexpr UBaseType_t kSecureTaskPriority = 5;

class SecureBleConnectivity : public IConnectivity {
public:
    explicit SecureBleConnectivity(IConnectivity* inner);
    ~SecureBleConnectivity() override;

    // ── IConnectivity interface ──────────────────────────────────────────────
    bool begin() override;
    void setOnCommandReceivedCallback(CommandCallback cb) override;

    /**
     * Encrypts/signs `data` by delegating to sendSecured(), which the subclass
     * implements. The result is pushed to _inner via sendRaw().
     */
    void sendResponse(const uint8_t* data, size_t len) override;

    void* getSessionContext() override;

protected:
    // ── Hooks for subclasses ─────────────────────────────────────────────────

    /**
     * Called by the processing task for each raw frame dequeued from _rawQueue.
     *
     * The subclass MUST implement this to authenticate/decrypt the frame.
     * On success, call deliverPlaintext(plain, plainLen).
     * On failure (auth error, malformed frame), silently drop the frame.
     */
    virtual void onRawFrameReceived(const uint8_t* data, size_t len) = 0;

    /**
     * Called by sendResponse() with the caller's plaintext.
     *
     * The subclass MUST implement this to encrypt/frame the data, then call
     * sendRaw(encData, encLen) to push it to the transport layer.
     */
    virtual void sendSecured(const uint8_t* plain, size_t len) = 0;

    // ── Helpers available to subclasses ──────────────────────────────────────

    /** Forwards plaintext to the user callback (e.g. Elm327Task). */
    void deliverPlaintext(const uint8_t* plain, size_t len);

    /** Pushes raw (already encrypted) bytes to the inner transport. */
    void sendRaw(const uint8_t* data, size_t len);

    IConnectivity* _inner;

private:
    CommandCallback _userCallback;
    QueueHandle_t   _rawQueue   = nullptr;
    TaskHandle_t    _taskHandle = nullptr;

    struct RawMessage {
        uint8_t buf[kSecureMaxMsgLen];
        size_t  len;
    };

    static void secureTaskEntry(void* arg);
    void        secureTask();
};