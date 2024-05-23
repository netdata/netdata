// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_setresgid(gid_t gid __maybe_unused, gid_t egid __maybe_unused, gid_t sgid __maybe_unused) {
#if defined(OS_LINUX) || defined(OS_FREEBSD)
    return setresgid(gid, egid, sgid);
#endif

#if defined(OS_MACOS)
    return setregid(gid, egid);
#endif

    errno = ENOSYS;
    return -1;
}
