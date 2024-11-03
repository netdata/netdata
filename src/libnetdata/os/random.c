// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if !defined(HAVE_ARC4RANDOM_BUF) && !defined(HAVE_RAND_S)
static SPINLOCK random_lock = NETDATA_SPINLOCK_INITIALIZER;
static __attribute__((constructor)) void seed_random() {
    // Use current time and process ID to create a high-entropy seed
    struct timeval tv;
    gettimeofday(&tv, NULL);

    uint32_t seed = (uint32_t)(tv.tv_sec ^ tv.tv_usec ^ getpid());

    // Seed the random number generator
    srandom(seed);
}

#if defined(HAVE_GETRANDOM)
#include <sys/random.h>
void getrandom_helper(void *buf, size_t buflen) {
    ssize_t result;
    while (buflen > 0) {
        result = getrandom(buf, buflen, 0);
        if (result == -1) {
            if (errno == EINTR) {
                // Interrupted, retry
                continue;
            } else if (errno == EAGAIN) {
                // Insufficient entropy; wait and retry
                tinysleep();
                continue;
            } else {
                // Fallback to using random() with a spinlock
                spinlock_lock(&random_lock);
                while (buflen > 0) {
                    if (buflen >= sizeof(uint32_t)) {
                        // Generate 4 bytes at a time
                        uint32_t temp = random();
                        memcpy(buf, &temp, sizeof(uint32_t));
                        buf = (uint8_t *)buf + sizeof(uint32_t);
                        buflen -= sizeof(uint32_t);
                    } else if (buflen >= sizeof(uint16_t)) {
                        // Generate 2 bytes at a time
                        uint16_t temp = random();
                        memcpy(buf, &temp, sizeof(uint16_t));
                        buf = (uint8_t *)buf + sizeof(uint16_t);
                        buflen -= sizeof(uint16_t);
                    } else {
                        // Generate remaining bytes
                        uint32_t temp = random();
                        for (size_t i = 0; i < buflen; i++) {
                            ((uint8_t *)buf)[i] = temp & 0xFF;
                            temp >>= 8;
                        }
                        buflen = 0;
                    }
                }
                spinlock_unlock(&random_lock);
                return;
            }
        }
        buf = (uint8_t *)buf + result;
        buflen -= result;
    }
}
#endif // HAVE_GETRANDOM
#endif // !HAVE_ARC4RANDOM_BUF && !HAVE_RAND_S

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

#elif defined(HAVE_GETRANDOM)
    if (max <= UINT8_MAX) {
        uint8_t v;
        getrandom_helper(&v, sizeof(v), 0);
        value = v;
    } else if(max <= UINT16_MAX) {
        uint16_t v;
        getrandom_helper(&v, sizeof(v), 0);
        value = v;
    } else if (max <= UINT32_MAX) {
        uint32_t v;
        getrandom_helper(&v, sizeof(v), 0);
        value = v;
    } else
        getrandom_helper(&value, sizeof(value), 0);

#else
    spinlock_lock(&random_lock);
    if(max <= INT32_MAX)
        value = random();
    else
        value = ((uint64_t) random() << 33) | ((uint64_t) random() << 2) | (random() & 0x3);
    spinlock_unlock(&random_lock);
#endif

    return value % max;
}

// Generate an 8-bit random number
uint8_t os_random8(void) {
    uint8_t value;

#if defined(HAVE_ARC4RANDOM_BUF)
    arc4random_buf(&value, sizeof(value));
#elif defined(HAVE_GETRANDOM)
    getrandom_helper(&value, sizeof(value), 0);
#elif defined(HAVE_RAND_S)
    unsigned int temp;
    rand_s(&temp);
    value = (uint8_t)temp;
#else
    spinlock_lock(&random_lock);
    value = (uint8_t)random();
    spinlock_unlock(&random_lock);
#endif

    return value;
}

// Generate a 16-bit random number
uint16_t os_random16(void) {
    uint16_t value;

#if defined(HAVE_ARC4RANDOM_BUF)
    arc4random_buf(&value, sizeof(value));
#elif defined(HAVE_GETRANDOM)
    getrandom_helper(&value, sizeof(value), 0);
#elif defined(HAVE_RAND_S)
    unsigned int temp;
    rand_s(&temp);
    value = (uint16_t)temp;
#else
    spinlock_lock(&random_lock);
    value = (uint16_t)random();
    spinlock_unlock(&random_lock);
#endif

    return value;
}

// Generate a 32-bit random number
uint32_t os_random32(void) {
    uint32_t value;

#if defined(HAVE_ARC4RANDOM_BUF)
    arc4random_buf(&value, sizeof(value));
#elif defined(HAVE_GETRANDOM)
    getrandom_helper(&value, sizeof(value), 0);
#elif defined(HAVE_RAND_S)
    unsigned int temp;
    rand_s(&temp);
    value = temp;
#else
    spinlock_lock(&random_lock);
    value = random();
    spinlock_unlock(&random_lock);
#endif

    return value;
}

// Generate a 64-bit random number
uint64_t os_random64(void) {
    uint64_t value;

#if defined(HAVE_ARC4RANDOM_BUF)
    arc4random_buf(&value, sizeof(value));
#elif defined(HAVE_GETRANDOM)
    getrandom_helper(&value, sizeof(value), 0);
#elif defined(HAVE_RAND_S)
    unsigned int temp_lo, temp_hi;
    rand_s(&temp_lo);
    rand_s(&temp_hi);
    value = ((uint64_t)temp_hi << 32) | (uint64_t)temp_lo;
#else
    spinlock_lock(&random_lock);
    value = ((uint64_t)random() << 33) | ((uint64_t)random() << 2) | (random() & 0x3);
    spinlock_unlock(&random_lock);
#endif

    return value;
}
