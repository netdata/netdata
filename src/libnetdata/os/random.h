// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RANDOM_H
#define NETDATA_RANDOM_H

#include "libnetdata/common.h"

typedef struct {
    struct random_data rand_state;       // Embedding random_data directly
    char rand_state_buf[64];             // Buffer for random state
} OS_RANDOM;

void os_random_init(OS_RANDOM *jt, unsigned int seed);
uint64_t os_random(OS_RANDOM *jt, uint64_t max);

#endif //NETDATA_RANDOM_H
