#pragma once
#include "collector/Clock.h"
#include "collector/PidReader.h"
#include "obd2/IObd2.h"

namespace collector {

/** Um PID por requisição, com requisição física ao motor. */
class SequentialPidReader : public IPidReader {
public:
    SequentialPidReader(IObd2& obd, const IClock& clock) : _obd(obd), _clock(clock) {}

    size_t maxPidsPerRequest() const override { return 1; }
    void read(const uint8_t* pids, size_t n, ReadResult* out, uint32_t timeoutMs) override;

private:
    IObd2&        _obd;
    const IClock& _clock;
};

}  // namespace collector
