// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS)

#include <windows.h>
#include <winevt.h>
#include <evntprov.h>
#include <wchar.h>
#include <objbase.h>
#include <guiddef.h>

#define NETDATA_PROVIDER_NAME L"Netdata"

// Define source names with "Netdata" prefix
static const wchar_t *source_names[_NDLS_MAX] = {
        NETDATA_PROVIDER_NAME L"_Unset",        // NDLS_UNSET (not used)
        NETDATA_PROVIDER_NAME L"_Access",       // NDLS_ACCESS
        NETDATA_PROVIDER_NAME L"_ACLK",         // NDLS_ACLK
        NETDATA_PROVIDER_NAME L"_Collectors",   // NDLS_COLLECTORS
        NETDATA_PROVIDER_NAME L"_Daemon",       // NDLS_DAEMON
        NETDATA_PROVIDER_NAME L"_Health",       // NDLS_HEALTH
        NETDATA_PROVIDER_NAME L"_Debug",        // NDLS_DEBUG
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

    wchar_t *modulePath = L"%SystemRoot%\\System32\\EventCreate.exe"; // Update as needed
    RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ,
                   (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));
    DWORD types_supported = EVENTLOG_SUCCESS | EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
    RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));

    RegCloseKey(hRegKey);
    return true;
}

bool nd_log_wevents_init(void) {
    if(nd_log.wevents.initialized)
        return true;

    const wchar_t *logName = NETDATA_PROVIDER_NAME; // Custom log name "Netdata"

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

static WORD get_event_type(ND_LOG_FIELD_PRIORITY priority) {
    switch (priority) {
        case NDLP_EMERG:
        case NDLP_ALERT:
        case NDLP_CRIT:
        case NDLP_ERR:
            return EVENTLOG_ERROR_TYPE;

        case NDLP_WARNING:
            return EVENTLOG_WARNING_TYPE;

        case NDLP_NOTICE:
        case NDLP_INFO:
        case NDLP_DEBUG:
        default:
            return EVENTLOG_INFORMATION_TYPE;
    }
}

#define is_field_set(fields, fields_max, field) ((field) < (fields_max) && (fields)[field].entry.set)

static const char *get_field_value(struct log_field *fields, ND_LOG_FIELD_ID i, size_t fields_max, BUFFER **tmp) {
    if(!is_field_set(fields, fields_max, i) || !fields[i].wevents)
        return "";

    static __thread char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

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

bool nd_logger_wevents(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    if(!nd_log.wevents.initialized || !source->hEventLog)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if(fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD eventType = get_event_type(priority);
    DWORD eventID = 1;

    CLEAN_BUFFER *wb = buffer_create(4096, NULL);
    CLEAN_BUFFER *tmp = NULL;
    const char *t;

    buffer_strcat(wb, get_field_value(fields, NDF_SYSLOG_IDENTIFIER, fields_max, &tmp));
    buffer_strcat(wb, "(");
    buffer_strcat(wb, get_field_value(fields, NDF_TID, fields_max, &tmp));
    if(*(t = get_field_value(fields, NDF_THREAD_TAG, fields_max, &tmp))) {
        buffer_strcat(wb, ", ");
        buffer_strcat(wb, t);
    }
    buffer_strcat(wb, "): ");

    switch(source->source) {
        default:
        case NDLS_DEBUG:
            eventID = 1;
            buffer_strcat(wb, get_field_value(fields, NDF_MESSAGE, fields_max, &tmp));
            break;

        case NDLS_DAEMON:
            eventID = 101;
            buffer_strcat(wb, get_field_value(fields, NDF_MESSAGE, fields_max, &tmp));
            break;

        case NDLS_COLLECTORS:
            eventID = 201;
            buffer_strcat(wb, get_field_value(fields, NDF_MESSAGE, fields_max, &tmp));
            break;

        case NDLS_HEALTH:
            eventID = 301;
            buffer_strcat(wb, "Alert '");
            buffer_strcat(wb, get_field_value(fields, NDF_ALERT_NAME, fields_max, &tmp));
            buffer_strcat(wb, "' of instance '");
            buffer_strcat(wb, get_field_value(fields, NDF_NIDL_INSTANCE, fields_max, &tmp));
            buffer_strcat(wb, "'\r\n  Node: '");
            buffer_strcat(wb, get_field_value(fields, NDF_NIDL_NODE, fields_max, &tmp));
            buffer_strcat(wb, "'\r\n  Transitioned from ");
            buffer_strcat(wb, get_field_value(fields, NDF_ALERT_STATUS_OLD, fields_max, &tmp));
            buffer_strcat(wb, " to ");
            buffer_strcat(wb, get_field_value(fields, NDF_ALERT_STATUS, fields_max, &tmp));
            break;

        case NDLS_ACCESS:
            if(is_field_set(fields, fields_max, NDF_MESSAGE)) {
                eventID = 501;
                buffer_strcat(wb, get_field_value(fields, NDF_MESSAGE, fields_max, &tmp));
            }
            else if(is_field_set(fields, fields_max, NDF_RESPONSE_CODE)) {
                eventID = 502;
                buffer_strcat(wb, "Transaction ");
                buffer_strcat(wb, get_field_value(fields, NDF_TRANSACTION_ID, fields_max, &tmp));
                buffer_strcat(wb, ", Response Code: ");
                buffer_strcat(wb, get_field_value(fields, NDF_RESPONSE_CODE, fields_max, &tmp));
                buffer_strcat(wb, "\r\n\r\n  Request: ");
                buffer_strcat(wb, get_field_value(fields, NDF_REQUEST_METHOD, fields_max, &tmp));
                buffer_strcat(wb, " ");
                buffer_strcat(wb, get_field_value(fields, NDF_REQUEST, fields_max, &tmp));
                buffer_strcat(wb, "\r\n\r\n  From: ");
                buffer_strcat(wb, get_field_value(fields, NDF_SRC_IP, fields_max, &tmp));
                if(*(t = get_field_value(fields, NDF_SRC_FORWARDED_FOR, fields_max, &tmp))) {
                    eventID += 10;
                    buffer_strcat(wb, ", for: ");
                    buffer_strcat(wb, t);
                }
                if(is_field_set(fields, fields_max, NDF_USER_NAME) || is_field_set(fields, fields_max, NDF_USER_ROLE)) {
                    eventID += 20;
                    buffer_strcat(wb, "\r\n  User: ");
                    buffer_strcat(wb, get_field_value(fields, NDF_USER_NAME, fields_max, &tmp));
                    buffer_strcat(wb, ", role: ");
                    buffer_strcat(wb, get_field_value(fields, NDF_USER_ROLE, fields_max, &tmp));
                    buffer_strcat(wb, ", permissions: ");
                    buffer_strcat(wb, get_field_value(fields, NDF_USER_ACCESS, fields_max, &tmp));
                }
            }
            else {
                eventID = 503;
                buffer_strcat(wb, get_field_value(fields, NDF_REQUEST, fields_max, &tmp));
            }
            break;

        case NDLS_ACLK:
            eventID = 901;
            buffer_strcat(wb, get_field_value(fields, NDF_MESSAGE, fields_max, &tmp));
            break;
    }

    buffer_strcat(wb, "\r\n");

    if(*(t = get_field_value(fields, NDF_ERRNO, fields_max, &tmp))) {
        buffer_strcat(wb, "\r\n   unix errno: ");
        buffer_strcat(wb, t);
    }

    if(*(t = get_field_value(fields, NDF_WINERROR, fields_max, &tmp))) {
        buffer_strcat(wb, "\r\n   Windows Error: ");
        buffer_strcat(wb, t);
    }

    static __thread wchar_t msg[4096];
//    static __thread wchar_t all[4096];
    utf8_to_utf16(msg, _countof(msg), buffer_tostring(wb), buffer_strlen(wb));

//    buffer_flush(wb);
//    for(size_t i = 1; i < fields_max ; i++) {
//        if(!is_field_set(fields, fields_max, i) || !fields[i].wevents)
//            continue;
//
//        t = get_field_value(fields, i, fields_max, &tmp);
//        if(*t) buffer_sprintf(wb, "\r\n  %s: %s", fields[i].wevents, t);
//    }
//    utf8_to_utf16(all, _countof(all), buffer_tostring(wb), buffer_strlen(wb));
//    LPCWSTR messages[2] = { msg, all };

    LPCWSTR messages[1] = { msg };
    BOOL rc = ReportEventW(source->hEventLog, eventType, 0, eventID, NULL, 1, 0, messages, NULL);
    return rc == TRUE;
}

#endif
