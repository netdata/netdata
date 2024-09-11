// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "windows-events.h"

typedef struct wevt_event {
    uint64_t id;                        // EventRecordId (unique and sequential per channel)
    uint16_t event_id;                  // This is the template that defines the message to be shown
    uint16_t opcode;
    uint8_t  level;                     // The severity of event
    uint8_t  version;
    uint16_t task;
    uint32_t process_id;
    uint32_t thread_id;
    uint64_t keyword;                   // Categorization of the event
    ND_UUID  provider;
    ND_UUID  correlation_activity_id;
    nsec_t   created_ns;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct {
    WEVT_EVENT first_event;
    WEVT_EVENT last_event;

    uint64_t entries;
    nsec_t duration_ns;
    uint64_t size_bytes;
} EVT_RETENTION;

typedef struct wevt_log {
    EVT_HANDLE event_query;
    EVT_HANDLE render_context;

    struct {
        // temp buffer used for rendering event log messages
        // never use directly
        struct {
            EVT_VARIANT	*data;
            size_t size;
            size_t len;
        } content;

        // temp buffer used for fetching and converting UNICODE and UTF-8
        // every string operation overwrites it, multiple times per event log entry
        // it can be used within any function, for its own purposes,
        // but never share between functions
        TXT_UNICODE unicode;

        // string attributes of the current event log entry
        // valid until another event if fetched
        TXT_UTF8 channel;
        TXT_UTF8 provider;
        TXT_UTF8 source;
        TXT_UTF8 computer;
        TXT_UTF8 event;
        TXT_UTF8 user;
        TXT_UTF8 opcode;
        TXT_UTF8 keyword;
        TXT_UTF8 xml;
    } ops;

} WEVT_LOG;

WEVT_LOG *wevt_openlog6(void);
void wevt_closelog6(WEVT_LOG *log);

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, EVT_RETENTION *retention);

EVT_HANDLE wevt_query(LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction);
void wevt_query_done(WEVT_LOG *log);

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev, bool full);

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
