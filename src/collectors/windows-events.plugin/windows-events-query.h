// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "windows-events.h"

#define BATCH_NEXT_EVENT 500

typedef struct wevt_event {
    uint64_t id;                        // EventRecordId (unique and sequential per channel)
    uint16_t event_id;                  // This is the template that defines the message to be shown
    uint8_t  version;
    uint8_t  level;                     // The severity of event
    uint64_t keywords;                  // Categorization of the event
    uint8_t  opcode;                    // we receive this as 8bit, but publishers use 32bit
    uint16_t task;
    uint32_t process_id;
    uint32_t thread_id;
    ND_UUID  provider;
    ND_UUID  correlation_activity_id;
    nsec_t   created_ns;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct {
    EVT_VARIANT	*data;
    size_t size;
    size_t used;
} WEVT_VARIANT;

typedef struct {
    WEVT_EVENT first_event;
    WEVT_EVENT last_event;

    uint64_t entries;
    nsec_t duration_ns;
    uint64_t size_bytes;
} EVT_RETENTION;

struct provider_meta_handle;

typedef struct wevt_log {
    struct {
        DWORD size;
        DWORD used;
        EVT_HANDLE bk[BATCH_NEXT_EVENT];
    } batch;

    EVT_HANDLE bookmark;
    EVT_HANDLE event_query;
    EVT_HANDLE render_context;
    struct provider_meta_handle *publisher;

    struct {
        // temp buffer used for rendering event log messages
        // never use directly
        WEVT_VARIANT content;

        // temp buffer used for fetching and converting UNICODE and UTF-8
        // every string operation overwrites it, multiple times per event log entry
        // it can be used within any function, for its own purposes,
        // but never share between functions
        TXT_UNICODE unicode;

        // string attributes of the current event log entry
        // valid until another event if fetched

        // IMPORTANT:
        // EVERY FIELD NEEDS ITS OWN BUFFER!
        // the way facets work, all the field value pointers need to be valid
        // until the entire row closes, so reusing a buffer for the same field
        // actually copies the same value to all fields using the same buffer.

        TXT_UTF8 channel;
        TXT_UTF8 provider;
        TXT_UTF8 source;
        TXT_UTF8 computer;
        TXT_UTF8 user;

        TXT_UTF8 event;
        TXT_UTF8 level;
        TXT_UTF8 keywords;
        TXT_UTF8 opcode;
        TXT_UTF8 task;
        TXT_UTF8 xml;
    } ops;

    struct {
        size_t event_count;
        size_t failed_count;
    } query_stats;

    struct {
        size_t queries_count;
        size_t queries_failed;

        size_t event_count;
        size_t failed_count;
    } log_stats;

} WEVT_LOG;

WEVT_LOG *wevt_openlog6(void);
void wevt_closelog6(WEVT_LOG *log);

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, EVT_RETENTION *retention);

bool wevt_query(WEVT_LOG *log, LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction);
void wevt_query_done(WEVT_LOG *log);

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev, bool full);

bool wevt_get_message_unicode(TXT_UNICODE *unicode, EVT_HANDLE hMetadata, EVT_HANDLE bookmark, DWORD dwMessageId, EVT_FORMAT_MESSAGE_FLAGS flags);

struct provider_meta_handle;
bool wevt_get_event_utf8(WEVT_LOG *log, struct provider_meta_handle *p, EVT_HANDLE event_handle, TXT_UTF8 *dst);
bool wevt_get_xml_utf8(WEVT_LOG *log, struct provider_meta_handle *p, EVT_HANDLE event_handle, TXT_UTF8 *dst);

static inline void wevt_variant_cleanup(WEVT_VARIANT *v) {
    freez(v->data);
}

static inline void wevt_variant_resize(WEVT_VARIANT *v, size_t required_size) {
    if(required_size < v->size)
        return;

    wevt_variant_cleanup(v);
    v->size = compute_new_size(v->size, required_size);
    v->data = mallocz(v->size);
}

static inline uint8_t wevt_field_get_uint8(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeByte);
    return ev->ByteVal;
}

static inline uint16_t wevt_field_get_uint16(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt16);
    return ev->UInt16Val;
}

static inline uint32_t wevt_field_get_uint32(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt32);
    return ev->UInt32Val;
}

static inline uint64_t wevt_field_get_uint64(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt64);
    return ev->UInt64Val;
}

static inline uint64_t wevt_field_get_uint64_hex(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeHexInt64);
    return ev->UInt64Val;
}

static inline bool wevt_field_get_string_utf8(EVT_VARIANT *ev, TXT_UTF8 *dst) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull) {
        wevt_utf8_empty(dst);
        return false;
    }

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeString);
    return wevt_str_wchar_to_utf8(dst, ev->StringVal, -1);
}

bool wevt_convert_user_id_to_name(PSID sid, TXT_UTF8 *dst);

static inline bool wevt_field_get_sid(EVT_VARIANT *ev, TXT_UTF8 *dst) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull) {
        wevt_utf8_empty(dst);
        return false;
    }

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeSid);
    return wevt_convert_user_id_to_name(ev->SidVal, dst);
}

static inline uint64_t wevt_field_get_filetime_to_ns(EVT_VARIANT *ev) {
    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeFileTime);
    return os_windows_ulonglong_to_unix_epoch_ns(ev->FileTimeVal);
}

static inline bool wevt_GUID_to_ND_UUID(ND_UUID *nd_uuid, const GUID *guid) {
    if(guid && sizeof(GUID) == sizeof(ND_UUID)) {
        memcpy(nd_uuid->uuid, guid, sizeof(ND_UUID));
        return true;
    }
    else {
        *nd_uuid = UUID_ZERO;
        return false;
    }
}

static inline bool wevt_get_uuid_by_type(EVT_VARIANT *ev, ND_UUID *dst) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull) {
        wevt_GUID_to_ND_UUID(dst, NULL);
        return false;
    }

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeGuid);
    return wevt_GUID_to_ND_UUID(dst, ev->GuidVal);
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-severity-levels
static inline bool is_valid_publisher_level(uint64_t level) {
    // the spec says >= 16, but many publishers redefine the standard ones (<=5)
    // so we remove the lower bound
    return level <= 255;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-tasks-and-opcodes
static inline bool is_valid_publisher_opcode(uint64_t opcode) {
    // the spec says >= 16, but many publishers redefine the standard ones (<=10)
    // so we remove the lower bound
    return opcode <= 239;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-tasks-and-opcodes
static inline bool is_valid_publisher_task(uint64_t task) {
    return task > 0 && task <= 0xFFFF;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-keywords-used-to-classify-types-of-events
static inline bool is_valid_publisher_keywords(uint64_t keyword) {
    return keyword > 0 && keyword <= 0x0000FFFFFFFFFFFF;
}

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
