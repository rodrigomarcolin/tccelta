#include "rtos/CollectorControl.h"
#include "collector/Events.h"

namespace rtos {

namespace {

// AT SP: tempo máximo para os dois buffers voltarem (tudo gravado) antes do restart.
constexpr uint32_t kFlushWaitMs = 2000;

}  // namespace

bool CollectorControl::setDiagMode(bool on) {
    AcqRequest request = {};
    request.op   = AcqOp::SetMode;
    request.diag = on;
    return callAcquisition(_ctx, request);
}

collector::ConfigError CollectorControl::commit(const collector::CollectorConfig& candidate) {
    const collector::ConfigError error = collector::validate(candidate);
    if (error != collector::ConfigError::None) return error;

    AcqRequest request = {};
    request.op     = AcqOp::ApplyConfig;
    request.config = &candidate;
    if (!callAcquisition(_ctx, request)) return collector::ConfigError::Busy;
    _nvs.saveConfig(candidate);
    _config = candidate;
    return error;
}

collector::ConfigError CollectorControl::setPid(uint8_t pid, uint32_t periodMs) {
    collector::CollectorConfig candidate = _config;
    if (pid > collector::kPidRecordTypeMax) return collector::ConfigError::InvalidPid;
    if (!candidate.setPid(pid, periodMs)) return collector::ConfigError::TooManyPids;
    return commit(candidate);
}

bool CollectorControl::deletePid(uint8_t pid) {
    collector::CollectorConfig candidate = _config;
    return candidate.removePid(pid) && commit(candidate) == collector::ConfigError::None;
}

collector::ConfigError CollectorControl::setTmax(uint32_t tmaxMs) {
    collector::CollectorConfig candidate = _config;
    candidate.tmaxMs = tmaxMs;
    return commit(candidate);
}

collector::ConfigError CollectorControl::setFlushInterval(uint32_t intervalMs) {
    collector::CollectorConfig candidate = _config;
    candidate.flushIntervalMs = intervalMs;
    return commit(candidate);
}

bool CollectorControl::setProtocol(uint8_t number) {
    if (!collector::isValidProtocol(number)) return false;
    collector::CollectorConfig candidate = _config;
    candidate.protocol = static_cast<collector::CanProtocol>(number);
    _nvs.saveConfig(candidate);

    // Fecha o bloco aberto e espera tudo ir para a flash antes de reiniciar.
    AcqRequest request = {};
    request.op = AcqOp::Stop;
    callAcquisition(_ctx, request);
    const TickType_t deadline = xTaskGetTickCount() + pdMS_TO_TICKS(kFlushWaitMs);
    while (!_pool.allFree() && xTaskGetTickCount() < deadline) vTaskDelay(pdMS_TO_TICKS(20));

    esp_restart();
    return true;
}

bool CollectorControl::currentValue(uint8_t pid, collector::CurrentValues::Value& out) const {
    return _config.find(pid) != nullptr && _values.get(pid, out);
}

void CollectorControl::setInternet(bool available) {
    collector::Event("NET").flag("up", available);
    if (available) xEventGroupSetBits(_ctx.events, INTERNET);
    else           xEventGroupClearBits(_ctx.events, INTERNET);
}

void CollectorControl::reportGps(const collector::GpsFix& fix) {
    AcqRequest request = {};
    request.op        = AcqOp::Gps;
    request.gps       = fix;
    request.gpsTimeMs = millis();  // marcado ao receber, não ao processar
    if (!postAcquisition(_ctx, request)) {
        ++_ctx.counters.droppedGps;
        collector::Event("GPS_DROP").token("reason", "queue").num("t_rx", request.gpsTimeMs);
    }
}

bool CollectorControl::startTmaxMeasurement() {
    if (diagMode() || _tmax.get().state == collector::TmaxReport::State::Running) return false;
    AcqRequest request = {};
    request.op = AcqOp::MeasureTmax;
    return callAcquisition(_ctx, request);
}

bool CollectorControl::injectAck(uint32_t seq) {
    if (!collector::kEventsEnabled) return false;
    DataMessage message = {};
    message.len = _protocol.encodeAck(seq, message.bytes, sizeof(message.bytes));
    return message.len > 0 && xQueueSend(_ctx.uploadRx, &message, 0) == pdTRUE;
}

collector::CollectorStats CollectorControl::stats() const {
    return {_ctx.counters.droppedRecords, _ctx.counters.droppedGps,
            _ctx.counters.failedBlocks, _ctx.counters.lostBlocks};
}

}  // namespace rtos
