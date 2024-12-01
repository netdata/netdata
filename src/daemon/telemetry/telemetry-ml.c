// SPDX-License-Identifier: GPL-3.0-or-later

#define TELEMETRY_INTERNALS 1
#include "telemetry-ml.h"

static struct ml_statistics {
    bool enabled;

    alignas(64) uint64_t ml_models_consulted;
    alignas(64) uint64_t ml_models_received;
    alignas(64) uint64_t ml_models_ignored;
    alignas(64) uint64_t ml_models_sent;
    alignas(64) uint64_t ml_models_deserialization_failures;
} ml_statistics = { 0 };

void telemetry_ml_models_received() {
    if(!ml_statistics.enabled) return;
    __atomic_fetch_add(&ml_statistics.ml_models_received, 1, __ATOMIC_RELAXED);
}

void telemetry_ml_models_ignored() {
    if(!ml_statistics.enabled) return;
    __atomic_fetch_add(&ml_statistics.ml_models_ignored, 1, __ATOMIC_RELAXED);
}

void telemetry_ml_models_sent() {
    if(!ml_statistics.enabled) return;
    __atomic_fetch_add(&ml_statistics.ml_models_sent, 1, __ATOMIC_RELAXED);
}

void global_statistics_ml_models_deserialization_failures() {
    if(!ml_statistics.enabled) return;
    __atomic_fetch_add(&ml_statistics.ml_models_deserialization_failures, 1, __ATOMIC_RELAXED);
}

void telemetry_ml_models_consulted(size_t models_consulted) {
    if(!ml_statistics.enabled) return;
    __atomic_fetch_add(&ml_statistics.ml_models_consulted, models_consulted, __ATOMIC_RELAXED);
}

static inline void ml_statistics_copy(struct ml_statistics *gs) {
    gs->ml_models_consulted          = __atomic_load_n(&ml_statistics.ml_models_consulted, __ATOMIC_RELAXED);
    gs->ml_models_received           = __atomic_load_n(&ml_statistics.ml_models_received, __ATOMIC_RELAXED);
    gs->ml_models_sent               = __atomic_load_n(&ml_statistics.ml_models_sent, __ATOMIC_RELAXED);
    gs->ml_models_ignored            = __atomic_load_n(&ml_statistics.ml_models_ignored, __ATOMIC_RELAXED);
    gs->ml_models_deserialization_failures = __atomic_load_n(&ml_statistics.ml_models_deserialization_failures, __ATOMIC_RELAXED);
}

void telemetry_ml_do(bool extended) {
    if(!extended) return;

    struct ml_statistics gs;
    ml_statistics_copy(&gs);

    ml_update_global_statistics_charts(
        gs.ml_models_consulted,
        gs.ml_models_received,
        gs.ml_models_sent,
        gs.ml_models_ignored,
        gs.ml_models_deserialization_failures);
}
