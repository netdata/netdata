// SPDX-License-Identifier: GPL-3.0-or-later

#include "pulse-db-rrd.h"

int64_t pulse_rrd_memory_size = 0;

void pulse_db_rrd_memory_change(int64_t value) {
    __atomic_add_fetch(&pulse_rrd_memory_size, value, __ATOMIC_RELAXED);
}

void pulse_db_rrd_memory_add(uint64_t value) {
    __atomic_add_fetch(&pulse_rrd_memory_size, value, __ATOMIC_RELAXED);
}

void pulse_db_rrd_memory_sub(uint64_t value) {
    __atomic_sub_fetch(&pulse_rrd_memory_size, value, __ATOMIC_RELAXED);
}

