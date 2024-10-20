// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_QUERY_H
#define NETDATA_WINDOWS_EVENTS_QUERY_H

#include "libnetdata/libnetdata.h"
#include "windows-events.h"

#define BATCH_NEXT_EVENT 500

typedef struct wevt_event {
    uint64_t id;                        // EventRecordId (unique and sequential per channel)
    uint8_t  version;
    uint8_t  level;                     // The severity of event
    uint8_t  opcode;                    // we receive this as 8bit, but providers use 32bit
    uint16_t event_id;                  // This is the template that defines the message to be shown
    uint16_t task;
    uint16_t qualifiers;
    uint32_t process_id;
    uint32_t thread_id;
    uint64_t keywords;                  // Categorization of the event
    ND_UUID  provider;
    ND_UUID  activity_id;
    ND_UUID  related_activity_id;
    nsec_t   created_ns;
    WEVT_PROVIDER_PLATFORM platform;
} WEVT_EVENT;

#define WEVT_EVENT_EMPTY (WEVT_EVENT){ .id = 0, .created_ns = 0, }

typedef struct {
    EVT_VARIANT	*data;
    DWORD size;
    DWORD used;
    DWORD count;
} WEVT_VARIANT;

typedef struct {
    WEVT_EVENT first_event;
    WEVT_EVENT last_event;

    uint64_t entries;
    nsec_t duration_ns;
    uint64_t size_bytes;
} EVT_RETENTION;

struct provider_meta_handle;

typedef enum __attribute__((packed)) {
    WEVT_QUERY_BASIC        = (1 << 0),
    WEVT_QUERY_EXTENDED     = (1 << 1),
    WEVT_QUERY_EVENT_DATA   = (1 << 2),
} WEVT_QUERY_TYPE;

#define WEVT_QUERY_RETENTION  WEVT_QUERY_BASIC
#define WEVT_QUERY_NORMAL    (WEVT_QUERY_BASIC | WEVT_QUERY_EXTENDED)
#define WEVT_QUERY_FTS       (WEVT_QUERY_BASIC | WEVT_QUERY_EXTENDED | WEVT_QUERY_EVENT_DATA)

typedef struct wevt_log {
    struct {
        DWORD size;
        DWORD used;
        EVT_HANDLE hEvents[BATCH_NEXT_EVENT];
    } batch;

    EVT_HANDLE hEvent;
    EVT_HANDLE hQuery;
    EVT_HANDLE hRenderSystemContext;
    EVT_HANDLE hRenderUserContext;
    struct provider_meta_handle *provider;

    WEVT_QUERY_TYPE type;

    struct {
        struct {
            // temp buffer used for rendering event log messages
            // never use directly
            WEVT_VARIANT system;
            WEVT_VARIANT user;
        } raw;

        // temp buffer used for fetching and converting UNICODE and UTF-8
        // every string operation overwrites it, multiple times per event log entry
        // it can be used within any function, for its own purposes,
        // but never share between functions
        TXT_UTF16 unicode;

        // string attributes of the current event log entry
        // valid until another event if fetched

        // IMPORTANT:
        // EVERY FIELD NEEDS ITS OWN BUFFER!
        // the way facets work, all the field value pointers need to be valid
        // until the entire row closes, so reusing a buffer for the same field
        // actually copies the same value to all fields using the same buffer.

        TXT_UTF8 channel;
        TXT_UTF8 provider;
        TXT_UTF8 computer;
        TXT_UTF8 account;
        TXT_UTF8 domain;
        TXT_UTF8 sid;

        TXT_UTF8 event; // the message to be shown to the user
        TXT_UTF8 level;
        TXT_UTF8 keywords;
        TXT_UTF8 opcode;
        TXT_UTF8 task;
        TXT_UTF8 xml;

        BUFFER *event_data;
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

WEVT_LOG *wevt_openlog6(WEVT_QUERY_TYPE type);
void wevt_closelog6(WEVT_LOG *log);

bool wevt_channel_retention(WEVT_LOG *log, const wchar_t *channel, const wchar_t *query, EVT_RETENTION *retention);

bool wevt_query(WEVT_LOG *log, LPCWSTR channel, LPCWSTR query, EVT_QUERY_FLAGS direction);
void wevt_query_done(WEVT_LOG *log);

bool wevt_get_next_event(WEVT_LOG *log, WEVT_EVENT *ev);

bool EvtFormatMessage_utf16(
    TXT_UTF16 *dst, EVT_HANDLE hMetadata, EVT_HANDLE hEvent, DWORD dwMessageId, EVT_FORMAT_MESSAGE_FLAGS flags);

bool EvtFormatMessage_Event_utf8(TXT_UTF16 *tmp, struct provider_meta_handle *p, EVT_HANDLE hEvent, TXT_UTF8 *dst);
bool EvtFormatMessage_Xml_utf8(TXT_UTF16 *tmp, struct provider_meta_handle *p, EVT_HANDLE hEvent, TXT_UTF8 *dst);

void evt_variant_to_buffer(BUFFER *b, EVT_VARIANT *ev, const char *separator);

static inline void wevt_variant_cleanup(WEVT_VARIANT *v) {
    freez(v->data);
}

static inline void wevt_variant_resize(WEVT_VARIANT *v, size_t required_size) {
    if(required_size < v->size)
        return;

    wevt_variant_cleanup(v);
    v->size = txt_compute_new_size(v->size, required_size);
    v->data = mallocz(v->size);
}

static inline void wevt_variant_count_from_used(WEVT_VARIANT *v) {
    v->count = v->used / sizeof(*v->data);
}

static inline uint8_t wevt_field_get_uint8(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeByte);
    return ev->ByteVal;
}

static inline uint16_t wevt_field_get_uint16(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt16);
    return ev->UInt16Val;
}

static inline uint32_t wevt_field_get_uint32(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt32);
    return ev->UInt32Val;
}

static inline uint64_t wevt_field_get_uint64(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeUInt64);
    return ev->UInt64Val;
}

static inline uint64_t wevt_field_get_uint64_hex(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeHexInt64);
    return ev->UInt64Val;
}

static inline bool wevt_field_get_string_utf8(EVT_VARIANT *ev, TXT_UTF8 *dst) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull) {
        txt_utf8_empty(dst);
        return false;
    }

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeString);
    return wchar_to_txt_utf8(dst, ev->StringVal, -1);
}

bool cached_sid_to_account_domain_sidstr(PSID sid, TXT_UTF8 *dst_account, TXT_UTF8 *dst_domain, TXT_UTF8 *dst_sid_str);
static inline bool wevt_field_get_sid(EVT_VARIANT *ev, TXT_UTF8 *dst_account, TXT_UTF8 *dst_domain, TXT_UTF8 *dst_sid_str) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull) {
        txt_utf8_empty(dst_account);
        txt_utf8_empty(dst_domain);
        txt_utf8_empty(dst_sid_str);
        return false;
    }

    fatal_assert((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeSid);
    return cached_sid_to_account_domain_sidstr(ev->SidVal, dst_account, dst_domain, dst_sid_str);
}

static inline uint64_t wevt_field_get_filetime_to_ns(EVT_VARIANT *ev) {
    if((ev->Type & EVT_VARIANT_TYPE_MASK) == EvtVarTypeNull)
        return 0;

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
static inline bool is_valid_provider_level(uint64_t level, bool strict) {
    if(strict)
        // when checking if the name is provider independent
        return level >= 16 && level <= 255;
    else
        // when checking acceptable values in provider manifests
        return level <= 255;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-tasks-and-opcodes
static inline bool is_valid_provider_opcode(uint64_t opcode, bool strict) {
    if(strict)
        // when checking if the name is provider independent
        return opcode >= 10 && opcode <= 239;
    else
        // when checking acceptable values in provider manifests
        return opcode <= 255;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-tasks-and-opcodes
static inline bool is_valid_provider_task(uint64_t task, bool strict) {
    if(strict)
        // when checking if the name is provider independent
        return task > 0 && task <= 0xFFFF;
    else
        // when checking acceptable values in provider manifests
        return task <= 0xFFFF;
}

// https://learn.microsoft.com/en-us/windows/win32/wes/defining-keywords-used-to-classify-types-of-events
static inline bool is_valid_provider_keyword(uint64_t keyword, bool strict) {
    if(strict)
        // when checking if the name is provider independent
        return keyword > 0 && keyword <= 0x0000FFFFFFFFFFFF;
    else
        // when checking acceptable values in provider manifests
        return true;
}

#endif //NETDATA_WINDOWS_EVENTS_QUERY_H
