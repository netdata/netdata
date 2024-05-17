// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ADJTIMEX_H
#define NETDATA_ADJTIMEX_H

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/timex.h>
#endif

struct timex;
int os_adjtimex(struct timex *buf);

#endif //NETDATA_ADJTIMEX_H
