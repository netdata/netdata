// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_H
#define NETDATA_WINDOWS_EVENTS_H

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"

typedef enum {
    WEVT_NO_CHANNEL_MATCHED,
    WEVT_FAILED_TO_OPEN,
    WEVT_FAILED_TO_SEEK,
    WEVT_TIMED_OUT,
    WEVT_OK,
    WEVT_NOT_MODIFIED,
    WEVT_CANCELLED,
} WEVT_QUERY_STATUS;

#define WEVT_CHANNEL_CLASSIC_TRACE      0x0
#define WEVT_CHANNEL_GLOBAL_SYSTEM      0x8
#define WEVT_CHANNEL_GLOBAL_APPLICATION 0x9
#define WEVT_CHANNEL_GLOBAL_SECURITY    0xa

#define WEVT_LEVEL_NONE                 0x0
#define WEVT_LEVEL_CRITICAL             0x1
#define WEVT_LEVEL_ERROR                0x2
#define WEVT_LEVEL_WARNING              0x3
#define WEVT_LEVEL_INFORMATION          0x4
#define WEVT_LEVEL_VERBOSE              0x5
#define WEVT_LEVEL_RESERVED_6           0x6
#define WEVT_LEVEL_RESERVED_7           0x7
#define WEVT_LEVEL_RESERVED_8           0x8
#define WEVT_LEVEL_RESERVED_9           0x9
#define WEVT_LEVEL_RESERVED_10          0xa
#define WEVT_LEVEL_RESERVED_11          0xb
#define WEVT_LEVEL_RESERVED_12          0xc
#define WEVT_LEVEL_RESERVED_13          0xd
#define WEVT_LEVEL_RESERVED_14          0xe
#define WEVT_LEVEL_RESERVED_15          0xf

#define WEVT_OPCODE_INFO                0x0
#define WEVT_OPCODE_START               0x1
#define WEVT_OPCODE_STOP                0x2
#define WEVT_OPCODE_DC_START            0x3
#define WEVT_OPCODE_DC_STOP             0x4
#define WEVT_OPCODE_EXTENSION           0x5
#define WEVT_OPCODE_REPLY               0x6
#define WEVT_OPCODE_RESUME              0x7
#define WEVT_OPCODE_SUSPEND             0x8
#define WEVT_OPCODE_SEND                0x9
#define WEVT_OPCODE_RECEIVE             0xf0
#define WEVT_OPCODE_RESERVED_241        0xf1
#define WEVT_OPCODE_RESERVED_242        0xf2
#define WEVT_OPCODE_RESERVED_243        0xf3
#define WEVT_OPCODE_RESERVED_244        0xf4
#define WEVT_OPCODE_RESERVED_245        0xf5
#define WEVT_OPCODE_RESERVED_246        0xf6
#define WEVT_OPCODE_RESERVED_247        0xf7
#define WEVT_OPCODE_RESERVED_248        0xf8
#define WEVT_OPCODE_RESERVED_249        0xf9
#define WEVT_OPCODE_RESERVED_250        0xfa
#define WEVT_OPCODE_RESERVED_251        0xfb
#define WEVT_OPCODE_RESERVED_252        0xfc
#define WEVT_OPCODE_RESERVED_253        0xfd
#define WEVT_OPCODE_RESERVED_254        0xfe
#define WEVT_OPCODE_RESERVED_255        0xff

#define WEVT_TASK_NONE                  0x0

#define WEVT_KEYWORD_NONE               0x0
#define WEVT_KEYWORD_RESPONSE_TIME      0x0001000000000000
#define WEVT_KEYWORD_WDI_CONTEXT        0x0002000000000000
#define WEVT_KEYWORD_WDI_DIAG           0x0004000000000000
#define WEVT_KEYWORD_SQM                0x0008000000000000
#define WEVT_KEYWORD_AUDIT_FAILURE      0x0010000000000000
#define WEVT_KEYWORD_AUDIT_SUCCESS      0x0020000000000000
#define WEVT_KEYWORD_CORRELATION_HINT   0x0040000000000000
#define WEVT_KEYWORD_EVENTLOG_CLASSIC   0x0080000000000000
#define WEVT_KEYWORD_RESERVED_56        0x0100000000000000
#define WEVT_KEYWORD_RESERVED_57        0x0200000000000000
#define WEVT_KEYWORD_RESERVED_58        0x0400000000000000
#define WEVT_KEYWORD_RESERVED_59        0x0800000000000000
#define WEVT_KEYWORDE_RESERVED_60       0x1000000000000000
#define WEVT_KEYWORD_RESERVED_61        0x2000000000000000
#define WEVT_KEYWORD_RESERVED_62        0x4000000000000000
#define WEVT_KEYWORD_RESERVED_63        0x8000000000000000

#define WEVT_LEVEL_NAME_NONE                "None"
#define WEVT_LEVEL_NAME_CRITICAL            "Critical"
#define WEVT_LEVEL_NAME_ERROR               "Error"
#define WEVT_LEVEL_NAME_WARNING             "Warning"
#define WEVT_LEVEL_NAME_INFORMATION         "Information"
#define WEVT_LEVEL_NAME_VERBOSE             "Verbose"

#define WEVT_OPCODE_NAME_INFO               "Info"
#define WEVT_OPCODE_NAME_START              "Start"
#define WEVT_OPCODE_NAME_STOP               "Stop"
#define WEVT_OPCODE_NAME_DC_START           "DC Start"
#define WEVT_OPCODE_NAME_DC_STOP            "DC Stop"
#define WEVT_OPCODE_NAME_EXTENSION          "Extension"
#define WEVT_OPCODE_NAME_REPLY              "Reply"
#define WEVT_OPCODE_NAME_RESUME             "Resume"
#define WEVT_OPCODE_NAME_SUSPEND            "Suspend"
#define WEVT_OPCODE_NAME_SEND               "Send"
#define WEVT_OPCODE_NAME_RECEIVE            "Receive"

#define WEVT_TASK_NAME_NONE                 "None"

#define WEVT_KEYWORD_NAME_NONE              "None"
#define WEVT_KEYWORD_NAME_RESPONSE_TIME     "Response Time"
#define WEVT_KEYWORD_NAME_WDI_CONTEXT       "WDI Context"
#define WEVT_KEYWORD_NAME_WDI_DIAG          "WDI Diagnostics"
#define WEVT_KEYWORD_NAME_SQM               "SQM (Software Quality Metrics)"
#define WEVT_KEYWORD_NAME_AUDIT_FAILURE     "Audit Failure"
#define WEVT_KEYWORD_NAME_AUDIT_SUCCESS     "Audit Success"
#define WEVT_KEYWORD_NAME_CORRELATION_HINT  "Correlation Hint"
#define WEVT_KEYWORD_NAME_EVENTLOG_CLASSIC  "Event Log Classic"

#define WEVT_PREFIX_LEVEL                   "Level "        // the space at the end is needed
#define WEVT_PREFIX_KEYWORDS                "Keywords "     // the space at the end is needed
#define WEVT_PREFIX_OPCODE                  "Opcode "       // the space at the end is needed
#define WEVT_PREFIX_TASK                    "Task "         // the space at the end is needed

#include "windows-events-sources.h"
#include "windows-events-unicode.h"
#include "windows-events-xml.h"
#include "windows-events-providers.h"
#include "windows-events-fields-cache.h"
#include "windows-events-query.h"

// enable or disable preloading on full-text-search
#define ON_FTS_PRELOAD_MESSAGE      1
#define ON_FTS_PRELOAD_XML          0
#define ON_FTS_PRELOAD_EVENT_DATA   1

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze the Microsoft Windows Events log."
#define WEVT_FUNCTION_NAME           "windows-events"

#define WINDOWS_EVENTS_WORKER_THREADS 5
#define WINDOWS_EVENTS_DEFAULT_TIMEOUT 600
#define WINDOWS_EVENTS_SCAN_EVERY_USEC (5 * 60 * USEC_PER_SEC)
#define WINDOWS_EVENTS_PROGRESS_EVERY_UT (250 * USEC_PER_MS)
#define FUNCTION_PROGRESS_EVERY_ROWS (2000)
#define FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS (1000)
#define ANCHOR_DELTA_UT (10 * USEC_PER_SEC)

// run providers release every 5 mins
#define WINDOWS_EVENTS_RELEASE_PROVIDERS_HANDLES_EVERY_UT (5 * 60 * USEC_PER_SEC)
// release idle handles that are older than 5 mins
#define WINDOWS_EVENTS_RELEASE_IDLE_PROVIDER_HANDLES_TIME_UT (5 * 60 * USEC_PER_SEC)

#define WEVT_FIELD_COMPUTER             "Computer"
#define WEVT_FIELD_CHANNEL              "Channel"
#define WEVT_FIELD_PROVIDER             "Provider"
#define WEVT_FIELD_PROVIDER_GUID        "ProviderGUID"
#define WEVT_FIELD_EVENTRECORDID        "EventRecordID"
#define WEVT_FIELD_VERSION              "Version"
#define WEVT_FIELD_QUALIFIERS           "Qualifiers"
#define WEVT_FIELD_EVENTID              "EventID"
#define WEVT_FIELD_LEVEL                "Level"
#define WEVT_FIELD_KEYWORDS             "Keywords"
#define WEVT_FIELD_OPCODE               "Opcode"
#define WEVT_FIELD_ACCOUNT              "UserAccount"
#define WEVT_FIELD_DOMAIN               "UserDomain"
#define WEVT_FIELD_SID                  "UserSID"
#define WEVT_FIELD_TASK                 "Task"
#define WEVT_FIELD_PROCESSID            "ProcessID"
#define WEVT_FIELD_THREADID             "ThreadID"
#define WEVT_FIELD_ACTIVITY_ID          "ActivityID"
#define WEVT_FIELD_RELATED_ACTIVITY_ID  "RelatedActivityID"
#define WEVT_FIELD_XML                  "XML"
#define WEVT_FIELD_MESSAGE              "Message"
#define WEVT_FIELD_EVENTS_API           "EventsAPI"
#define WEVT_FIELD_EVENT_DATA_HIDDEN    "__HIDDEN__EVENT__DATA__"
#define WEVT_FIELD_EVENT_MESSAGE_HIDDEN "__HIDDEN__MESSAGE__DATA__"
#define WEVT_FIELD_EVENT_XML_HIDDEN     "__HIDDEN__XML__DATA__"

// functions needed by LQS

// structures needed by LQS
struct lqs_extension {
    wchar_t *query;

    struct {
        struct {
            size_t completed;
            size_t total;
        } queries;

        struct {
            size_t current_query_total;
            size_t completed;
            size_t total;
        } entries;

        usec_t last_ut;
    } progress;

    // struct {
    //     usec_t start_ut;
    //     usec_t stop_ut;
    //     usec_t first_msg_ut;
    //
    //     uint64_t first_msg_seqnum;
    // } query_file;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t slots;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t every;
    //     uint32_t skipped;
    //     uint32_t recalibrate;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples_per_file;

    // struct {
    //     usec_t start_ut;
    //     usec_t end_ut;
    //     usec_t step_ut;
    //     uint32_t enable_after_samples;
    //     uint32_t sampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    //     uint32_t unsampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    // } samples_per_time_slot;

    // per file progress info
    // size_t cached_count;

    // progress statistics
    usec_t matches_setup_ut;
    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t files_matched;
    size_t file_working;
};

// prepare LQS
#define LQS_DEFAULT_SLICE_MODE      0
#define LQS_FUNCTION_NAME           WEVT_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    WEVT_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             WEVT_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         WEVTS_ALL
#define LQS_SOURCE_TYPE_NONE        WEVTS_NONE
#define LQS_PARAMETER_SOURCE_NAME   "Event Channels" // this is how it is shown to users
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) WEVT_SOURCE_TYPE_2id_one(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) wevt_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

#include "windows-events-query-builder.h" // needs the LQS definition, so it has to be last

#endif //NETDATA_WINDOWS_EVENTS_H
