// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

void os_random_init(OS_RANDOM *jt, unsigned int seed) {
    if (!seed)
        seed = (unsigned int)(os_gettid() + now_monotonic_usec());

    jt->seed = seed;

#ifdef RANDOM_USE_RANDOM_R
    memset(&jt->rand_state, 0, sizeof(jt->rand_state));
    initstate_r(seed, jt->rand_state_buf, sizeof(jt->rand_state_buf), &jt->rand_state);
#endif
}

uint64_t os_random(OS_RANDOM *jt, uint64_t max) {
    if (max == 0) return 0;

    uint64_t rand_val = 0;

#if defined(RANDOM_USE_RANDOM_R)
    if (max <= (1ULL << 31)) {
        int32_t temp;
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
    } else if (max <= (1ULL << 62)) {
        int32_t temp;
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x7FFFFFFF) << 31;
    } else {
        int32_t temp;
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x7FFFFFFF) << 31;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x3) << 62;  // Only take 2 bits from the third call
    }

#elif defined(RANDOM_USE_ARC4RANDOM)
    if (max <= UINT32_MAX)
        rand_val = arc4random_uniform((uint32_t)max);
    else {
        rand_val  = (uint64_t)arc4random_uniform(UINT32_MAX);
        rand_val |= ((uint64_t)arc4random_uniform(UINT32_MAX) << 32);
    }

#elif defined(RANDOM_USE_RAND_S)
    if (max <= UINT32_MAX) {
        unsigned int rand_num;
        rand_s(&rand_num);
        rand_val = (uint64_t)rand_num;
    } else {
        unsigned int rand_num;
        rand_s(&rand_num);
        rand_val = (uint64_t)rand_num;
        rand_s(&rand_num);
        rand_val |= ((uint64_t)rand_num << 32);
    }
#endif

    return rand_val % max;
}
