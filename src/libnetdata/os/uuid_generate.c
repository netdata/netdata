// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifdef COMPILED_FOR_WINDOWS
#include <windows.h>

void os_uuid_generate(void *out) {
    RPC_STATUS status = UuidCreate(out);
    while (status != RPC_S_OK && status != RPC_S_UUID_LOCAL_ONLY) {
        tinysleep();
        status = UuidCreate(out);
    }
}

#endif
