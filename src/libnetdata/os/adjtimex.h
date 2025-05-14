// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ADJTIMEX_H
#define NETDATA_ADJTIMEX_H

#ifdef OS_MACOS
#include <AvailabilityMacros.h>
#endif

#if defined(OS_LINUX) || defined(OS_FREEBSD) || \
    (defined(OS_MACOS) && (MAC_OS_X_VERSION_MIN_REQUIRED >= 101300))
#include <sys/timex.h>
#endif

struct timex;
int os_adjtimex(struct timex *buf);

#endif //NETDATA_ADJTIMEX_H
