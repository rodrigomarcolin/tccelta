#include "collector/Utilization.h"

namespace collector {

double utilization(const CollectorConfig& config) {
    double total = 0.0;
    for (size_t i = 0; i < config.pidCount; ++i) {
        total += static_cast<double>(config.tmaxMs) / static_cast<double>(config.pids[i].periodMs);
    }
    return total;
}

bool isFeasible(const CollectorConfig& config) {
    return utilization(config) <= kMaxUtilization;
}

}  // namespace collector
