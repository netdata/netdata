// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

int os_adjtimex(struct timex *buf __maybe_unused) {
#if defined(OS_MACOS) || defined(OS_FREEBSD)
    return ntp_adjtime(buf);
#endif

#if defined(OS_LINUX)
    return adjtimex(buf);
#endif

    errno = ENOSYS;
    return -1;
}
