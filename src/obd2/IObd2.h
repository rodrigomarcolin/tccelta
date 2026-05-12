#pragma once
#include <cstdint>

class IObd2 {
public:
    virtual ~IObd2() = default;

    virtual bool begin() = 0;

    /**
     * Read a PID value.
     * @param service  OBD-II service byte (e.g. 0x01 for current data)
     * @param pid      PID byte
     * @param buf      output buffer for data bytes (A, B, C, D …)
     * @param maxLen   capacity of buf
     * @return number of data bytes written, or -1 on error/timeout
     */
    virtual int readPid(uint8_t service, uint8_t pid,
                        uint8_t* buf, size_t maxLen) = 0;
};
