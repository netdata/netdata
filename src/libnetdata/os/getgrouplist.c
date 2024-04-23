// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_getgrouplist(const char *username __maybe_unused, gid_t gid __maybe_unused, gid_t *supplementary_groups __maybe_unused, int *ngroups __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD)
    return getgrouplist(username, gid, supplementary_groups, ngroups);
#endif

#if defined(COMPILED_FOR_MACOS)
    return getgrouplist(username, gid, (int *)supplementary_groups, ngroups);
#endif

    errno = ENOSYS;
    return -1;
}
