// SPDX-License-Identifier: GPL-3.0-or-later

#if !defined(HAVE_STRNDUP) && !(defined(NETDATA_TRACE_ALLOCATIONS) && defined(HAVE_DLSYM) && defined(ENABLE_DLSYM))
#include "../libnetdata.h"

char *os_strndup(const char *s1, size_t n) {
    size_t bytes = strnlen(s1, n);
    if (unlikely(bytes == SIZE_MAX)) {
        errno = ENOMEM;
        return NULL;
    }

    // cppcheck-suppress memleak
    char *copy = malloc(bytes + 1);
    if (!copy)
        return NULL;

    memcpy(copy, s1, bytes);
    copy[bytes] = 0;
    return copy;
}
#endif
