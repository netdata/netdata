// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HAVE_STRNDUP
#include "../libnetdata.h"

static inline char *os_strndup( const char *s1, size_t n)
{
    size_t bytes = strnlen(s1, n);
    if (unlikely(bytes == SIZE_MAX))
        return NULL;

    char *copy= (char*)malloc( bytes+1 );
    memcpy( copy, s1, bytes );
    copy[bytes] = 0;
    return copy;
};
#endif
