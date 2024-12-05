// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include <random>
#include <cstring>

static thread_local std::mt19937_64 engine{std::random_device{}()};

void os_random_bytes(void *buf, size_t bytes)
{
    uint8_t *ptr = static_cast<uint8_t *>(buf);
    while (bytes >= sizeof(uint64_t)) {
        uint64_t value = engine();
        std::memcpy(ptr, &value, sizeof(uint64_t));
        ptr += sizeof(uint64_t);
        bytes -= sizeof(uint64_t);
    }
    if (bytes > 0) {
        uint64_t value = engine();
        std::memcpy(ptr, &value, bytes);
    }
}

uint64_t os_random(uint64_t max)
{
    if (max <= 1)
        return 0;

    uint64_t mask = max - 1;
    mask |= mask >> 1;
    mask |= mask >> 2;
    mask |= mask >> 4;
    mask |= mask >> 8;
    mask |= mask >> 16;
    mask |= mask >> 32;

    while (true) {
        uint64_t value = engine() & mask;
        if (value < max)
            return value;
    }
}

uint8_t os_random8(void)
{
    return static_cast<uint8_t>(engine());
}

uint16_t os_random16(void)
{
    return static_cast<uint16_t>(engine());
}

uint32_t os_random32(void)
{
    return static_cast<uint32_t>(engine());
}

uint64_t os_random64(void)
{
    return static_cast<uint64_t>(engine());
}
