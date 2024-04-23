// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ADJTIMEX_H
#define NETDATA_ADJTIMEX_H

#if defined(COMPILED_FOR_LINUX) || defined(COMPILED_FOR_FREEBSD) || defined(COMPILED_FOR_MACOS)
#include <sys/timex.h>
#endif

struct timex;
int os_adjtimex(struct timex *buf);

#endif //NETDATA_ADJTIMEX_H
