#pragma once
#include "collector/CollectorConfig.h"

namespace collector {

/** Fração do tempo em que o barramento fica ocupado: U = Σ Tmax / Pᵢ (Eq. 2). */
double utilization(const CollectorConfig& config);

/** U ≤ Umax. */
bool isFeasible(const CollectorConfig& config);

}  // namespace collector
