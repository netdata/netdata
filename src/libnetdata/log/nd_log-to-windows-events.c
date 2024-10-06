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
/*
 * #include "nd_wevents.h"
 *
 * Until we put it in cmake, here are the values:
 */

#define NETDATA_DAEMON_CATEGORY          ((WORD)0x00000020L)
#define NETDATA_COLLECTOR_CATEGORY       ((WORD)0x00000021L)
#define NETDATA_ACCESS_CATEGORY          ((WORD)0x00000022L)
#define NETDATA_HEALTH_CATEGORY          ((WORD)0x00000023L)
#define NETDATA_ACLK_CATEGORY            ((WORD)0x00000024L)

#define ND_EVENT_DAEMON_INFO             ((WORD)0x40001000L)
#define ND_EVENT_DAEMON_WARNING          ((WORD)0x80001001L)
#define ND_EVENT_DAEMON_ERROR            ((WORD)0xC0001002L)
#define ND_EVENT_COLLECTOR_INFO          ((WORD)0x40002000L)
#define ND_EVENT_COLLECTOR_WARNING       ((WORD)0x80002001L)
#define ND_EVENT_COLLECTOR_ERROR         ((WORD)0xC0002002L)
#define ND_EVENT_ACCESS_INFO             ((WORD)0x40003000L)
#define ND_EVENT_ACCESS_WARNING          ((WORD)0x80003001L)
#define ND_EVENT_ACCESS_ERROR            ((WORD)0xC0003002L)
#define ND_EVENT_HEALTH_INFO             ((WORD)0x40004000L)
#define ND_EVENT_HEALTH_WARNING          ((WORD)0x80004001L)
#define ND_EVENT_HEALTH_ERROR            ((WORD)0xC0004002L)
#define ND_EVENT_ACLK_INFO               ((WORD)0x40005000L)
#define ND_EVENT_ACLK_WARNING            ((WORD)0x80005001L)
#define ND_EVENT_ACLK_ERROR              ((WORD)0xC0005002L)

// --------------------------------------------------------------------------------------------------------------------

#define NETDATA_PROVIDER_NAME L"Netdata"
static HANDLE hEventLog = NULL;

static DWORD get_category(ND_LOG_SOURCES source) {
    // this is actually the task of the event

    switch(source) {
        default:
        case NDLS_DEBUG:
        case NDLS_DAEMON:
            return NETDATA_DAEMON_CATEGORY;

        case NDLS_COLLECTORS:
            return NETDATA_COLLECTOR_CATEGORY;

        case NDLS_ACCESS:
            return NETDATA_ACCESS_CATEGORY;

        case NDLS_HEALTH:
            return NETDATA_HEALTH_CATEGORY;

        case NDLS_ACLK:
            return NETDATA_ACLK_CATEGORY;
    }
}

static DWORD get_event_id(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority) {
    // the return value must match the manifest and the .mc file

    if(source == NDLS_DEBUG || source == NDLS_DAEMON) {
        switch(priority) {
            case NDLP_EMERG:
            case NDLP_ALERT:
            case NDLP_CRIT:
            case NDLP_ERR:
                return ND_EVENT_DAEMON_ERROR;

            case NDLP_WARNING:
                return ND_EVENT_DAEMON_WARNING;

            case NDLP_NOTICE:
            case NDLP_INFO:
            case NDLP_DEBUG:
            default:
                return ND_EVENT_DAEMON_INFO;
        }
    }

    if(source == NDLS_COLLECTORS) {
        switch(priority) {
            case NDLP_EMERG:
            case NDLP_ALERT:
            case NDLP_CRIT:
            case NDLP_ERR:
                return ND_EVENT_COLLECTOR_ERROR;

            case NDLP_WARNING:
                return ND_EVENT_COLLECTOR_WARNING;

            case NDLP_NOTICE:
            case NDLP_INFO:
            case NDLP_DEBUG:
            default:
                return ND_EVENT_COLLECTOR_INFO;
        }
    }

    if(source == NDLS_ACCESS) {
        switch(priority) {
            case NDLP_EMERG:
            case NDLP_ALERT:
            case NDLP_CRIT:
            case NDLP_ERR:
                return ND_EVENT_ACCESS_ERROR;

            case NDLP_WARNING:
                return ND_EVENT_ACCESS_WARNING;

            case NDLP_NOTICE:
            case NDLP_INFO:
            case NDLP_DEBUG:
            default:
                return ND_EVENT_ACCESS_INFO;
        }
    }

    if(source == NDLS_HEALTH) {
        switch(priority) {
            case NDLP_EMERG:
            case NDLP_ALERT:
            case NDLP_CRIT:
            case NDLP_ERR:
                return ND_EVENT_HEALTH_ERROR;

            case NDLP_WARNING:
                return ND_EVENT_HEALTH_WARNING;

            case NDLP_NOTICE:
            case NDLP_INFO:
            case NDLP_DEBUG:
            default:
                return ND_EVENT_HEALTH_INFO;
        }
    }

    if(source == NDLS_ACLK) {
        switch(priority) {
            case NDLP_EMERG:
            case NDLP_ALERT:
            case NDLP_CRIT:
            case NDLP_ERR:
                return ND_EVENT_ACLK_ERROR;

            case NDLP_WARNING:
                return ND_EVENT_ACLK_WARNING;

            case NDLP_NOTICE:
            case NDLP_INFO:
            case NDLP_DEBUG:
            default:
                return ND_EVENT_ACLK_INFO;
        }
    }

    return 0;
}

bool nd_log_wevents_init(void) {
    if(nd_log.wevents.initialized && hEventLog)
        return true;

    // Register the event source in the Windows Event Log under "Application"
    HKEY hRegKey;
    DWORD disposition;
    LONG result = RegCreateKeyExW(HKEY_LOCAL_MACHINE,
                                  L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\" NETDATA_PROVIDER_NAME,
                                  0, NULL, REG_OPTION_NON_VOLATILE, KEY_SET_VALUE, NULL, &hRegKey, &disposition);

    if (result != ERROR_SUCCESS)
        return false; // Could not create the registry key

    // Set the types of events this source will log (Error, Warning, Information)
    DWORD types_supported = EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
    RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));

    // Set the EventMessageFile value to point to the executable or DLL containing the event message strings
    wchar_t *modulePath = L"%SystemRoot%\\System32\\nd_wevents.dll";
    RegSetValueExW(hRegKey, L"CategoryMessageFile", 0, REG_EXPAND_SZ, (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));
    RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ, (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));
    RegSetValueExW(hRegKey, L"ParameterMessageFile", 0, REG_EXPAND_SZ, (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));

    RegCloseKey(hRegKey);

    // Register the event source with the system (this opens a handle to the event log)
    hEventLog = RegisterEventSourceW(NULL, NETDATA_PROVIDER_NAME);
    if (!hEventLog)
        return false; // Failed to register event source

    for(size_t i = 0; i < _NDLS_MAX ;i++)
        nd_log.sources[i].source = i;

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

    // we remove NDF_STOP from the list
    // so that %1 it the NDF_TIMESTAMP and %64 is NDF_MESSAGE
    for(size_t i = 1; i < _NDF_MAX ;i++)
        messages[i - 1] = fields_buffers[i].buf;
}

bool nd_logger_wevents(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    if(!nd_log.wevents.initialized || !hEventLog)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if(fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD eventType = get_event_type(priority);
    DWORD eventID = get_event_id(source->source, priority);
    DWORD category = get_category(source->source);

    CLEAN_BUFFER *tmp = NULL;

    char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    for (size_t i = 0; i < fields_max; i++) {
        fields_buffers[i].buf[0] = L'\0';

        if (!fields[i].entry.set || !fields[i].wevents)
            continue;

        const char *s = NULL;
        if(fields[i].annotator)
            s = fields[i].annotator(&fields[i]);

        else switch(fields[i].entry.type) {
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
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    uuid_unparse_lower(*fields[i].entry.uuid, number_str);
                    s = number_str;
                }
                break;
            case NDFT_CALLBACK:
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);

                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
                break;

            default:
                s = "UNHANDLED";
                break;
        }

        if(s && *s)
            MultiByteToWideChar(CP_UTF8, 0, s, -1, fields_buffers[i].buf, (int)fields_buffers[i].size);
    }

    BOOL rc = ReportEventW(hEventLog, eventType, category, eventID, NULL, _NDF_MAX - 1, 0, messages, NULL);
    spinlock_unlock(&spinlock);

    return rc == TRUE;
}

#endif
