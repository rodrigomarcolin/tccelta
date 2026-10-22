#include "tasks/AcquisitionTask.h"
#include "collector/Events.h"

using rtos::AcqOp;
using rtos::AcqRequest;

namespace {

constexpr uint32_t   kStackBytes = 8192;
constexpr UBaseType_t kPriority  = 6;
constexpr BaseType_t kCore       = 1;

// Na medição, espera bem mais que o Tmax atual: é justamente ele que se mede.
constexpr uint32_t kMeasureTimeoutMs = 1000;

}  // namespace

TaskHandle_t AcquisitionTask::start(const collector::CollectorConfig& config) {
    _config = config;
    _collector.applyConfig(config, collector::Collector::ConfigReason::Boot);
    _collector.setRunning(true);
    return rtos::startTask(this, "Acquisition", kStackBytes, kPriority, kCore);
}

void AcquisitionTask::run() {
    while (true) {
        uint32_t waitMs = 0;
        if (_measuring) {
            measureOnce();
        } else {
            waitMs = _collector.poll();
            _ctx.counters.droppedRecords = _collector.droppedRecords();
        }

        // Espera pedidos até a próxima ação; depois de uma leitura, só esvazia a fila.
        TickType_t ticks = waitMs == UINT32_MAX ? portMAX_DELAY : pdMS_TO_TICKS(waitMs);
        AcqRequest request;
        while (xQueueReceive(_ctx.acqRequests, &request, ticks) == pdTRUE) {
            const bool accepted = handle(request);
            if (request.requester) {
                if (request.accepted) *request.accepted = accepted;
                xTaskNotifyGive(request.requester);
            }
            ticks = 0;
        }
    }
}

bool AcquisitionTask::handle(const AcqRequest& request) {
    // Durante a medição de Tmax, só o GPS é aceito.
    if (_measuring && request.op != AcqOp::Gps) return false;

    switch (request.op) {
        case AcqOp::RunOnBus: {
            const uint32_t startMs = collector::kEventsEnabled ? _clock.nowMs() : 0;
            request.job(_bus, request.jobContext);
            emitBus(startMs);
            return true;
        }

        case AcqOp::ApplyConfig:
            _config = *request.config;
            _collector.applyConfig(_config);
            return true;

        case AcqOp::SetMode:
            _diag = request.diag;
            resumeCollection();
            emitMode();
            // O bit muda antes de notificar: o próximo comando já vê o modo novo.
            if (_diag) xEventGroupSetBits(_ctx.events, rtos::DIAG_MODE);
            else       xEventGroupClearBits(_ctx.events, rtos::DIAG_MODE);
            return true;

        case AcqOp::Stop:
            _stopped = true;
            _collector.setRunning(false, collector::CloseReason::Stop);
            emitMode();
            return true;

        case AcqOp::MeasureTmax:
            if (_diag || _stopped) return false;
            startMeasurement();
            return true;

        case AcqOp::Gps: {
            if (_stopped) return false;  // reiniciando
            const collector::GpsGate::Verdict verdict = _gpsGate.check(_diag, request.gpsTimeMs);
            if (verdict != collector::GpsGate::Verdict::Accept) {
                ++_ctx.counters.droppedGps;
                collector::Event("GPS_DROP").token("reason", collector::toString(verdict))
                    .num("t_rx", request.gpsTimeMs);
                return false;
            }
            collector::Event("GPS").num("lat", request.gps.latE7).num("lon", request.gps.lonE7)
                .num("alt", request.gps.altMm).num("t_rx", request.gpsTimeMs);
            _collector.addGps(request.gps, request.gpsTimeMs);
            return true;
        }
    }
    return false;
}

void AcquisitionTask::startMeasurement() {
    _measuring = true;
    resumeCollection();
    emitMode();
    _meter.begin(_config);
    _tmaxReport.set(_meter.report());

    if (collector::kEventsEnabled) {
        uint8_t pids[collector::kMaxPids];
        for (size_t i = 0; i < _config.pidCount; ++i) pids[i] = _config.pids[i].pid;
        collector::Event("TMAX_START").hexList("pids", pids, _config.pidCount)
            .num("samples", collector::kTmaxSamplesPerPid);
    }
}

void AcquisitionTask::measureOnce() {
    if (_meter.total() > 0) {
        uint8_t buf[8];
        const uint8_t pid = _meter.nextPid();
        const uint32_t startMs = _clock.nowMs();
        const int len = _bus.readEnginePid(pid, buf, sizeof(buf), kMeasureTimeoutMs);
        const uint32_t latencyMs = _clock.nowMs() - startMs;
        _meter.record(len > 0, latencyMs);
        collector::Event("TMAX_SAMPLE").hex8("pid", pid).flag("ok", len > 0).num("lat", latencyMs);
    }
    const collector::TmaxReport report = _meter.report();
    _tmaxReport.set(report);

    if (_meter.finished()) {
        _measuring = false;
        collector::Event("TMAX_DONE").num("tmax", report.suggestedTmaxMs);
        resumeCollection();
        emitMode();
    }
}

void AcquisitionTask::emitBus(uint32_t startMs) const {
    if (!collector::kEventsEnabled) return;
    const uint32_t endMs = _clock.nowMs();
    collector::Event("BUS").at(endMs).token("kind", "diag").num("start", startMs)
        .num("dur", endMs - startMs);
}

void AcquisitionTask::emitMode() const {
    collector::Event("MODE").flag("diag", _diag).flag("running", _collector.running());
}
