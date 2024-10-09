// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS)
#include <windows.h>
#include <winevt.h>
#include <evntprov.h>
#include <wchar.h>
#include <objbase.h>
#include <guiddef.h>


// --------------------------------------------------------------------------------------------------------------------
// construct an event id (must be aligned with .mc file)

#include "wevt_netdata.h"
#include "nd_log-to-windows-common.h"

// Function to construct EventID
static DWORD complete_event_id(DWORD facility, DWORD severity, DWORD event_code) {
    DWORD event_id = 0;

    // Set Severity
    event_id |= ((DWORD)(severity) << EVENT_ID_SEV_SHIFT) & EVENT_ID_SEV_MASK;

    // Set Customer Code Flag (C)
    event_id |= (0x1 << EVENT_ID_C_SHIFT) & EVENT_ID_C_MASK;

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

// --------------------------------------------------------------------------------------------------------------------
// initialization

// Define source names with "Netdata" prefix
static const wchar_t *source_names[_NDLS_MAX] = {
        NETDATA_PROVIDER_WNAME L"_Unset",        // NDLS_UNSET (not used)
        NETDATA_PROVIDER_WNAME L"_Access",       // NDLS_ACCESS
        NETDATA_PROVIDER_WNAME L"_ACLK",         // NDLS_ACLK
        NETDATA_PROVIDER_WNAME L"_Collectors",   // NDLS_COLLECTORS
        NETDATA_PROVIDER_WNAME L"_Daemon",       // NDLS_DAEMON
        NETDATA_PROVIDER_WNAME L"_Health",       // NDLS_HEALTH
        NETDATA_PROVIDER_WNAME L"_Debug",        // NDLS_DEBUG
};

static bool add_to_registry(const wchar_t *logName, const wchar_t *sourceName) {
    // Build the registry path: SYSTEM\CurrentControlSet\Services\EventLog\<LogName>\<SourceName>
    wchar_t key[MAX_PATH];
    swprintf(key, MAX_PATH, L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\%ls\\%ls", logName, sourceName);

    HKEY hRegKey;
    DWORD disposition;
    LONG result = RegCreateKeyExW(HKEY_LOCAL_MACHINE, key,
                                  0, NULL, REG_OPTION_NON_VOLATILE, KEY_SET_VALUE, NULL, &hRegKey, &disposition);

    if (result != ERROR_SUCCESS)
        return false; // Could not create the registry key

    wchar_t *modulePath = L"%SystemRoot%\\System32\\wevt_netdata.dll"; // Update as needed
    RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ,
                   (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));
    DWORD types_supported = EVENTLOG_SUCCESS | EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
    RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));

    RegCloseKey(hRegKey);
    return true;
}

static bool check_event_id(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, MESSAGE_ID messageID, DWORD event_code) {
    DWORD generated = construct_event_id(source, priority, messageID);
    if(generated != event_code) {
        
#ifdef NETDATA_INTERNAL_CHECKS
        // this is just used for a break point, to see the values in hex
        char current[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(current, generated);

        char wanted[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(wanted, event_code);

        const char *got = current;
        const char *good = wanted;
        (void)got; (void)good;
#endif

        return false;
    }

    return true;
}

bool nd_log_wevents_init(void) {
    if(nd_log.wevents.initialized)
        return true;

    // validate we have the right keys
    internal_fatal(
            !check_event_id(NDLS_COLLECTORS, NDLP_INFO, MSGID_MESSAGE_ONLY, COLLECTORS_INFO_MESSAGE_ONLY) ||
            !check_event_id(NDLS_DAEMON, NDLP_ERR, MSGID_MESSAGE_ONLY, DAEMON_ERR_MESSAGE_ONLY) ||
            !check_event_id(NDLS_ACCESS, NDLP_WARNING, MSGID_ACCESS_USER, ACCESS_WARN_ACCESS_USER) ||
            !check_event_id(NDLS_HEALTH, NDLP_CRIT, MSGID_ALERT_TRANSITION, HEALTH_CRIT_ALERT_TRANSITION) ||
            !check_event_id(NDLS_DEBUG, NDLP_ALERT, MSGID_ACCESS_FORWARDER_USER, DEBUG_ALERT_ACCESS_FORWARDER_USER),
       "The encoding of the event ids is wrong!");

    const wchar_t *logName = NETDATA_PROVIDER_WNAME; // Custom log name "Netdata"

    // Loop through each source and add it to the registry
    for(size_t i = 0; i < _NDLS_MAX; i++) {
        nd_log.sources[i].source = i;

        // Skip NDLS_UNSET
        if(i == NDLS_UNSET) continue;

        if(!add_to_registry(logName, source_names[i])) {
            // Handle error (optional)
            return false;
        }

        // Register the event source with the prefixed source name
        nd_log.sources[i].hEventLog = RegisterEventSourceW(NULL, source_names[i]);
        if (!nd_log.sources[i].hEventLog) {
            // Handle error (optional)
            return false;
        }
    }

    nd_log.wevents.initialized = true;
    return true;
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

static LPCWSTR messages[_NDF_MAX - 1];

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
        messages[i - 1] = fields_buffers[i].buf;
}

// --------------------------------------------------------------------------------------------------------------------

#define is_field_set(fields, fields_max, field) ((field) < (fields_max) && (fields)[field].entry.set)

static const char *get_field_value(struct log_field *fields, ND_LOG_FIELD_ID i, size_t fields_max, BUFFER **tmp) {
    if(!is_field_set(fields, fields_max, i) || !fields[i].wevents)
        return "";

    static char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

    const char *s = NULL;
    if (fields[i].annotator)
        s = fields[i].annotator(&fields[i]);

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

static void wevt_generate_all_fields_unsafe(struct log_field *fields, size_t fields_max) {
    CLEAN_BUFFER *tmp = NULL;
    char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

    for (size_t i = 0; i < fields_max; i++) {
        fields_buffers[i].buf[0] = L'\0';

        if (!fields[i].entry.set || !fields[i].wevents)
            continue;

        const char *s = NULL;
        if (fields[i].annotator)
            s = fields[i].annotator(&fields[i]);

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
                    if (!tmp)
                        tmp = buffer_create(1024, NULL);
                    else
                        buffer_flush(tmp);

                    if (fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                        s = buffer_tostring(tmp);
                    else
                        s = NULL;
                    break;

                default:
                    s = "UNHANDLED";
                    break;
            }

        if (s && *s)
            utf8_to_utf16(fields_buffers[i].buf, (int) fields_buffers[i].size, s, -1);
    }
}

static bool has_user_role_permissions(struct log_field *fields, size_t fields_max, BUFFER **tmp) {
    if(is_field_set(fields, fields_max, NDF_USER_NAME)) return true;
    if(is_field_set(fields, fields_max, NDF_USER_ACCESS)) return true;

    if(is_field_set(fields, fields_max, NDF_USER_ACCESS)) {
        const char *t = get_field_value(fields, NDF_USER_ROLE, fields_max, tmp);
        if (*t && strcmp(t, "none") != 0) return true;
    }

    return false;
}

bool nd_logger_wevents(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    if(!nd_log.wevents.initialized || !source->hEventLog)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if(fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD wType = get_event_type_from_priority(priority);
    MESSAGE_ID messageID = MSGID_MESSAGE_ONLY;

    CLEAN_BUFFER *tmp = NULL;

    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);
    wevt_generate_all_fields_unsafe(fields, fields_max);

    switch(source->source) {
        default:
        case NDLS_DEBUG:
            messageID = MSGID_MESSAGE_ONLY;
            break;

        case NDLS_DAEMON:
            messageID = MSGID_MESSAGE_ONLY;
            break;

        case NDLS_COLLECTORS:
            messageID = MSGID_MESSAGE_ONLY;
            break;

        case NDLS_HEALTH:
            messageID = MSGID_ALERT_TRANSITION;
            break;

        case NDLS_ACCESS:
            if(is_field_set(fields, fields_max, NDF_MESSAGE)) {
                messageID = MSGID_ACCESS_MESSAGE;

                if(has_user_role_permissions(fields, fields_max, &tmp))
                    messageID = MSGID_ACCESS_MESSAGE_USER;
            }
            else if(is_field_set(fields, fields_max, NDF_RESPONSE_CODE)) {
                messageID = MSGID_ACCESS;

                if(*get_field_value(fields, NDF_SRC_FORWARDED_FOR, fields_max, &tmp))
                    messageID = MSGID_ACCESS_FORWARDER;

                if(has_user_role_permissions(fields, fields_max, &tmp)) {
                    if(messageID == MSGID_ACCESS)
                        messageID = MSGID_ACCESS_USER;
                    else
                        messageID = MSGID_ACCESS_FORWARDER_USER;
                }
            }
            else
                messageID = MSGID_REQUEST_ONLY;
            break;

        case NDLS_ACLK:
            messageID = MSGID_MESSAGE_ONLY;
            break;
    }

    if(messageID == MSGID_MESSAGE_ONLY && (
        *get_field_value(fields, NDF_ERRNO, fields_max, &tmp) ||
        *get_field_value(fields, NDF_WINERROR, fields_max, &tmp))) {
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

    BOOL rc = ReportEventW(source->hEventLog, wType, 0, eventID, NULL, _NDF_MAX - 1, 0, messages, NULL);

    spinlock_unlock(&spinlock);

    return rc == TRUE;
}

#endif
