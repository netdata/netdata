// SPDX-License-Identifier: GPL-3.0-or-later

#include "netipc_netdata.h"

#if defined(OS_LINUX) || defined(OS_WINDOWS)

#include "libnetdata/xxHash/xxhash.h"

uint64_t netipc_auth_token(void) {
    ND_UUID id = nd_log_get_invocation_id();
    return XXH3_64bits(id.uuid, sizeof(id.uuid));
}

#endif // OS_LINUX || OS_WINDOWS
