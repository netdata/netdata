// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS) && (defined(HAVE_ETW) || defined(HAVE_WEL))

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

// Define provider names per source (only when not using ETW)
static const wchar_t *wel_provider_per_source[_NDLS_MAX] = {
        [NDLS_UNSET]        = NULL,                             // not used, linked to NDLS_DAEMON
        [NDLS_ACCESS]       = NETDATA_WEL_PROVIDER_ACCESS_W,    //
        [NDLS_ACLK]         = NETDATA_WEL_PROVIDER_ACLK_W,      //
        [NDLS_COLLECTORS]   = NETDATA_WEL_PROVIDER_COLLECTORS_W,//
        [NDLS_DAEMON]       = NETDATA_WEL_PROVIDER_DAEMON_W,    //
        [NDLS_HEALTH]       = NETDATA_WEL_PROVIDER_HEALTH_W,    //
        [NDLS_DEBUG]        = NULL,                             // used, linked to NDLS_DAEMON
};

bool wel_replace_program_with_wevt_netdata_dll(wchar_t *str, size_t size) {
    const wchar_t *replacement = L"\\wevt_netdata.dll";

    // Find the last occurrence of '\\' to isolate the filename
    wchar_t *lastBackslash = wcsrchr(str, L'\\');

    if (lastBackslash != NULL) {
        // Calculate new length after replacement
        size_t newLen = (lastBackslash - str) + wcslen(replacement);

        // Ensure new length does not exceed buffer size
        if (newLen >= size)
            return false; // Not enough space in the buffer

        // Terminate the string at the last backslash
        *lastBackslash = L'\0';

        // Append the replacement filename
        wcsncat(str, replacement, size - wcslen(str) - 1);

        // Check if the new file exists
        if (GetFileAttributesW(str) != INVALID_FILE_ATTRIBUTES)
            return true; // The file exists
        else
            return false; // The file does not exist
    }

    return false; // No backslash found (likely invalid input)
}

static bool wel_add_to_registry(const wchar_t *channel, const wchar_t *provider, DWORD defaultMaxSize) {
    // Build the registry path: SYSTEM\CurrentControlSet\Services\EventLog\<LogName>\<SourceName>
    wchar_t key[MAX_PATH];
    if(!provider)
        swprintf(key, MAX_PATH, L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\%ls", channel);
    else
        swprintf(key, MAX_PATH, L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\%ls\\%ls", channel, provider);

    HKEY hRegKey;
    DWORD disposition;
    LONG result = RegCreateKeyExW(HKEY_LOCAL_MACHINE, key,
                                  0, NULL, REG_OPTION_NON_VOLATILE, KEY_SET_VALUE, NULL, &hRegKey, &disposition);

    if (result != ERROR_SUCCESS)
        return false; // Could not create the registry key

    // Check if MaxSize is already set
    DWORD maxSize = 0;
    DWORD size = sizeof(maxSize);
    if (RegQueryValueExW(hRegKey, L"MaxSize", NULL, NULL, (LPBYTE)&maxSize, &size) != ERROR_SUCCESS) {
        // MaxSize is not set, set it to the default value
        RegSetValueExW(hRegKey, L"MaxSize", 0, REG_DWORD, (const BYTE*)&defaultMaxSize, sizeof(defaultMaxSize));
    }

    wchar_t modulePath[MAX_PATH];
    if (GetModuleFileNameW(NULL, modulePath, MAX_PATH) == 0) {
        RegCloseKey(hRegKey);
        return false;
    }

    if(wel_replace_program_with_wevt_netdata_dll(modulePath, _countof(modulePath))) {
        RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ,
                       (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));

        DWORD types_supported = EVENTLOG_SUCCESS | EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
        RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));
    }

    RegCloseKey(hRegKey);
    return true;
}

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
    // Register the ETW provider
    if (EventRegister(&NETDATA_ETW_PROVIDER_GUID, NULL, NULL, &regHandle) != ERROR_SUCCESS)
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

//    if(!nd_log.eventlog.etw && !wel_add_to_registry(NETDATA_WEL_CHANNEL_NAME_W, NULL, 50 * 1024 * 1024))
//        return false;

    // Loop through each source and add it to the registry
    for(size_t i = 0; i < _NDLS_MAX; i++) {
        nd_log.sources[i].source = i;

        const wchar_t *sub_channel = wel_provider_per_source[i];

        if(!sub_channel)
            // we will map these to NDLS_DAEMON
            continue;

        DWORD defaultMaxSize = 0;
        switch (i) {
            case NDLS_ACLK:
                defaultMaxSize = 5 * 1024 * 1024;
                break;

            case NDLS_HEALTH:
                defaultMaxSize = 35 * 1024 * 1024;
                break;

            default:
            case NDLS_ACCESS:
            case NDLS_COLLECTORS:
            case NDLS_DAEMON:
                defaultMaxSize = 20 * 1024 * 1024;
                break;
        }

        if(!nd_log.eventlog.etw) {
            if(!wel_add_to_registry(NETDATA_WEL_CHANNEL_NAME_W, sub_channel, defaultMaxSize))
                return false;

            // when not using a manifest, each source is a provider
            nd_log.sources[i].hEventLog = RegisterEventSourceW(NULL, sub_channel);
            if (!nd_log.sources[i].hEventLog)
                return false;
        }
    }

    if(!nd_log.eventlog.etw) {
        // Map the unset ones to NDLS_DAEMON
        for (size_t i = 0; i < _NDLS_MAX; i++) {
            if (!nd_log.sources[i].hEventLog)
                nd_log.sources[i].hEventLog = nd_log.sources[NDLS_DAEMON].hEventLog;
        }
    }

    nd_log.eventlog.initialized = true;
    return true;
}

bool nd_log_init_etw(void) {
    nd_log.eventlog.etw = true;
    return nd_log_init_windows();
}

bool nd_log_init_wel(void) {
    nd_log.eventlog.etw = false;
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
static wchar_t small_wide_buffers[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
static wchar_t medium_wide_buffers[2][MEDIUM_WIDE_BUFFERS_SIZE];
static wchar_t big_wide_buffers[2][BIG_WIDE_BUFFERS_SIZE];

static struct {
    size_t size;
    wchar_t *buf;
} fields_buffers[_NDF_MAX] = { 0 };

#if defined(HAVE_ETW)
static EVENT_DATA_DESCRIPTOR etw_eventData[_NDF_MAX - 1];
#endif

static LPCWSTR wel_messages[_NDF_MAX - 1];

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

    for(size_t i = 1; i < _NDF_MAX ;i++)
        wel_messages[i - 1] = fields_buffers[i].buf;
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
                    uuid_unparse_lower(*fields[i].entry.uuid, number_str);
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

            if(nd_log.eventlog.etw)
                // UNBELIEVABLE! they do recursive parameter expansion in ETW...
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
    if (!nd_log.eventlog.initialized)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if (fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD wType = get_event_type_from_priority(priority);
    (void) wType;

    CLEAN_BUFFER *tmp = NULL;

    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);
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

    // wType
    //
    // without a manifest => this determines the Level of the event
    // with a manifest    => Level from the manifest is used (wType ignored)
    //                       [however it is good to have, in case the manifest is not accessible somehow]
    //

    // wCategory
    //
    // without a manifest => numeric Task values appear
    // with a manifest    => Task from the manifest is used (wCategory ignored)

    BOOL rc;
#if defined(HAVE_ETW)
    if (nd_log.eventlog.etw) {
        // metadata based logging - ETW

        for (size_t i = 1; i < _NDF_MAX; i++)
            EventDataDescCreate(&etw_eventData[i - 1], fields_buffers[i].buf,
                                (wcslen(fields_buffers[i].buf) + 1) * sizeof(WCHAR));

        EVENT_DESCRIPTOR EventDesc = {
                .Id = eventID & EVENT_ID_CODE_MASK, // ETW needs the raw event id
                .Version = 0,
                .Channel = source->channelID,
                .Level = get_level_from_priority(priority),
                .Opcode = source->Opcode,
                .Task = source->Task,
                .Keyword = source->Keyword,
        };

        rc = ERROR_SUCCESS == EventWrite(regHandle, &EventDesc, _NDF_MAX - 1, etw_eventData);

    }
    else
#endif
    {
        // eventID based logging - WEL
        rc = ReportEventW(source->hEventLog, wType, 0, eventID, NULL, _NDF_MAX - 1, 0, wel_messages, NULL);
    }

    spinlock_unlock(&spinlock);

    return rc == TRUE;
}

#if defined(HAVE_ETW)
bool nd_logger_etw(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    return nd_logger_windows(source, fields, fields_max);
}
#endif

#if defined(HAVE_WEL)
bool nd_logger_wel(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    return nd_logger_windows(source, fields, fields_max);
}
#endif

#endif
