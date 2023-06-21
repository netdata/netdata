// SPDX-License-Identifier: GPL-3.0-or-later

#include "pipename.h"

#include <stdlib.h>

const char *daemon_pipename(void) {
    const char *pipename = getenv("NETDATA_PIPENAME");
    if (pipename)
        return pipename;

#ifdef _WIN32
    return "\\\\?\\pipe\\netdata-cli";
#else
    return "/tmp/netdata-ipc";
#endif
}
