// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_EVENTS_H
#define NETDATA_WINDOWS_EVENTS_H

#include "libnetdata/libnetdata.h"
#include "collectors/all.h"
#include <windows.h>
#include <winevt.h>
#include <wchar.h>

typedef enum {
    WEVT_NO_CHANNEL_MATCHED,
    WEVT_FAILED_TO_OPEN,
    WEVT_FAILED_TO_SEEK,
    WEVT_TIMED_OUT,
    WEVT_OK,
    WEVT_NOT_MODIFIED,
    WEVT_CANCELLED,
} WEVT_QUERY_STATUS;

#define WINEVENT_CHANNEL_CLASSIC_TRACE      0x0
#define WINEVENT_CHANNEL_GLOBAL_SYSTEM      0x8
#define WINEVENT_CHANNEL_GLOBAL_APPLICATION 0x9
#define WINEVENT_CHANNEL_GLOBAL_SECURITY    0xa

#define WINEVENT_LEVEL_NONE                 0x0
#define WINEVENT_LEVEL_CRITICAL             0x1
#define WINEVENT_LEVEL_ERROR                0x2
#define WINEVENT_LEVEL_WARNING              0x3
#define WINEVENT_LEVEL_INFORMATION 0x4
#define WINEVENT_LEVEL_VERBOSE              0x5
#define WINEVENT_LEVEL_RESERVED_6           0x6
#define WINEVENT_LEVEL_RESERVED_7           0x7
#define WINEVENT_LEVEL_RESERVED_8           0x8
#define WINEVENT_LEVEL_RESERVED_9           0x9
#define WINEVENT_LEVEL_RESERVED_10          0xa
#define WINEVENT_LEVEL_RESERVED_11          0xb
#define WINEVENT_LEVEL_RESERVED_12          0xc
#define WINEVENT_LEVEL_RESERVED_13          0xd
#define WINEVENT_LEVEL_RESERVED_14          0xe
#define WINEVENT_LEVEL_RESERVED_15          0xf

#define WINEVENT_OPCODE_INFO                0x0
#define WINEVENT_OPCODE_START               0x1
#define WINEVENT_OPCODE_STOP                0x2
#define WINEVENT_OPCODE_DC_START            0x3
#define WINEVENT_OPCODE_DC_STOP             0x4
#define WINEVENT_OPCODE_EXTENSION           0x5
#define WINEVENT_OPCODE_REPLY               0x6
#define WINEVENT_OPCODE_RESUME              0x7
#define WINEVENT_OPCODE_SUSPEND             0x8
#define WINEVENT_OPCODE_SEND                0x9
#define WINEVENT_OPCODE_RECEIVE             0xf0
#define WINEVENT_OPCODE_RESERVED_241        0xf1
#define WINEVENT_OPCODE_RESERVED_242        0xf2
#define WINEVENT_OPCODE_RESERVED_243        0xf3
#define WINEVENT_OPCODE_RESERVED_244        0xf4
#define WINEVENT_OPCODE_RESERVED_245        0xf5
#define WINEVENT_OPCODE_RESERVED_246        0xf6
#define WINEVENT_OPCODE_RESERVED_247        0xf7
#define WINEVENT_OPCODE_RESERVED_248        0xf8
#define WINEVENT_OPCODE_RESERVED_249        0xf9
#define WINEVENT_OPCODE_RESERVED_250        0xfa
#define WINEVENT_OPCODE_RESERVED_251        0xfb
#define WINEVENT_OPCODE_RESERVED_252        0xfc
#define WINEVENT_OPCODE_RESERVED_253        0xfd
#define WINEVENT_OPCODE_RESERVED_254        0xfe
#define WINEVENT_OPCODE_RESERVED_255        0xff

#define WINEVENT_TASK_NONE                  0x0

#define WINEVT_KEYWORD_NONE                 0x0
#define WINEVENT_KEYWORD_RESPONSE_TIME      0x0001000000000000
#define WINEVENT_KEYWORD_WDI_CONTEXT        0x0002000000000000
#define WINEVENT_KEYWORD_WDI_DIAG           0x0004000000000000
#define WINEVENT_KEYWORD_SQM                0x0008000000000000
#define WINEVENT_KEYWORD_AUDIT_FAILURE      0x0010000000000000
#define WINEVENT_KEYWORD_AUDIT_SUCCESS      0x0020000000000000
#define WINEVENT_KEYWORD_CORRELATION_HINT   0x0040000000000000
#define WINEVENT_KEYWORD_EVENTLOG_CLASSIC   0x0080000000000000
#define WINEVENT_KEYWORD_RESERVED_56        0x0100000000000000
#define WINEVENT_KEYWORD_RESERVED_57        0x0200000000000000
#define WINEVENT_KEYWORD_RESERVED_58        0x0400000000000000
#define WINEVENT_KEYWORD_RESERVED_59        0x0800000000000000
#define WINEVENT_KEYWORDE_RESERVED_60       0x1000000000000000
#define WINEVENT_KEYWORD_RESERVED_61        0x2000000000000000
#define WINEVENT_KEYWORD_RESERVED_62        0x4000000000000000
#define WINEVENT_KEYWORD_RESERVED_63        0x8000000000000000

#define WINEVENT_NAME_NONE                  "None"

#define WINEVENT_NAME_CRITICAL              "Critical"
#define WINEVENT_NAME_ERROR                 "Error"
#define WINEVENT_NAME_WARNING               "Warning"
#define WINEVENT_NAME_INFORMATION           "Information"
#define WINEVENT_NAME_VERBOSE               "Verbose"

#define WINEVENT_NAME_INFO                  "Info"
#define WINEVENT_NAME_START                 "Start"
#define WINEVENT_NAME_STOP                  "Stop"
#define WINEVENT_NAME_DC_START              "DC Start"
#define WINEVENT_NAME_DC_STOP               "DC Stop"
#define WINEVENT_NAME_EXTENSION             "Extension"
#define WINEVENT_NAME_REPLY                 "Reply"
#define WINEVENT_NAME_RESUME                "Resume"
#define WINEVENT_NAME_SUSPEND               "Suspend"
#define WINEVENT_NAME_SEND                  "Send"
#define WINEVENT_NAME_RECEIVE               "Receive"

#define WINEVENT_NAME_RESPONSE_TIME         "Response Time"
#define WINEVENT_NAME_WDI_CONTEXT           "WDI Context"
#define WINEVENT_NAME_WDI_DIAG              "WDI Diagnostics"
#define WINEVENT_NAME_SQM                   "SQM (Software Quality Metrics)"
#define WINEVENT_NAME_AUDIT_FAILURE         "Audit Failure"
#define WINEVENT_NAME_AUDIT_SUCCESS         "Audit Success"
#define WINEVENT_NAME_CORRELATION_HINT      "Correlation Hint"
#define WINEVENT_NAME_EVENTLOG_CLASSIC      "Event Log Classic"

#define WINEVENT_NAME_LEVEL_PREFIX          "Level "
#define WINEVENT_NAME_KEYWORDS_PREFIX       "Keywords "
#define WINEVENT_NAME_OPCODE_PREFIX         "Opcode "
#define WINEVENT_NAME_TASK_PREFIX           "Task "


#include "windows-events-unicode.h"
#include "windows-events-query.h"
#include "windows-events-sources.h"
#include "windows-events-sid.h"
#include "windows-events-xml.h"
#include "windows-events-publishers.h"
#include "windows-events-fields-cache.h"

#endif //NETDATA_WINDOWS_EVENTS_H
