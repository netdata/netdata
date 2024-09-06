// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "windows-events.h"

typedef struct {
    uint64_t id;
    uint16_t event_id;
    uint8_t  level;
    uint64_t keywords;
    ND_UUID  correlation_activity_id;
    nsec_t   created_ns;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct {
    char *data;
    size_t size; // the allocated size of data buffer
    size_t len;  // the used size of the data buffer (including null terminators, if any)
} TXT_UTF8;

typedef struct {
    wchar_t *data;
    size_t size; // the allocated size of data buffer
    size_t len;  // the used size of the data buffer (including null terminators, if any)
} TXT_UNICODE;

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
    } ops;

} WEVT_LOG;

void wevt_closelog6(WEVT_LOG *log);
WEVT_LOG *wevt_openlog6(const wchar_t *channel, bool file_size);

static inline size_t compute_new_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % 4096 == 0) ? required_size : required_size + 4096;
    size = (size / 4096) * 4096;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

static inline void txt_utf8_cleanup(TXT_UTF8 *utf8) {
    freez(utf8->data);
}

static inline void txt_utf8_resize(TXT_UTF8 *utf8, size_t required_size) {
    if(required_size < utf8->size)
        return;

    txt_utf8_cleanup(utf8);
    utf8->size = compute_new_size(utf8->size, required_size);
    utf8->data = mallocz(utf8->size);
}

static inline void txt_unicode_cleanup(TXT_UNICODE *unicode) {
    freez(unicode->data);
}

static inline void txt_unicode_resize(TXT_UNICODE *unicode, size_t required_size) {
    if(required_size < unicode->size)
        return;

    txt_unicode_cleanup(unicode);
    unicode->size = compute_new_size(unicode->size, required_size);
    unicode->data = mallocz(unicode->size);
}

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
