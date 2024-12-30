// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-ml.h"

static struct ml_statistics {
    PAD64(uint64_t) ml_models_consulted;
    PAD64(uint64_t) ml_models_received;
    PAD64(uint64_t) ml_models_ignored;
    PAD64(uint64_t) ml_models_sent;
    PAD64(uint64_t) ml_models_deserialization_failures;
    PAD64(uint64_t) ml_memory_consumption;
    PAD64(uint64_t) ml_memory_new;
    PAD64(uint64_t) ml_memory_delete;
} ml_statistics = { 0 };

void pulse_ml_models_received()
{
    __atomic_fetch_add(&ml_statistics.ml_models_received, 1, __ATOMIC_RELAXED);
}

void pulse_ml_models_ignored()
{
    __atomic_fetch_add(&ml_statistics.ml_models_ignored, 1, __ATOMIC_RELAXED);
}

void pulse_ml_models_sent()
{
    __atomic_fetch_add(&ml_statistics.ml_models_sent, 1, __ATOMIC_RELAXED);
}

void global_statistics_ml_models_deserialization_failures()
{
    __atomic_fetch_add(&ml_statistics.ml_models_deserialization_failures, 1, __ATOMIC_RELAXED);
}

void pulse_ml_models_consulted(size_t models_consulted)
{
    __atomic_fetch_add(&ml_statistics.ml_models_consulted, models_consulted, __ATOMIC_RELAXED);
}

void pulse_ml_memory_allocated(size_t n)
{
    __atomic_fetch_add(&ml_statistics.ml_memory_consumption, n, __ATOMIC_RELAXED);
    __atomic_fetch_add(&ml_statistics.ml_memory_new, 1, __ATOMIC_RELAXED);
}

void pulse_ml_memory_freed(size_t n)
{
    __atomic_fetch_sub(&ml_statistics.ml_memory_consumption, n, __ATOMIC_RELAXED);
    __atomic_fetch_add(&ml_statistics.ml_memory_delete, 1, __ATOMIC_RELAXED);
}

uint64_t pulse_ml_get_current_memory_usage(void) {
    return __atomic_load_n(&ml_statistics.ml_memory_consumption, __ATOMIC_RELAXED);
}

static inline void ml_statistics_copy(struct ml_statistics *gs)
{
    gs->ml_models_consulted = __atomic_load_n(&ml_statistics.ml_models_consulted, __ATOMIC_RELAXED);
    gs->ml_models_received = __atomic_load_n(&ml_statistics.ml_models_received, __ATOMIC_RELAXED);
    gs->ml_models_sent = __atomic_load_n(&ml_statistics.ml_models_sent, __ATOMIC_RELAXED);
    gs->ml_models_ignored = __atomic_load_n(&ml_statistics.ml_models_ignored, __ATOMIC_RELAXED);
    gs->ml_models_deserialization_failures =
        __atomic_load_n(&ml_statistics.ml_models_deserialization_failures, __ATOMIC_RELAXED);

    gs->ml_memory_consumption = __atomic_load_n(&ml_statistics.ml_memory_consumption, __ATOMIC_RELAXED);
    gs->ml_memory_new = __atomic_load_n(&ml_statistics.ml_memory_new, __ATOMIC_RELAXED);
    gs->ml_memory_delete = __atomic_load_n(&ml_statistics.ml_memory_delete, __ATOMIC_RELAXED);
}

void pulse_ml_do(bool extended)
{
    if (!extended)
        return;

    struct ml_statistics gs;
    ml_statistics_copy(&gs);

    ml_update_global_statistics_charts(
        gs.ml_models_consulted,
        gs.ml_models_received,
        gs.ml_models_sent,
        gs.ml_models_ignored,
        gs.ml_models_deserialization_failures,
        gs.ml_memory_consumption,
        gs.ml_memory_new,
        gs.ml_memory_delete);
}
