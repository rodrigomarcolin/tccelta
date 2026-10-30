#pragma once
#include "collector/IBlockStore.h"
#include "collector/ICollectorControl.h"
#include "rtos/Adapters.h"
#include "rtos/CollectorContext.h"
#include "rtos/SharedSyncStatus.h"
#include "storage/NvsStore.h"

namespace rtos {

/**
 * `ICollectorControl` sobre a fila da Aquisição: valida e grava a config na
 * NVS, depois espera a Aquisição aplicar. Usado só pelo `Elm327Task`.
 */
class CollectorControl : public collector::ICollectorControl {
public:
    CollectorControl(CollectorContext& ctx, NvsStore& nvs, const SharedValues& values,
                     const SharedTmaxReport& tmax, const QueueBlockPool& pool,
                     collector::IBlockStore& store, const SharedSyncStatus& syncStatus,
                     const collector::CollectorConfig& config)
        : _ctx(ctx), _nvs(nvs), _values(values), _tmax(tmax), _pool(pool), _store(store),
          _syncStatus(syncStatus), _config(config) {}

    bool diagMode() const override { return _ctx.hasBits(DIAG_MODE); }
    bool setDiagMode(bool on) override;

    collector::CollectorConfig config() const override { return _config; }
    collector::ConfigError setPid(uint8_t pid, uint32_t periodMs) override;
    bool deletePid(uint8_t pid) override;
    collector::ConfigError setTmax(uint32_t tmaxMs) override;
    collector::ConfigError setFlushInterval(uint32_t intervalMs) override;
    bool setProtocol(uint8_t number) override;

    bool currentValue(uint8_t pid, collector::CurrentValues::Value& out) const override;

    void setInternet(bool available) override;
    void reportGps(const collector::GpsFix& fix) override;

    bool startTmaxMeasurement() override;
    collector::TmaxReport tmaxReport() const override { return _tmax.get(); }

    collector::CollectorStats stats() const override;
    bool injectAck(uint32_t seq) override;
    void syncStatusLine(char* out, size_t capacity) const override;
    bool corruptBlock(uint32_t seq) override;
    void requestWipe() override;

private:
    CollectorContext&          _ctx;
    NvsStore&                  _nvs;
    const SharedValues&        _values;
    const SharedTmaxReport&    _tmax;
    const QueueBlockPool&      _pool;
    collector::IBlockStore&    _store;
    const SharedSyncStatus&    _syncStatus;
    collector::CollectorConfig _config;

    /** Valida, grava e aplica; só troca `_config` se tudo der certo. */
    collector::ConfigError commit(const collector::CollectorConfig& candidate);
};

}  // namespace rtos
