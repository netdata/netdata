// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_setresuid(uid_t uid __maybe_unused, uid_t euid __maybe_unused, uid_t suid __maybe_unused) {
#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD)
    return setresuid(uid, euid, suid);
#endif

#if defined(COMPILED_FOR_MACOS)
    return setreuid(uid, euid);
#endif

    errno = ENOSYS;
    return -1;
}
