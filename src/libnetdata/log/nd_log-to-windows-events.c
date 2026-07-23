// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS) && defined(HAVE_ETW)

// Bounded wide-string length: like wcsnlen, but returns 0 (not maxlen) when the
// string is not null-terminated within `maxlen`. Callers use the result for
// (len + 1) * sizeof(wchar_t) buffer-size calculations and get a deterministic
// 0 instead of an out-of-bounds read on a malformed input.
//
// IMPORTANT: `maxlen` is the caller's claim about the maximum number of wide
// chars the string could contain, not the size of a `const wchar_t *` pointer.
// Do NOT pass `_countof(ptr_to_string)` for a `const wchar_t *ptr` argument —
// that evaluates to `sizeof(const wchar_t *) / sizeof(wchar_t)` (2 on 64-bit
// Windows, 1 on 32-bit), which is the size of the POINTER, not the size of
// the pointed-to string. Use a literal cap or `_countof(arr)` only when `arr`
// is a true wide-char array.
static inline size_t etw_wcslen_bounded(const wchar_t *s, size_t maxlen) {
    if (!s) return 0;
    size_t n = wcsnlen(s, maxlen);
    return (n == maxlen) ? 0 : n;
}

// --------------------------------------------------------------------------------------------------------------------
// construct an event id

// load message resources generated header
#include "wevt_netdata.h"

// include the common definitions with the message resources and manifest generator
#include "nd_log-to-windows-common.h"

#if defined(HAVE_ETW)
// we need the manifest, only in ETW mode

// eliminate compiler warnings and load manifest generated header
#undef EXTERN_C
#define EXTERN_C
#undef __declspec
#define __declspec(x)
#include "wevt_netdata_manifest.h"

static REGHANDLE regHandle;
#endif

// Function to construct EventID
static DWORD complete_event_id(DWORD facility, DWORD severity, DWORD event_code) {
    DWORD event_id = 0;

    // Set Severity
    event_id |= ((DWORD)(severity) << EVENT_ID_SEV_SHIFT) & EVENT_ID_SEV_MASK;

    // Set Customer Code Flag (C)
    event_id |= (0x0 << EVENT_ID_C_SHIFT) & EVENT_ID_C_MASK;

    // Set Reserved Bit (R) - typically 0
    event_id |= (0x0 << EVENT_ID_R_SHIFT) & EVENT_ID_R_MASK;

    // Set Facility
    event_id |= ((DWORD)(facility) << EVENT_ID_FACILITY_SHIFT) & EVENT_ID_FACILITY_MASK;

    // Set Code
    event_id |= ((DWORD)(event_code) << EVENT_ID_CODE_SHIFT) & EVENT_ID_CODE_MASK;

    return event_id;
}

DWORD construct_event_id(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, MESSAGE_ID messageID) {
    DWORD event_code = construct_event_code(source, priority, messageID);
    return complete_event_id(FACILITY_NETDATA, get_severity_from_priority(priority), event_code);
}

static bool check_event_id(ND_LOG_SOURCES source __maybe_unused, ND_LOG_FIELD_PRIORITY priority __maybe_unused, MESSAGE_ID messageID __maybe_unused, DWORD event_code __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    DWORD generated = construct_event_id(source, priority, messageID);
    if(generated != event_code) {

        // this is just used for a break point, to see the values in hex
        char current[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(current, generated);

        char wanted[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(wanted, event_code);

        const char *got = current;
        const char *good = wanted;
        internal_fatal(true, "EventIDs mismatch, expected %s, got %s", good, got);
    }
#endif

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// initialization

#if defined(HAVE_ETW)
static void etw_set_source_meta(struct nd_log_source *source, USHORT channelID, const EVENT_DESCRIPTOR *ed) {
    // It turns out that the keyword varies per only per channel!
    // so, to log with the right keyword, Task, Opcode we copy the ids from the header
    // the messages compiler (mc.exe) generated from the manifest.

    source->channelID = channelID;
    source->Opcode = ed->Opcode;
    source->Task = ed->Task;
    source->Keyword = ed->Keyword;
}

static bool etw_register_provider(void) {
    // Manifest-based providers can write immediately after registration. An enable
    // callback only reports trace-session changes and must not gate agent startup.
    if(EventRegister(&NETDATA_ETW_PROVIDER_GUID, NULL, NULL, &regHandle) != ERROR_SUCCESS)
        return false;

    etw_set_source_meta(&nd_log.sources[NDLS_DAEMON], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_COLLECTORS], CHANNEL_COLLECTORS, &ED_COLLECTORS_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_ACCESS], CHANNEL_ACCESS, &ED_ACCESS_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_HEALTH], CHANNEL_HEALTH, &ED_HEALTH_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_ACLK], CHANNEL_ACLK, &ED_ACLK_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_UNSET], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_DEBUG], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);

    return true;
}
#endif

static void etw_queue_init(void);

bool nd_log_init_windows(void) {
    if(nd_log.eventlog.initialized)
        return true;

    // validate we have the right keys
    if(
            !check_event_id(NDLS_COLLECTORS, NDLP_INFO, MSGID_MESSAGE_ONLY, MC_COLLECTORS_INFO_MESSAGE_ONLY) ||
            !check_event_id(NDLS_DAEMON, NDLP_ERR, MSGID_MESSAGE_ONLY, MC_DAEMON_ERR_MESSAGE_ONLY) ||
            !check_event_id(NDLS_ACCESS, NDLP_WARNING, MSGID_ACCESS_USER, MC_ACCESS_WARN_ACCESS_USER) ||
            !check_event_id(NDLS_HEALTH, NDLP_CRIT, MSGID_ALERT_TRANSITION, MC_HEALTH_CRIT_ALERT_TRANSITION) ||
            !check_event_id(NDLS_DEBUG, NDLP_ALERT, MSGID_ACCESS_FORWARDER_USER, MC_DEBUG_ALERT_ACCESS_FORWARDER_USER))
       return false;

#if defined(HAVE_ETW)
    if(nd_log.eventlog.etw && !etw_register_provider())
        return false;
#endif

    nd_log.eventlog.initialized = true;
    etw_queue_init();
    return true;
}

bool nd_log_init_etw(void) {
    nd_log.eventlog.etw = true;
    return nd_log_init_windows();
}

// --------------------------------------------------------------------------------------------------------------------
// we pass all our fields to the windows events logs
// numbered the same way we have them in memory.
//
// to avoid runtime memory allocations, we use a static allocations with ready to use buffers
// which are immediately available for logging.

#define SMALL_WIDE_BUFFERS_SIZE 256
#define MEDIUM_WIDE_BUFFERS_SIZE 2048
#define BIG_WIDE_BUFFERS_SIZE 16384
// Largest per-field wide-char buffer (incl. null terminator). Used by
// etw_wcslen_bounded() to satisfy SonarQube S5813 at ETW write sites.
#define NDF_FIELD_MAX_CHARS BIG_WIDE_BUFFERS_SIZE
static wchar_t small_wide_buffers[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
static wchar_t medium_wide_buffers[2][MEDIUM_WIDE_BUFFERS_SIZE];
static wchar_t big_wide_buffers[2][BIG_WIDE_BUFFERS_SIZE];

static struct {
    size_t size;
    wchar_t *buf;
} fields_buffers[_NDF_MAX] = { 0 };

__attribute__((constructor)) void wevents_initialize_buffers(void) {
    for(size_t i = 0; i < _NDF_MAX ;i++) {
        fields_buffers[i].buf = small_wide_buffers[i];
        fields_buffers[i].size = SMALL_WIDE_BUFFERS_SIZE;
    }

    fields_buffers[NDF_NIDL_INSTANCE].buf = medium_wide_buffers[0];
    fields_buffers[NDF_NIDL_INSTANCE].size = MEDIUM_WIDE_BUFFERS_SIZE;

    fields_buffers[NDF_REQUEST].buf = big_wide_buffers[0];
    fields_buffers[NDF_REQUEST].size = BIG_WIDE_BUFFERS_SIZE;
    fields_buffers[NDF_MESSAGE].buf = big_wide_buffers[1];
    fields_buffers[NDF_MESSAGE].size = BIG_WIDE_BUFFERS_SIZE;
}

// ----------------------------------------------------------------------------
// Async ETW writer — decouples producers from EventWrite.
//
// The global wchar buffers (small/medium/big) are shared state. The old design held a
// single spinlock across BOTH field generation AND EventWrite; if EventWrite blocked
// (for example, a temporarily blocked Event Log service) every logging thread spun
// indefinitely, blocking the main thread and the shutdown cleanup thread.
//
// New design:
//   - etw_queue.mutex replaces the spinlock and covers only field generation + enqueue.
//   - A single background thread (etw_async_writer) dequeues entries and calls EventWrite.
//   - If EventWrite blocks, only the background thread stalls; producers enqueue and return.
//   - When the queue is full the entry is dropped and the caller falls back to stderr.

#define ETW_QUEUE_DEPTH 8

struct etw_queue_entry {
    DWORD           eventID;
    USHORT          channelID;
    UCHAR           level;
    UCHAR           opcode;
    USHORT          task;
    ULONGLONG       keyword;
    // Snapshots of the three global wchar buffer pools (copied under etw_queue.mutex)
    wchar_t small[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
    wchar_t medium[2][MEDIUM_WIDE_BUFFERS_SIZE];
    wchar_t big[2][BIG_WIDE_BUFFERS_SIZE];
};

static struct {
    struct etw_queue_entry  slots[ETW_QUEUE_DEPTH];
    size_t                  head, tail, count;
    bool                    stopped;
    uint64_t                dropped;
    netdata_mutex_t         mutex;
    netdata_cond_t          not_empty;
    HANDLE                  drain_ack;  // auto-reset; consumer sets it on exit
    ND_THREAD              *thread;
    bool                    initialized;
} etw_queue;

// Mirror the fields_buffers[] layout: return the right buffer from a queue entry.
static inline const wchar_t *etw_entry_field(const struct etw_queue_entry *e, size_t i) {
    if(i == NDF_NIDL_INSTANCE) return e->medium[0];
    if(i == NDF_REQUEST)       return e->big[0];
    if(i == NDF_MESSAGE)       return e->big[1];
    return e->small[i];
}

static void etw_entry_process(struct etw_queue_entry *e) {
    EVENT_DATA_DESCRIPTOR desc[_NDF_MAX - 1];
    for(size_t i = 1; i < _NDF_MAX; i++) {
        const wchar_t *buf = etw_entry_field(e, i);
        // Field buffers are fixed-size (small/medium/big) and always null-terminated
        // when populated; the bounded helper is only here to satisfy SonarQube S5813
        // and to return 0 on a malformed buffer rather than overread it.
        EventDataDescCreate(&desc[i - 1], buf,
                            (ULONG)((etw_wcslen_bounded(buf, NDF_FIELD_MAX_CHARS) + 1) * sizeof(WCHAR)));
    }
    EVENT_DESCRIPTOR ed = {
        .Id      = e->eventID & EVENT_ID_CODE_MASK,
        .Version = 0,
        .Channel = (UCHAR)e->channelID,
        .Level   = e->level,
        .Opcode  = e->opcode,
        .Task    = e->task,
        .Keyword = e->keyword,
    };
    (void)EventWrite(regHandle, &ed, _NDF_MAX - 1, desc);
}

static void etw_async_writer(void *arg __maybe_unused) {

    for(;;) {
        netdata_mutex_lock(&etw_queue.mutex);

        while(etw_queue.count == 0 && !etw_queue.stopped)
            netdata_cond_wait(&etw_queue.not_empty, &etw_queue.mutex);

        if(etw_queue.count == 0) {
            // stopped and queue drained
            netdata_mutex_unlock(&etw_queue.mutex);
            break;
        }

        // Snapshot head without advancing. count stays > 0, so producers cannot
        // wrap tail back to this slot while we process it outside the mutex.
        size_t idx = etw_queue.head;
        netdata_mutex_unlock(&etw_queue.mutex);

        // EventWrite may block here — no lock held, producers unaffected.
        etw_entry_process(&etw_queue.slots[idx]);

        // Release the slot
        netdata_mutex_lock(&etw_queue.mutex);
        etw_queue.head = (etw_queue.head + 1) % ETW_QUEUE_DEPTH;
        etw_queue.count--;
        netdata_mutex_unlock(&etw_queue.mutex);
    }

    if(etw_queue.drain_ack)
        SetEvent(etw_queue.drain_ack);
}

static void etw_queue_init(void) {
    netdata_mutex_init(&etw_queue.mutex);
    netdata_cond_init(&etw_queue.not_empty);
    etw_queue.drain_ack = CreateEvent(NULL, FALSE, FALSE, NULL);
    etw_queue.thread = nd_thread_create("ETW-ASYNC", NETDATA_THREAD_OPTION_DEFAULT,
                                        etw_async_writer, NULL);
    etw_queue.initialized = true;
}

void nd_log_stop_windows_async(void) {
    if(!etw_queue.initialized)
        return;

    netdata_mutex_lock(&etw_queue.mutex);
    etw_queue.stopped = true;
    netdata_cond_signal(&etw_queue.not_empty);
    netdata_mutex_unlock(&etw_queue.mutex);

    // Wait up to 2 s for the async writer to drain remaining entries.
    // If ETW is still blocked we time out and let the process terminate normally.
    if(etw_queue.drain_ack)
        WaitForSingleObject(etw_queue.drain_ack, 2000);

}

// --------------------------------------------------------------------------------------------------------------------

#define is_field_set(fields, fields_max, field) ((field) < (fields_max) && (fields)[field].entry.set)

static const char *get_field_value_unsafe(struct log_field *fields, ND_LOG_FIELD_ID i, size_t fields_max, BUFFER **tmp) {
    if(!is_field_set(fields, fields_max, i) || !fields[i].eventlog)
        return "";

    static char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

    const char *s = NULL;
    if (fields[i].logfmt_annotator)
        s = fields[i].logfmt_annotator(&fields[i]);

    else
        switch (fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                print_uint64(number_str, fields[i].entry.u64);
                s = number_str;
                break;
            case NDFT_I64:
                print_int64(number_str, fields[i].entry.i64);
                s = number_str;
                break;
            case NDFT_DBL:
                print_netdata_double(number_str, fields[i].entry.dbl);
                s = number_str;
                break;
            case NDFT_UUID:
                if (!uuid_is_null(*fields[i].entry.uuid)) {
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, number_str);
                    s = number_str;
                }
                break;
            case NDFT_CALLBACK:
                if (!*tmp)
                    *tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(*tmp);

                if (fields[i].entry.cb.formatter(*tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(*tmp);
                else
                    s = NULL;
                break;

            default:
                s = "UNHANDLED";
                break;
        }

    if(!s || !*s) return "";
    return s;
}
static void etw_replace_percent_with_unicode(wchar_t *s, size_t size) {
    size_t original_len = wcslen(s);

    // Traverse the string, replacing '%' with the Unicode fullwidth percent sign
    for (size_t i = 0; i < original_len && i < size - 1; i++) {
        if (s[i] == L'%' && iswdigit(s[i + 1])) {
            // s[i] = 0xFF05;  // Replace '%' with fullwidth percent sign '％'
            // s[i] = 0x29BC; // ⦼
            s[i] = 0x2105; // ℅
        }
    }

    // Ensure null termination if needed
    s[size - 1] = L'\0';
}

static void wevt_generate_all_fields_unsafe(struct log_field *fields, size_t fields_max, BUFFER **tmp) {
    for (size_t i = 0; i < fields_max; i++) {
        fields_buffers[i].buf[0] = L'\0';

        if (!fields[i].entry.set || !fields[i].eventlog)
            continue;

        const char *s = get_field_value_unsafe(fields, i, fields_max, tmp);
        if (s && *s) {
            utf8_to_utf16(fields_buffers[i].buf, (int) fields_buffers[i].size, s, -1);

            // ETW recursively expands percent-prefixed fields.
            etw_replace_percent_with_unicode(fields_buffers[i].buf, fields_buffers[i].size);
        }
    }
}

static bool has_user_role_permissions(struct log_field *fields, size_t fields_max, BUFFER **tmp) {
    const char *t;

    t = get_field_value_unsafe(fields, NDF_USER_NAME, fields_max, tmp);
    if (*t) return true;

    t = get_field_value_unsafe(fields, NDF_USER_ROLE, fields_max, tmp);
    if (*t && strcmp(t, "none") != 0) return true;

    t = get_field_value_unsafe(fields, NDF_USER_ACCESS, fields_max, tmp);
    if (*t && strcmp(t, "0x0") != 0) return true;

    return false;
}

static bool nd_logger_windows(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    if (!nd_log.eventlog.initialized || !etw_queue.initialized)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if (fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    CLEAN_BUFFER *tmp = NULL;

    // etw_queue.mutex replaces the old spinlock:
    //   - protects field generation into the shared global wchar buffers
    //   - protects queue head/tail/count while we claim a slot
    //   - does NOT cover EventWrite (that runs in the async writer thread)
    netdata_mutex_lock(&etw_queue.mutex);

    wevt_generate_all_fields_unsafe(fields, fields_max, &tmp);

    MESSAGE_ID messageID;
    switch (source->source) {
        default:
        case NDLS_DEBUG:
        case NDLS_DAEMON:
        case NDLS_COLLECTORS:
            messageID = MSGID_MESSAGE_ONLY;
            break;

        case NDLS_HEALTH:
            messageID = MSGID_ALERT_TRANSITION;
            break;

        case NDLS_ACCESS:
            if (is_field_set(fields, fields_max, NDF_MESSAGE)) {
                messageID = MSGID_ACCESS_MESSAGE;

                if (has_user_role_permissions(fields, fields_max, &tmp))
                    messageID = MSGID_ACCESS_MESSAGE_USER;
                else if (*get_field_value_unsafe(fields, NDF_REQUEST, fields_max, &tmp))
                    messageID = MSGID_ACCESS_MESSAGE_REQUEST;
            } else if (is_field_set(fields, fields_max, NDF_RESPONSE_CODE)) {
                messageID = MSGID_ACCESS;

                if (*get_field_value_unsafe(fields, NDF_SRC_FORWARDED_FOR, fields_max, &tmp))
                    messageID = MSGID_ACCESS_FORWARDER;

                if (has_user_role_permissions(fields, fields_max, &tmp)) {
                    if (messageID == MSGID_ACCESS)
                        messageID = MSGID_ACCESS_USER;
                    else
                        messageID = MSGID_ACCESS_FORWARDER_USER;
                }
            } else
                messageID = MSGID_REQUEST_ONLY;
            break;

        case NDLS_ACLK:
            messageID = MSGID_MESSAGE_ONLY;
            break;
    }

    if (messageID == MSGID_MESSAGE_ONLY && (
            *get_field_value_unsafe(fields, NDF_ERRNO, fields_max, &tmp) ||
            *get_field_value_unsafe(fields, NDF_WINERROR, fields_max, &tmp))) {
        messageID = MSGID_MESSAGE_ERRNO;
    }

    DWORD eventID = construct_event_id(source->source, priority, messageID);

    bool enqueued = false;
    if(etw_queue.count < ETW_QUEUE_DEPTH) {
        struct etw_queue_entry *e = &etw_queue.slots[etw_queue.tail];
        etw_queue.tail = (etw_queue.tail + 1) % ETW_QUEUE_DEPTH;
        etw_queue.count++;
        enqueued = true;

        e->eventID    = eventID;
        e->channelID  = source->channelID;
        e->level      = get_level_from_priority(priority);
        e->opcode     = source->Opcode;
        e->task       = source->Task;
        e->keyword    = source->Keyword;
        // Snapshot the global buffers into the queue slot before releasing the mutex.
        memcpy(e->small,  small_wide_buffers,  sizeof(small_wide_buffers));
        memcpy(e->medium, medium_wide_buffers, sizeof(medium_wide_buffers));
        memcpy(e->big,    big_wide_buffers,    sizeof(big_wide_buffers));

        netdata_cond_signal(&etw_queue.not_empty);
    } else {
        etw_queue.dropped++;
    }

    netdata_mutex_unlock(&etw_queue.mutex);

    return enqueued;
}

#if defined(HAVE_ETW)
bool nd_logger_etw(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    return nd_logger_windows(source, fields, fields_max);
}
#endif

#endif
