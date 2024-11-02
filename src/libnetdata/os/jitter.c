// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

void os_jitter_init(OS_JITTER *jt, unsigned int seed) {
    memset(&jt->rand_state, 0, sizeof(jt->rand_state));
    initstate_r(seed, jt->rand_state_buf, sizeof(jt->rand_state_buf), &jt->rand_state);
}

uint32_t os_jitter_ut(OS_JITTER *jt, uint32_t max) {
    int32_t rand_val;
    random_r(&jt->rand_state, &rand_val);
    return (uint32_t)rand_val % max;
}

void os_jitter_wait(OS_JITTER *jt, uint32_t max) {
    microsleep(os_jitter_ut(jt, max));
}
