// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(HAVE_GETRANDOM)
#include <sys/random.h>
#endif

#if !defined(HAVE_ARC4RANDOM_BUF) && !defined(HAVE_GETRANDOM) && !defined(HAVE_RAND_S)
static __attribute__((constructor)) void seed_random() {
    // Use current time and process ID to create a high-entropy seed
    struct timeval tv;
    gettimeofday(&tv, NULL);

    uint32_t seed = (uint32_t)(tv.tv_sec ^ tv.tv_usec ^ getpid());

    // Seed the random number generator
    srandom(seed);
}
#endif

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max) {
    if (max <= 1) return 0;

    uint64_t value;

#if defined(HAVE_ARC4RANDOM_BUF)
    if (max <= UINT8_MAX) {
        uint8_t v;
        arc4random_buf(&v, sizeof(v));
        value = v;
    } else if(max <= UINT16_MAX) {
        uint16_t v;
        arc4random_buf(&v, sizeof(v));
        value = v;
    } else if (max <= UINT32_MAX) {
        uint32_t v;
        arc4random_buf(&v, sizeof(v));
        value = v;
    } else
        arc4random_buf(&value, sizeof(value));

#elif defined(HAVE_GETRANDOM)
    if (max <= UINT8_MAX) {
        uint8_t v;
        getrandom(&v, sizeof(v), 0);
        value = v;
    } else if(max <= UINT16_MAX) {
        uint16_t v;
        getrandom(&v, sizeof(v), 0);
        value = v;
    } else if (max <= UINT32_MAX) {
        uint32_t v;
        getrandom(&v, sizeof(v), 0);
        value = v;
    } else
        getrandom(&value, sizeof(value), 0);

#elif defined(HAVE_RAND_S)
    if (max <= UINT_MAX) {
        unsigned int temp;
        rand_s(&temp);
        value = temp;
    } else {
        unsigned int temp_lo, temp_hi;
        rand_s(&temp_lo);
        rand_s(&temp_hi);
        value = ((uint64_t)temp_hi << 32) + (uint64_t)temp_lo;
    }

#else
    if(max <= INT32_MAX)
        value = random();
    else
        value = ((uint64_t) random() << 33) | ((uint64_t) random() << 2) | (random() & 0x3);
#endif

    return value % max;
}
