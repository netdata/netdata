// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RANDOM_H
#define NETDATA_RANDOM_H

#include "libnetdata/common.h"

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max);

#endif //NETDATA_RANDOM_H
