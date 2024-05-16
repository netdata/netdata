// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HAVE_STRNDUP
#include "../libnetdata.h"

static inline char *os_strndup( const char *s1, size_t n)
{
    char *copy= (char*)malloc( n+1 );
    memcpy( copy, s1, n );
    copy[n] = 0;
    return copy;
};
#endif
