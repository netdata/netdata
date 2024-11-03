// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(HAVE_GETRANDOM)
#include <sys/random.h>
#endif

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max) {
    if (max <= 1) return 0;

    union {
        uint64_t u64;
        struct {
            uint32_t hi;
            uint32_t lo;
        } u32;
    } value;

#if defined(HAVE_ARC4RANDOM_UNIFORM)
    if (max <= UINT32_MAX) {
        value.u64 = arc4random_uniform((uint32_t)max);
    } else {
        value.u32.lo = arc4random_uniform(UINT32_MAX);
        value.u32.hi = arc4random_uniform(UINT32_MAX);
    }

#elif defined(HAVE_RAND_S)
    if (max <= UINT32_MAX) {
        unsigned int temp;
        rand_s(&temp);
        value.u64 = temp;
    } else {
        unsigned int temp_lo, temp_hi;
        rand_s(&temp_lo);
        rand_s(&temp_hi);
        value.u32.lo = temp_lo;
        value.u32.hi = temp_hi;
    }

#elif defined(HAVE_GETRANDOM)
    if (max <= UINT8_MAX) {
        uint8_t v;
        getrandom(&v, sizeof(v), 0);
        value.u64 = v;
    } else if(max <= UINT16_MAX) {
        uint16_t v;
        getrandom(&v, sizeof(v), 0);
        value.u64 = v;
    } else if (max <= UINT32_MAX) {
        uint32_t v;
        getrandom(&v, sizeof(v), 0);
        value.u64 = v;
    } else {
        uint64_t v;
        getrandom(&v, sizeof(v), 0);
        value.u64 = v;
    }

#else
#error "Can't use any random number generator"
#endif

    return value.u64 % max;
}
