#pragma once
#include <cstdint>
#include "collector/BlockBuilder.h"
#include "collector/Clock.h"
#include "collector/CollectorConfig.h"
#include "collector/CurrentValues.h"
#include "collector/PidReader.h"
#include "collector/Scheduler.h"

namespace collector {

/** Pool de buffers de bloco (buffer duplo). Implementado sobre filas no ESP32. */
class IBlockPool {
public:
    virtual ~IBlockPool() = default;
    /** Pega um buffer livre sem bloquear; nullptr se não houver. */
    virtual Block* acquire() = 0;
    /** Entrega um bloco fechado para ser gravado. */
    virtual void submit(Block* block) = 0;
    /** Devolve um buffer sem gravar (bloco vazio). */
    virtual void release(Block* block) = 0;
    /** Índice do buffer (0, 1, ...), para os eventos de depuração. */
    virtual int indexOf(const Block* block) const = 0;
};

/** Destino dos valores lidos (a parte compartilhada da CVT). */
class IValueSink {
public:
    virtual ~IValueSink() = default;
    virtual void update(const ReadResult& result) = 0;
    virtual bool get(uint8_t pid, CurrentValues::Value& out) const = 0;
};

/** Por que um bloco foi fechado (evento BLK_CLOSE). */
enum class CloseReason : uint8_t { Full, Deadline, Config, Mode, Stop };

/**
 * Lógica da tarefa de Aquisição, sem RTOS: decide e faz as leituras, publica
 * os valores, monta os blocos e os fecha por tamanho ou por prazo.
 */
class Collector {
public:
    Collector(const IClock& clock, IPidReader& reader, IBlockPool& pool, IValueSink& values)
        : _clock(clock), _reader(reader), _pool(pool), _values(values) {}

    enum class ConfigReason : uint8_t { Boot, Apply };

    /** Fecha o bloco aberto e recarrega o escalonador. */
    void applyConfig(const CollectorConfig& config, ConfigReason reason = ConfigReason::Apply);

    /**
     * Liga/desliga a coleta (desligada no diagnóstico, na medição de Tmax e
     * após Stop). Ao desligar, fecha o bloco aberto com `why`.
     */
    void setRunning(bool running, CloseReason why = CloseReason::Mode);
    bool running() const { return _running; }

    /**
     * Executa no máximo uma leitura. Devolve quanto a tarefa pode esperar
     * (atendendo pedidos) antes de chamar de novo: 0 = chamar já.
     */
    uint32_t poll();

    void addGps(const GpsFix& fix, uint32_t tMs);

    /** Fecha e entrega o bloco aberto, se houver registros. */
    void closeBlock(CloseReason why);

    uint32_t tmaxMs() const { return _tmaxMs; }
    uint32_t droppedRecords() const { return _droppedRecords; }

private:
    const IClock& _clock;
    IPidReader&   _reader;
    IBlockPool&   _pool;
    IValueSink&   _values;
    Scheduler     _scheduler;
    BlockBuilder  _builder;
    Block*        _openBlock = nullptr;
    uint32_t      _tmaxMs  = kDefaultTmaxMs;
    bool          _running = false;
    uint32_t      _droppedRecords = 0;
    uint16_t      _openRecords = 0;

    void closeIfDue(uint32_t nowMs);

    void emitConfig(const CollectorConfig& config, ConfigReason reason) const;
    void emitDecide(const Scheduler::Decision& decision) const;
    void emitRead(const ReadResult& result, bool early, uint32_t endMs) const;
    void emitPidRecord(const ReadResult& result, bool stored) const;
    void emitGpsRecord(const GpsFix& fix, uint32_t tMs, bool stored) const;
    void emitBlockOpen(const Block& block, uint32_t t0) const;
    void emitBlockClose(const Block& block, CloseReason why, bool empty) const;

    /** Garante um bloco aberto e tenta `add`; se não couber, troca de bloco. */
    template <typename AddFn>
    bool append(uint32_t tMs, AddFn add);
};

}  // namespace collector
