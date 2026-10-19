#pragma once
#include "collector/Collector.h"
#include "collector/TmaxMeter.h"
#include "obd2/IObd2.h"
#include "rtos/Adapters.h"
#include "rtos/CollectorContext.h"

/**
 * Única tarefa que usa o barramento CAN (núcleo 1, prioridade alta).
 * Alterna entre o `Collector` e os pedidos de `acqRequests`, usando a espera
 * na fila como temporizador até a próxima leitura ou o prazo do bloco.
 */
class AcquisitionTask {
public:
    AcquisitionTask(rtos::CollectorContext& ctx, collector::Collector& collector, IObd2& bus,
                    const collector::IClock& clock, rtos::SharedTmaxReport& tmaxReport)
        : _ctx(ctx), _collector(collector), _bus(bus), _clock(clock), _tmaxReport(tmaxReport) {}

    void start(const collector::CollectorConfig& config);
    void run();

private:
    rtos::CollectorContext&    _ctx;
    collector::Collector&      _collector;
    IObd2&                     _bus;
    const collector::IClock&   _clock;
    rtos::SharedTmaxReport&    _tmaxReport;
    collector::CollectorConfig _config;
    collector::TmaxMeter       _meter;
    bool _diag      = false;
    bool _stopped   = false;
    bool _measuring = false;

    /** Executa o pedido; devolve false se ele foi recusado. */
    bool handle(const rtos::AcqRequest& request);
    void startMeasurement();
    void measureOnce();
    void resumeCollection() { _collector.setRunning(!_diag && !_stopped && !_measuring); }
};
