#pragma once
#include <cstdint>
#include "collector/BlockFormat.h"
#include "collector/CollectorConfig.h"
#include "collector/CurrentValues.h"
#include "collector/TmaxMeter.h"

namespace collector {

/** Contadores expostos no AT#LIST. */
struct CollectorStats {
    uint32_t droppedRecords;  // sem buffer livre
    uint32_t droppedGps;      // fila da Aquisição cheia
    uint32_t failedBlocks;    // append falhou duas vezes
    uint32_t lostBlocks;      // descartados por falta de espaço
};

/**
 * O que o ELM327 precisa da coleta: modo, configuração (AT#/AT ST/AT SP),
 * valores correntes, GPS, estado da Internet e medição de Tmax. Cada método
 * só retorna depois que a mudança foi aplicada.
 */
class ICollectorControl {
public:
    virtual ~ICollectorControl() = default;

    virtual bool diagMode() const = 0;
    virtual bool setDiagMode(bool on) = 0;

    virtual CollectorConfig config() const = 0;
    virtual ConfigError setPid(uint8_t pid, uint32_t periodMs) = 0;
    virtual bool deletePid(uint8_t pid) = 0;
    virtual ConfigError setTmax(uint32_t tmaxMs) = 0;
    virtual ConfigError setFlushInterval(uint32_t intervalMs) = 0;
    /** Grava o protocolo, espera os blocos irem para a flash e reinicia. */
    virtual bool setProtocol(uint8_t number) = 0;

    virtual bool currentValue(uint8_t pid, CurrentValues::Value& out) const = 0;

    virtual void setInternet(bool available) = 0;
    virtual void reportGps(const GpsFix& fix) = 0;

    /** Dispara a medição; false se já há uma em andamento ou está em diagnóstico. */
    virtual bool startTmaxMeasurement() = 0;
    virtual TmaxReport tmaxReport() const = 0;

    virtual CollectorStats stats() const = 0;

    /**
     * Bancada (só builds com COLLECTOR_EVENTS): entrega à SyncTask um ACK(seq)
     * com tag válida, como se viesse do app (spec fase-1/05 §12).
     */
    virtual bool injectAck(uint32_t seq) = 0;

    /** Bancada: escreve a linha do `AT#SYNC?` (estado do SyncEngine). */
    virtual void syncStatusLine(char* out, size_t capacity) const = 0;

    /** Bancada: corrompe o bloco `seq` na flash (`AT#CORRUPT`). */
    virtual bool corruptBlock(uint32_t seq) = 0;

    /** Bancada: pede o wipe no próximo boot e reinicia (`AT#WIPE`). */
    virtual void requestWipe() = 0;
};

}  // namespace collector
