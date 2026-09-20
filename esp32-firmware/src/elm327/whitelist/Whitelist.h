#pragma once
#include <cstdint>
#include <cstddef>

// Tracks which (service, pid) pairs the dongle is permitted to forward
// to the vehicle bus. Secure-by-default: an empty, enabled whitelist
// allows nothing.
class Whitelist {
public:
    static constexpr size_t MAX_ENTRIES = 64;
    static constexpr size_t MAX_SERVICES = 8;

    struct Entry {
        uint8_t service;
        uint8_t pid;
    };

    Whitelist();

    // Enable/disable enforcement. Disabled = allow everything
    // (useful for bench testing; should be true in production builds).
    void setEnabled(bool on) { _enabled = on; }
    bool isEnabled() const   { return _enabled; }

    // Returns false if the table is full.
    bool add(uint8_t service, uint8_t pid);

    // Allow a command that has no PID, such as OBD service 03.
    bool addService(uint8_t service);

    // Remove a single entry, if present.
    void remove(uint8_t service, uint8_t pid);

    // Clear all entries.
    void clear() { _count = 0; _serviceCount = 0; }

    // Core check used by callers.
    bool isAllowed(uint8_t service, uint8_t pid) const;
    bool isServiceAllowed(uint8_t service) const;

    size_t size() const { return _count; }

private:
    Entry  _entries[MAX_ENTRIES];
    size_t _count   = 0;
    uint8_t _services[MAX_SERVICES] = {};
    size_t _serviceCount = 0;
    bool   _enabled = true;

    int indexOf(uint8_t service, uint8_t pid) const;
};
