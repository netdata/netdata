// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

void os_random_init(OS_RANDOM *jt, unsigned int seed) {
    if(!seed)
        seed = (unsigned int)(os_gettid() + now_monotonic_usec());

    memset(&jt->rand_state, 0, sizeof(jt->rand_state));
    initstate_r(seed, jt->rand_state_buf, sizeof(jt->rand_state_buf), &jt->rand_state);
}

uint64_t os_random(OS_RANDOM *jt, uint64_t max) {
    uint64_t rand_val = 0;
    int32_t temp;

    // Determine the number of random_r() calls based on the value of max
    if (max <= (1ULL << 31)) {
        // Case 1: max fits in 31 bits, 1 call
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
    }
    else if (max <= (1ULL << 62)) {
        // Case 2: max fits in 62 bits, 2 calls
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x7FFFFFFF) << 31;
    }
    else {
        // Case 3: max requires up to 64 bits, 3 calls
        random_r(&jt->rand_state, &temp);
        rand_val = (uint64_t)temp & 0x7FFFFFFF;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x7FFFFFFF) << 31;
        random_r(&jt->rand_state, &temp);
        rand_val |= ((uint64_t)temp & 0x3) << 62;  // Only take 2 bits from the third call
    }

    return rand_val % max;
}
