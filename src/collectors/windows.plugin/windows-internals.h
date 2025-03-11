// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_INTERNALS_H
#define NETDATA_WINDOWS_INTERNALS_H

#include "libnetdata/libnetdata.h"

static inline ULONGLONG FileTimeToULL(FILETIME ft)
{
    ULARGE_INTEGER ul;
    ul.LowPart = ft.dwLowDateTime;
    ul.HighPart = ft.dwHighDateTime;
    return ul.QuadPart;
}

#include "perflib-rrd.h"

#endif //NETDATA_WINDOWS_INTERNALS_H
