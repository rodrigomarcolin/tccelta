#include "collector/GpsGate.h"

namespace collector {

GpsGate::Verdict GpsGate::check(bool diagMode, uint32_t tRxMs) {
    if (diagMode) return Verdict::Diag;
    if (_hasLast && tRxMs - _lastMs < kGpsMinIntervalMs - kGpsJitterMs) return Verdict::Rate;
    _hasLast = true;
    _lastMs  = tRxMs;
    return Verdict::Accept;
}

const char* toString(GpsGate::Verdict verdict) {
    switch (verdict) {
        case GpsGate::Verdict::Accept: return "accept";
        case GpsGate::Verdict::Diag:   return "diag";
        case GpsGate::Verdict::Rate:   return "rate";
    }
    return "-";
}

}  // namespace collector
