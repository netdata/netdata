// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JITTER_H
#define NETDATA_JITTER_H

#include "libnetdata/common.h"

typedef struct {
    struct random_data rand_state;       // Embedding random_data directly
    char rand_state_buf[64];             // Buffer for random state
} OS_JITTER;

void os_jitter_init(OS_JITTER *jt, unsigned int seed);
uint32_t os_jitter_ut(OS_JITTER *jt, uint32_t max);
void os_jitter_wait(OS_JITTER *jt, uint32_t max);

#endif //NETDATA_JITTER_H
