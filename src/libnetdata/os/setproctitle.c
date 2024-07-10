// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "setproctitle.h"

void os_setproctitle(const char *new_name, const int argc, const char **argv) {
#ifdef HAVE_SYS_PRCTL_H
    // Set the process name (comm)
    prctl(PR_SET_NAME, new_name, 0, 0, 0);
#endif

#ifdef __FreeBSD__
    // Set the process name on FreeBSD
    setproctitle("%s", new_name);
#endif

    if(argc && argv) {
        // replace with spaces all parameters found (except argv[0])
        for(int i = 1; i < argc ;i++) {
            char *s = (char *)&argv[i][0];
            while(*s != '\0') *s++ = ' ';
        }

        // overwrite argv[0]
        size_t len = strlen(new_name);
        const size_t argv0_len = strlen(argv[0]);
        strncpyz((char *)argv[0], new_name, MIN(len, argv0_len));
        while(len < argv0_len)
            ((char *)argv[0])[len++] = ' ';
    }
}
