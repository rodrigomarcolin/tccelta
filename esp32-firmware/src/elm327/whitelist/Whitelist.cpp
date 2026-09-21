#include "Whitelist.h"
#include "WhitelistConfig.h"

Whitelist::Whitelist() {
    for (const Entry& entry : WhitelistConfig::kAllowedPids) {
        add(entry.service, entry.pid);
    }
    for (uint8_t service : WhitelistConfig::kAllowedServices) {
        addService(service);
    }
}

int Whitelist::indexOf(uint8_t service, uint8_t pid) const {
    for (size_t i = 0; i < _count; ++i) {
        if (_entries[i].service == service && _entries[i].pid == pid) {
            return static_cast<int>(i);
        }
    }
    return -1;
}

bool Whitelist::add(uint8_t service, uint8_t pid) {
    if (indexOf(service, pid) >= 0) return true;
    if (_count >= MAX_ENTRIES) return false;
    _entries[_count++] = {service, pid};
    return true;
}

bool Whitelist::addService(uint8_t service) {
    for (size_t i = 0; i < _serviceCount; ++i) {
        if (_services[i] == service) return true;
    }
    if (_serviceCount >= MAX_SERVICES) return false;
    _services[_serviceCount++] = service;
    return true;
}

void Whitelist::remove(uint8_t service, uint8_t pid) {
    int index = indexOf(service, pid);
    if (index < 0) return;
    for (size_t i = static_cast<size_t>(index) + 1; i < _count; ++i) {
        _entries[i - 1] = _entries[i];
    }
    --_count;
}

bool Whitelist::isAllowed(uint8_t service, uint8_t pid) const {
    return !_enabled || indexOf(service, pid) >= 0;
}

bool Whitelist::isServiceAllowed(uint8_t service) const {
    if (!_enabled) return true;
    for (size_t i = 0; i < _serviceCount; ++i) {
        if (_services[i] == service) return true;
    }
    return false;
}
