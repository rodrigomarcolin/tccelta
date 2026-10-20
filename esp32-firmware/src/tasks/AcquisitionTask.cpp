#include "tasks/AcquisitionTask.h"

using rtos::AcqOp;
using rtos::AcqRequest;

namespace {

constexpr uint32_t   kStackBytes = 8192;
constexpr UBaseType_t kPriority  = 6;
constexpr BaseType_t kCore       = 1;

// Na medição, espera bem mais que o Tmax atual: é justamente ele que se mede.
constexpr uint32_t kMeasureTimeoutMs = 1000;

}  // namespace

void AcquisitionTask::start(const collector::CollectorConfig& config) {
    _config = config;
    _collector.applyConfig(config);
    _collector.setRunning(true);
    rtos::startTask(this, "Acquisition", kStackBytes, kPriority, kCore);
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
        case AcqOp::RunOnBus:
            request.job(_bus, request.jobContext);
            return true;

        case AcqOp::ApplyConfig:
            _config = *request.config;
            _collector.applyConfig(_config);
            return true;

        case AcqOp::SetMode:
            _diag = request.diag;
            resumeCollection();
            // O bit muda antes de notificar: o próximo comando já vê o modo novo.
            if (_diag) xEventGroupSetBits(_ctx.events, rtos::DIAG_MODE);
            else       xEventGroupClearBits(_ctx.events, rtos::DIAG_MODE);
            return true;

        case AcqOp::Stop:
            _stopped = true;
            resumeCollection();
            return true;

        case AcqOp::MeasureTmax:
            if (_diag || _stopped) return false;
            startMeasurement();
            return true;

        case AcqOp::Gps:
            if (_stopped) return false;
            _collector.addGps(request.gps, request.gpsTimeMs);
            return true;
    }
    return false;
}

void AcquisitionTask::startMeasurement() {
    _measuring = true;
    resumeCollection();
    _meter.begin(_config);
    _tmaxReport.set(_meter.report());
}

void AcquisitionTask::measureOnce() {
    if (_meter.total() > 0) {
        uint8_t buf[8];
        const uint32_t startMs = _clock.nowMs();
        const int len = _bus.readEnginePid(_meter.nextPid(), buf, sizeof(buf), kMeasureTimeoutMs);
        _meter.record(len > 0, _clock.nowMs() - startMs);
    }
    _tmaxReport.set(_meter.report());

    if (_meter.finished()) {
        _measuring = false;
        resumeCollection();
    }
}
