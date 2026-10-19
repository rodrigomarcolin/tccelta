#pragma once
#include "collector/BlockWriter.h"
#include "collector/IBlockStore.h"
#include "rtos/CollectorContext.h"

/**
 * Grava na flash os blocos fechados pela Aquisição (núcleo 1, prioridade
 * média) e devolve os buffers ao pool.
 */
class FlushTask {
public:
    FlushTask(rtos::CollectorContext& ctx, collector::BlockWriter& writer,
              const collector::IBlockStore& store)
        : _ctx(ctx), _writer(writer), _store(store) {}

    void start();
    void run();

private:
    rtos::CollectorContext&       _ctx;
    collector::BlockWriter&       _writer;
    const collector::IBlockStore& _store;
};
