// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_H
#define NETDATA_WINDOWS_EVENTS_H

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"
#include <windows.h>
#include <winevt.h>
#include <wchar.h>

typedef enum {
    WEVT_NO_CHANNEL_MATCHED,
    WEVT_FAILED_TO_OPEN,
    WEVT_FAILED_TO_SEEK,
    WEVT_TIMED_OUT,
    WEVT_OK,
    WEVT_NOT_MODIFIED,
    WEVT_CANCELLED,
} WEVT_QUERY_STATUS;

#include "windows-events-unicode.h"
#include "windows-events-query.h"
#include "windows-events-sources.h"
#include "windows-events-sid.h"
#include "windows-events-xml.h"

#endif //NETDATA_WINDOWS_EVENTS_H
