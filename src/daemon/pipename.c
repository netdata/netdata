// SPDX-License-Identifier: GPL-3.0-or-later

#include "pipename.h"

#include "libnetdata/libnetdata.h"

static const char *cached_pipename = NULL;
static bool cached_pipename_available = false;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

const char *daemon_pipename(void) {
    if(__atomic_load_n(&cached_pipename_available, __ATOMIC_ACQUIRE))
        return cached_pipename;

    spinlock_lock(&spinlock);

    const char *pipename = cached_pipename;
    if(!pipename) {
        cached_pipename = nd_environment_get_dup("NETDATA_PIPENAME");
        if (!cached_pipename) {
            //#if defined(OS_WINDOWS)
            // cached_pipename = strdupz("\\\\?\\pipe\\netdata-cli");
            //#else
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "%s/netdata.pipe", os_run_dir(false));
            cached_pipename = strdupz(filename);
            //#endif
        }

        pipename = cached_pipename;
        __atomic_store_n(&cached_pipename_available, true, __ATOMIC_RELEASE);
    }

    spinlock_unlock(&spinlock);

    return pipename;
}
