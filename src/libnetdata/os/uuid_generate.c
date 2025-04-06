// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#undef uuid_generate
#undef uuid_generate_random
#undef uuid_generate_time

#ifdef OS_WINDOWS
void os_uuid_generate(void *out) {
    RPC_STATUS status = UuidCreate(out);
    while (status != RPC_S_OK && status != RPC_S_UUID_LOCAL_ONLY) {
        tinysleep();
        status = UuidCreate(out);
    }
}

void os_uuid_generate_random(void *out) {
    os_uuid_generate(out);
}

void os_uuid_generate_time(void *out) {
    os_uuid_generate(out);
}

#else

#if !defined(OS_MACOS)
#include <uuid.h>
#endif

void os_uuid_generate(void *out) {
    // IMPORTANT: this generates a UUIDv4, which is random
    // and falls back to uuid_generate_time() if high resolution random generated is not available
    uuid_generate(out);
}

void os_uuid_generate_random(void *out) {
    uuid_generate_random(out);
}

void os_uuid_generate_time(void *out) {
    // IMPORTANT: this generates a UUIDv1, which is not random and may suffer from collisions
    uuid_generate_time(out);
}

#endif
