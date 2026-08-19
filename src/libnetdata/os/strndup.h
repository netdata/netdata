// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef STRNDUP_H
#define STRNDUP_H

#include "config.h"

#include <stddef.h>

#if !defined(HAVE_STRNDUP) && !(defined(NETDATA_TRACE_ALLOCATIONS) && defined(HAVE_DLSYM) && defined(ENABLE_DLSYM))
char *os_strndup(const char *s, size_t n);
#undef strndup
#define strndup(s, n) os_strndup(s, n)
#endif

#endif //STRNDUP_H
