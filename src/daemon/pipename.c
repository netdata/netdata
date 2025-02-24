// SPDX-License-Identifier: GPL-3.0-or-later

#include "pipename.h"

#include "libnetdata/libnetdata.h"

static const char *cached_pipename = NULL;

const char *daemon_pipename(void) {
    if(cached_pipename)
        return cached_pipename;

    const char *pipename = getenv("NETDATA_PIPENAME");
    if (pipename) {
        cached_pipename = strdupz(pipename);
        return pipename;
    }

//#if defined(OS_WINDOWS)
//    cached_pipename = strdupz("\\\\?\\pipe\\netdata-cli");
//    return cached_pipename;
//#else
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata.pipe", os_run_dir(false));
    cached_pipename = strdupz(filename);
    return cached_pipename;
//#endif
}
