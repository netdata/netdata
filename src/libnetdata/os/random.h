// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RANDOM_H
#define NETDATA_RANDOM_H

#include "libnetdata/common.h"

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#define RANDOM_USE_ARC4RANDOM
#elif defined(OS_WINDOWS)
#define RANDOM_USE_RAND_S
#else
#error "Can't use any random number generator"
#endif

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max);

#endif //NETDATA_RANDOM_H
