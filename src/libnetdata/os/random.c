// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if !defined(HAVE_ARC4RANDOM_BUF) && !defined(HAVE_RAND_S)
static SPINLOCK random_lock = NETDATA_SPINLOCK_INITIALIZER;
static __attribute__((constructor)) void random_seed() {
    // Use current time and process ID to create a high-entropy seed
    struct timeval tv;
    gettimeofday(&tv, NULL);

    uint32_t seed = (uint32_t)(tv.tv_sec ^ tv.tv_usec ^ getpid());

    // Seed the random number generator
    srandom(seed);
}

static void random_get_bytes(void *buf, size_t bytes) {
    spinlock_lock(&random_lock);
    while (bytes > 0) {
        if (bytes >= sizeof(uint32_t)) {
            // Generate 4 bytes at a time
            uint32_t temp = random();
            memcpy(buf, &temp, sizeof(uint32_t));
            buf = (uint8_t *)buf + sizeof(uint32_t);
            bytes -= sizeof(uint32_t);
        } else if (bytes >= sizeof(uint16_t)) {
            // Generate 2 bytes at a time
            uint16_t temp = random();
            memcpy(buf, &temp, sizeof(uint16_t));
            buf = (uint8_t *)buf + sizeof(uint16_t);
            bytes -= sizeof(uint16_t);
        } else {
            // Generate remaining bytes
            uint32_t temp = random();
            for (size_t i = 0; i < bytes; i++) {
                ((uint8_t *)buf)[i] = temp & 0xFF;
                temp >>= 8;
            }
            bytes = 0;
        }
    }
    spinlock_unlock(&random_lock);
}

#if defined(HAVE_GETRANDOM)
#include <sys/random.h>
void inline getrandom_get_bytes(void *buf, size_t buflen) {
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
                // fallback to RAND_bytes
                random_get_bytes(buf, buflen);
                return;
            }
        }
        buf = (uint8_t *)buf + result;
        buflen -= result;
    }
}
#endif // HAVE_GETRANDOM
#endif // !HAVE_ARC4RANDOM_BUF && !HAVE_RAND_S

#if defined(HAVE_RAND_S)
static inline void rand_s_get_bytes(void *buf, size_t bytes) {
    while (bytes > 0) {
        if (bytes >= sizeof(unsigned int)) {
            unsigned int temp;
            rand_s(&temp);
            memcpy(buf, &temp, sizeof(unsigned int));
            buf = (uint8_t *)buf + sizeof(unsigned int);
            bytes -= sizeof(unsigned int);
        } else if (bytes >= sizeof(uint16_t)) {
            // Generate 2 bytes at a time
            unsigned int t;
            rand_s(&t);
            uint16_t temp = t;
            memcpy(buf, &temp, sizeof(uint16_t));
            buf = (uint8_t *)buf + sizeof(uint16_t);
            bytes -= sizeof(uint16_t);
        } else {
            // Generate remaining bytes
            unsigned int temp;
            rand_s(&temp);
            for (size_t i = 0; i < sizeof(temp) && i < bytes; i++) {
                ((uint8_t *)buf)[0] = temp & 0xFF;
                temp >>= 8;
                buf = (uint8_t *)buf + 1;
                bytes--;
            }
        }
    }
}
#endif

inline void os_random_bytes(void *buf, size_t bytes) {
    if(RAND_bytes((unsigned char *)buf, bytes) == 1)
        return;

#if defined(HAVE_ARC4RANDOM_BUF)
    arc4random_buf(buf, bytes);
#elif defined(HAVE_GETRANDOM)
    getrandom_get_bytes(buf, bytes);
#elif defined(HAVE_RAND_S)
    rand_s_get_bytes(buf, bytes);
#else
    random_get_bytes(buf, bytes);
#endif
}

// Generate an 8-bit random number
uint8_t os_random8(void) {
    uint8_t value;
    os_random_bytes(&value, sizeof(value));
    return value;
}

// Generate a 16-bit random number
uint16_t os_random16(void) {
    uint16_t value;
    os_random_bytes(&value, sizeof(value));
    return value;
}

// Generate a 32-bit random number
uint32_t os_random32(void) {
    uint32_t value;
    os_random_bytes(&value, sizeof(value));
    return value;
}

// Generate a 64-bit random number
uint64_t os_random64(void) {
    uint64_t value;
    os_random_bytes(&value, sizeof(value));
    return value;
}

// return a random number 0 to max - 1
uint64_t os_random(uint64_t max) {
    if (max <= 1) return 0;

    if(max <= UINT8_MAX)
        return os_random8() % max;
    else if(max <= UINT16_MAX)
        return os_random16() % max;
    else if(max <= UINT32_MAX)
        return os_random32() % max;
    else
        return os_random64() % max;
}
