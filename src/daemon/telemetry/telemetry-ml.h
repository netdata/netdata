// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_ML_H
#define NETDATA_TELEMETRY_ML_H

#include "daemon/common.h"

#ifdef __cplusplus
extern "C" {
#endif

void telemetry_ml_models_consulted(size_t models_consulted);
void telemetry_ml_models_received();
void telemetry_ml_models_ignored();
void telemetry_ml_models_sent();

void telemetry_ml_memory_allocated(size_t n);
void telemetry_ml_memory_freed(size_t n);

void global_statistics_ml_models_deserialization_failures();

#if defined(TELEMETRY_INTERNALS)
void telemetry_ml_do(bool extended);
#endif

#ifdef __cplusplus
}
#endif


#endif //NETDATA_TELEMETRY_ML_H
