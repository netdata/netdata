// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "windows-events.h"

typedef struct {
    uint64_t id;
    nsec_t created_ns;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct {
    EVT_HANDLE event_query;
    EVT_HANDLE render_context;

    struct {
        WEVT_EVENT first_event;
        WEVT_EVENT last_event;

        uint64_t entries;
        nsec_t duration_ns;
        uint64_t size_bytes;
    } retention;

    struct {
        EVT_VARIANT	*renderedContent;
    } ops;

} WEVT_LOG;

void wevt_closelog6(WEVT_LOG *log);
WEVT_LOG *wevt_openlog6(const wchar_t *channel, bool file_size);

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
