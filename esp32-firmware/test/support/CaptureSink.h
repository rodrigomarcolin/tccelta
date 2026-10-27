#pragma once
#include <cstdint>
#include <string>
#include <vector>
#include "collector/Events.h"

/** Guarda as linhas como o ESP32 as imprimiria, sem o prefixo "EV n t". */
class CaptureSink : public collector::IEventSink {
public:
    void emit(const char* body, size_t len, const uint32_t* tMs) override {
        lines.emplace_back(body, len);
        times.push_back(tMs ? static_cast<int64_t>(*tMs) : -1);
    }

    /** Linhas que começam com `event`. */
    std::vector<std::string> of(const std::string& event) const {
        std::vector<std::string> out;
        for (const auto& line : lines) {
            if (line.compare(0, event.size() + 1, event + " ") == 0) out.push_back(line);
        }
        return out;
    }

    std::vector<std::string> lines;
    std::vector<int64_t> times;
};
