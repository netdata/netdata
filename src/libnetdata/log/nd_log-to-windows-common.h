// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_TO_WINDOWS_COMMON_H
#define NETDATA_ND_LOG_TO_WINDOWS_COMMON_H

// Helper macro to create wide string literals
#define WIDEN2(x) L ## x
#define WIDEN(x) WIDEN2(x)

#define NETDATA_ETW_PROVIDER_GUID_STR       "{96c5ca72-9bd8-4634-81e5-000014e7da7a}"
#define NETDATA_ETW_PROVIDER_GUID_STR_W     WIDEN(NETDATA_ETW_PROVIDER_GUID)

#define NETDATA_CHANNEL_NAME                "Netdata"
#define NETDATA_CHANNEL_NAME_W              WIDEN(NETDATA_CHANNEL_NAME)

#define NETDATA_WEL_CHANNEL_NAME            "NetdataWEL"
#define NETDATA_WEL_CHANNEL_NAME_W          WIDEN(NETDATA_WEL_CHANNEL_NAME)

#define NETDATA_ETW_CHANNEL_NAME            "Netdata"
#define NETDATA_ETW_CHANNEL_NAME_W          WIDEN(NETDATA_ETW_CHANNEL_NAME)

#define NETDATA_ETW_PROVIDER_NAME           "Netdata"
#define NETDATA_ETW_PROVIDER_NAME_W         WIDEN(NETDATA_ETW_PROVIDER_NAME)

#define NETDATA_WEL_PROVIDER_PREFIX         "Netdata"
#define NETDATA_WEL_PROVIDER_PREFIX_W       WIDEN(NETDATA_WEL_PROVIDER_PREFIX)

#define NETDATA_WEL_PROVIDER_ACCESS         NETDATA_WEL_PROVIDER_PREFIX "Access"
#define NETDATA_WEL_PROVIDER_ACCESS_W       WIDEN(NETDATA_WEL_PROVIDER_ACCESS)

#define NETDATA_WEL_PROVIDER_ACLK           NETDATA_WEL_PROVIDER_PREFIX "Aclk"
#define NETDATA_WEL_PROVIDER_ACLK_W         WIDEN(NETDATA_WEL_PROVIDER_ACLK)

#define NETDATA_WEL_PROVIDER_COLLECTORS     NETDATA_WEL_PROVIDER_PREFIX "Collectors"
#define NETDATA_WEL_PROVIDER_COLLECTORS_W   WIDEN(NETDATA_WEL_PROVIDER_COLLECTORS)

#define NETDATA_WEL_PROVIDER_DAEMON         NETDATA_WEL_PROVIDER_PREFIX "Daemon"
#define NETDATA_WEL_PROVIDER_DAEMON_W       WIDEN(NETDATA_WEL_PROVIDER_DAEMON)

#define NETDATA_WEL_PROVIDER_HEALTH         NETDATA_WEL_PROVIDER_PREFIX "Health"
#define NETDATA_WEL_PROVIDER_HEALTH_W       WIDEN(NETDATA_WEL_PROVIDER_HEALTH)


#define NETDATA_ETW_SUBCHANNEL_ACCESS       "Access"
#define NETDATA_ETW_SUBCHANNEL_ACCESS_W     WIDEN(NETDATA_ETW_SUBCHANNEL_ACCESS)

#define NETDATA_ETW_SUBCHANNEL_ACLK         "Aclk"
#define NETDATA_ETW_SUBCHANNEL_ACLK_W       WIDEN(NETDATA_ETW_SUBCHANNEL_ACLK)

#define NETDATA_ETW_SUBCHANNEL_COLLECTORS   "Collectors"
#define NETDATA_ETW_SUBCHANNEL_COLLECTORS_W WIDEN(NETDATA_ETW_SUBCHANNEL_COLLECTORS)

#define NETDATA_ETW_SUBCHANNEL_DAEMON       "Daemon"
#define NETDATA_ETW_SUBCHANNEL_DAEMON_W     WIDEN(NETDATA_ETW_SUBCHANNEL_DAEMON)

#define NETDATA_ETW_SUBCHANNEL_HEALTH       "Health"
#define NETDATA_ETW_SUBCHANNEL_HEALTH_W     WIDEN(NETDATA_ETW_SUBCHANNEL_HEALTH)

// Define shift values
#define EVENT_ID_SEV_SHIFT          30
#define EVENT_ID_C_SHIFT            29
#define EVENT_ID_R_SHIFT            28
#define EVENT_ID_FACILITY_SHIFT     16
#define EVENT_ID_CODE_SHIFT         0

#define EVENT_ID_PRIORITY_SHIFT     0          // Shift 0 bits
#define EVENT_ID_SOURCE_SHIFT       4          // Shift 4 bits

// Define masks
#define EVENT_ID_SEV_MASK           0xC0000000 // Bits 31-30
#define EVENT_ID_C_MASK             0x20000000 // Bit 29
#define EVENT_ID_R_MASK             0x10000000 // Bit 28
#define EVENT_ID_FACILITY_MASK      0x0FFF0000 // Bits 27-16
#define EVENT_ID_CODE_MASK          0x0000FFFF // Bits 15-0

#define EVENT_ID_PRIORITY_MASK      0x000F     // Bits 0-3
#define EVENT_ID_SOURCE_MASK        0x00F0     // Bits 4-7

typedef enum __attribute__((packed)) {
    MSGID_MESSAGE_ONLY = 1,
    MSGID_MESSAGE_ERRNO,
    MSGID_REQUEST_ONLY,
    MSGID_ALERT_TRANSITION,
    MSGID_ACCESS,
    MSGID_ACCESS_FORWARDER,
    MSGID_ACCESS_USER,
    MSGID_ACCESS_FORWARDER_USER,
    MSGID_ACCESS_MESSAGE,
    MSGID_ACCESS_MESSAGE_REQUEST,
    MSGID_ACCESS_MESSAGE_USER,

    // terminator
    _MSGID_MAX,
} MESSAGE_ID;

static inline uint32_t get_event_type_from_priority(ND_LOG_FIELD_PRIORITY priority) {
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

static inline uint8_t get_severity_from_priority(ND_LOG_FIELD_PRIORITY priority) {
    switch (priority) {
        case NDLP_EMERG:
        case NDLP_ALERT:
        case NDLP_CRIT:
        case NDLP_ERR:
            return STATUS_SEVERITY_ERROR;

        case NDLP_WARNING:
            return STATUS_SEVERITY_WARNING;

        case NDLP_NOTICE:
        case NDLP_INFO:
        case NDLP_DEBUG:
        default:
            return STATUS_SEVERITY_INFORMATIONAL;
    }
}

static inline uint8_t get_level_from_priority(ND_LOG_FIELD_PRIORITY priority) {
    switch (priority) {
        // return 0 = log an event regardless of any filtering applied

        case NDLP_EMERG:
        case NDLP_ALERT:
        case NDLP_CRIT:
            return 1;

        case NDLP_ERR:
            return 2;

        case NDLP_WARNING:
            return 3;

        case NDLP_NOTICE:
        case NDLP_INFO:
            return 4;

        case NDLP_DEBUG:
        default:
            return 5;
    }
}

static inline const char *get_level_from_priority_str(ND_LOG_FIELD_PRIORITY priority) {
    switch (priority) {
        // return "win:LogAlways" to log an event regardless of any filtering applied

        case NDLP_EMERG:
        case NDLP_ALERT:
        case NDLP_CRIT:
            return "win:Critical";

        case NDLP_ERR:
            return "win:Error";

        case NDLP_WARNING:
            return "win:Warning";

        case NDLP_NOTICE:
        case NDLP_INFO:
            return "win:Informational";

        case NDLP_DEBUG:
        default:
            return "win:Verbose";
    }
}

static inline uint16_t construct_event_code(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, MESSAGE_ID messageID) {
    return (source << 12  | priority << 8 | messageID << 0);
}

#endif //NETDATA_ND_LOG_TO_WINDOWS_COMMON_H
