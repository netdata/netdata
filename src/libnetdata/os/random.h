// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RANDOM_H
#define NETDATA_RANDOM_H

#include "libnetdata/common.h"

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max);

uint8_t os_random8(void);
uint16_t os_random16(void);
uint32_t os_random32(void);
uint64_t os_random64(void);

#endif //NETDATA_RANDOM_H
