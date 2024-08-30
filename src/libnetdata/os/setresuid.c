// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_setresuid(uid_t uid __maybe_unused, uid_t euid __maybe_unused, uid_t suid __maybe_unused) {
#if defined(OS_LINUX) || defined(OS_FREEBSD)
    return setresuid(uid, euid, suid);
#endif

#if defined(OS_MACOS)
    return setreuid(uid, euid);
#endif

    errno = ENOSYS;
    return -1;
}
