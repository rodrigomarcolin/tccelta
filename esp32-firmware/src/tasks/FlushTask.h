#pragma once
#include "collector/BlockWriter.h"
#include "collector/IBlockStore.h"
#include "rtos/CollectorContext.h"

/**
 * Grava na flash os blocos fechados pela Aquisição (núcleo 1, prioridade
 * média) e devolve os buffers ao pool. Depois de cada bloco gravado posta
 * `SyncInput{Flushed, seq}` para a SyncTask avisar o app (spec fase-1/04 §6).
 */
class FlushTask {
public:
    FlushTask(rtos::CollectorContext& ctx, collector::BlockWriter& writer, const collector::IBlockStore& store)
        : _ctx(ctx), _writer(writer), _store(store) {}

    TaskHandle_t start();
    void run();

    /** Evento FLUSH + MARKER do BOOT, emitidos também pelo boot (CollectorApp). */
    static void emitFlush(int buf, const collector::Block& block, const collector::WriteReport& report,
                          uint32_t startMs);

private:
    rtos::CollectorContext&       _ctx;
    collector::BlockWriter&       _writer;
    const collector::IBlockStore& _store;

    void postFlushed(uint32_t seq);
};
