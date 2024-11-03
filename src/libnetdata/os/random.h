// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RANDOM_H
#define NETDATA_RANDOM_H

#include "libnetdata/common.h"

#if defined(OS_LINUX) || defined(OS_FREEBSD)
#define RANDOM_USE_RANDOM_R
#elif defined(OS_MACOS)
#define RANDOM_USE_ARC4RANDOM
#elif defined(OS_WINDOWS)
#define RANDOM_USE_RAND_S
#else
#error "Can't use any random number generator"
#endif

typedef struct {
    unsigned int seed;
#ifdef RANDOM_USE_RANDOM_R
    struct random_data rand_state;
    char rand_state_buf[64];
#endif
} OS_RANDOM;

void os_random_init(OS_RANDOM *jt, unsigned int seed);
uint64_t os_random(OS_RANDOM *jt, uint64_t max);

#endif //NETDATA_RANDOM_H
