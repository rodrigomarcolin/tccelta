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

    /**
     * Read the DTC list for a service that carries no PID.
     * @param service    OBD-II service byte: 0x03 (confirmed), 0x07 (pending)
     *                   or 0x0A (permanent)
     * @param dtcCodes   output buffer of 16-bit DTC codes (e.g. 0x0301 = P0301)
     * @param maxCount   capacity of dtcCodes, in codes (not bytes)
     * @return number of DTC codes written, or -1 on error/timeout
     */
    virtual int readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) = 0;

    /**
     * Read a PID from the stored freeze frame (Mode 02, frame 0 — the only
     * frame supported; there is no history of older frames, so "frame" is
     * never exposed as a parameter here).
     *
     * PID 0x02 is special in the protocol: instead of a regular sensor
     * value, it returns the 2 bytes of the DTC that triggered the freeze
     * frame capture. That semantics belongs to the caller — for this
     * interface it is just another PID like any other.
     *
     * @param pid      PID byte (0x02 for the origin DTC, or any PID in the
     *                 fixed set actually captured by the vehicle)
     * @param buf      output buffer for data bytes
     * @param maxLen   capacity of buf
     * @return number of data bytes written, or -1 if the PID isn't part of
     *         the captured set, there is no freeze frame stored, or on
     *         error/timeout
     */
    virtual int readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) = 0;
};
