// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "windows-events.h"

typedef struct {
    uint64_t id;
    uint16_t event_id;
    uint16_t opcode;
    uint8_t  level;
    uint64_t keywords;
    ND_UUID  correlation_activity_id;
    nsec_t   created_ns;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct wevt_log {
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
        // temp buffer used for rendering event log messages
        // never use directly
        struct {
            EVT_VARIANT	*data;
            size_t size;
        } content;

        // temp buffer used for fetching and converting UNICODE and UTF-8
        // every string operation overwrites it, multiple times per event log entry
        // it can be used within any function, for its own purposes,
        // but never share between functions
        TXT_UNICODE unicode;

        // string attributes of the current event log entry
        // valid until another event if fetched
        TXT_UTF8 message;
        TXT_UTF8 provider;
        TXT_UTF8 source;
        TXT_UTF8 computer;
        TXT_UTF8 user;
        TXT_UTF8 opcode;
        TXT_UTF8 level;
        TXT_UTF8 keyword;
    } ops;

} WEVT_LOG;

void wevt_closelog6(WEVT_LOG *log);
WEVT_LOG *wevt_openlog6(const wchar_t *channel, bool file_size);
bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev);

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
