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
        // an empty value means "not set", as for NETDATA_RUN_DIR - otherwise it
        // would name the empty path
        const char *env_pipename = getenv("NETDATA_PIPENAME");
        if (env_pipename && *env_pipename)
            cached_pipename = strdupz(env_pipename);
        else {
            //#if defined(OS_WINDOWS)
            // cached_pipename = strdupz("\\\\?\\pipe\\netdata-cli");
            //#else
            // os_run_dir() returns NULL when there is no usable run directory.
            // Stay NULL too instead of formatting a name from it: a guessed path is
            // what the run-dir validation exists to prevent, and "%s" with NULL
            // would name "(null)/netdata.pipe". Callers report it.
            const char *run_dir = os_run_dir(false);
            if (run_dir && *run_dir) {
                char filename[FILENAME_MAX + 1];
                snprintfz(filename, FILENAME_MAX, "%s/netdata.pipe", run_dir);
                cached_pipename = strdupz(filename);
            }
            else
                netdata_log_error(
                    "Cannot determine the netdata run directory, so the netdatacli socket has no "
                    "name. Set NETDATA_RUN_DIR or NETDATA_PIPENAME.");
            //#endif
        }

        pipename = cached_pipename;
        __atomic_store_n(&cached_pipename_available, true, __ATOMIC_RELEASE);
    }

    spinlock_unlock(&spinlock);

    return pipename;
}
