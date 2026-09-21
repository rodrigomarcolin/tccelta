#pragma once
#include <cstdint>
#include <cstddef>

static constexpr size_t OBD2_MAX_ECU_RESPONSES = 8;
static constexpr size_t OBD2_MAX_RESPONSE_BYTES = 256;

struct Obd2Response {
    uint32_t ecuId = 0;
    uint8_t data[OBD2_MAX_RESPONSE_BYTES] = {};
    size_t len = 0;
};

struct Obd2ResponseSet {
    Obd2Response items[OBD2_MAX_ECU_RESPONSES];
    size_t count = 0;
};

class IObd2 {
public:
    virtual ~IObd2() = default;

    virtual bool begin() = 0;
    virtual void setResponseTimeoutMs(uint32_t timeoutMs) { (void)timeoutMs; }

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

    virtual int readPidAll(uint8_t service, uint8_t pid,
                           Obd2ResponseSet& responses,
                           uint32_t timeoutMs = 200,
                           size_t expectedResponses = 0) {
        responses.count = 0;
        if (responses.count >= OBD2_MAX_ECU_RESPONSES) return -1;
        Obd2Response& response = responses.items[responses.count];
        int len = readPid(service, pid, response.data,
                          sizeof(response.data));
        if (len < 0) return -1;
        response.ecuId = 0x7E8;
        response.len = static_cast<size_t>(len);
        responses.count = 1;
        (void)timeoutMs;
        (void)expectedResponses;
        return 1;
    }

    /**
     * Read the DTC list for a service that carries no PID.
     * @param service    OBD-II service byte: 0x03 (confirmed), 0x07 (pending)
     *                   or 0x0A (permanent)
     * @param dtcCodes   output buffer of 16-bit DTC codes (e.g. 0x0301 = P0301)
     * @param maxCount   capacity of dtcCodes, in codes (not bytes)
     * @return number of DTC codes written, or -1 on error/timeout
     */
    virtual int readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) = 0;

    virtual int readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                           uint32_t timeoutMs = 200,
                           size_t expectedResponses = 0) {
        responses.count = 0;
        Obd2Response& response = responses.items[responses.count];
        uint16_t codes[OBD2_MAX_RESPONSE_BYTES / 2];
        int count = readDtc(service, codes, sizeof(codes) / sizeof(codes[0]));
        if (count < 0) return -1;
        response.ecuId = 0x7E8;
        response.data[0] = static_cast<uint8_t>(service | 0x40u);
        response.data[1] = static_cast<uint8_t>(count);
        response.len = 2;
        for (int i = 0; i < count && response.len + 1 < sizeof(response.data); ++i) {
            response.data[response.len++] = static_cast<uint8_t>(codes[i] >> 8);
            response.data[response.len++] = static_cast<uint8_t>(codes[i]);
        }
        responses.count = 1;
        (void)timeoutMs;
        (void)expectedResponses;
        return 1;
    }

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

    virtual int readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                                      uint32_t timeoutMs = 200,
                                      size_t expectedResponses = 0) {
        responses.count = 0;
        Obd2Response& response = responses.items[responses.count];
        int len = readFreezeFramePid(pid, response.data,
                                     sizeof(response.data));
        if (len < 0) return -1;
        response.ecuId = 0x7E8;
        response.len = static_cast<size_t>(len);
        responses.count = 1;
        (void)timeoutMs;
        (void)expectedResponses;
        return 1;
    }
};
