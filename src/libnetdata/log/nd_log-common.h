// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_COMMON_H
#define NETDATA_ND_LOG_COMMON_H

#include <syslog.h>

typedef enum  __attribute__((__packed__)) {
    NDLS_UNSET = 0,   // internal use only
    NDLS_ACCESS,      // access.log
    NDLS_ACLK,        // aclk.log
    NDLS_COLLECTORS,  // collector.log
    NDLS_DAEMON,      // error.log
    NDLS_HEALTH,      // health.log
    NDLS_DEBUG,       // debug.log

    // terminator
    _NDLS_MAX,
} ND_LOG_SOURCES;

typedef enum __attribute__((__packed__)) {
    NDLP_EMERG      = LOG_EMERG,    // from syslog.h
    NDLP_ALERT      = LOG_ALERT,    // from syslog.h
    NDLP_CRIT       = LOG_CRIT,     // from syslog.h
    NDLP_ERR        = LOG_ERR,      // from syslog.h
    NDLP_WARNING    = LOG_WARNING,  // from syslog.h
    NDLP_NOTICE     = LOG_NOTICE,   // from syslog.h
    NDLP_INFO       = LOG_INFO,     // from syslog.h
    NDLP_DEBUG      = LOG_DEBUG,    // from syslog.h

    // terminator
    _NDLP_MAX,
} ND_LOG_FIELD_PRIORITY;

typedef enum __attribute__((__packed__)) {
    // KEEP THESE IN THE SAME ORDER AS in thread_log_fields (log.c)
    // so that it easy to audit for missing fields

    // NEVER RENUMBER THIS LIST
    // The Windows Events Log has them at fixed positions

    NDF_STOP = 0,
    NDF_TIMESTAMP_REALTIME_USEC = 1,                // the timestamp of the log message - added automatically
    NDF_SYSLOG_IDENTIFIER = 2,                      // the syslog identifier of the application - added automatically
    NDF_LOG_SOURCE = 3,                             // DAEMON, COLLECTORS, HEALTH, MSGID_ACCESS, ACLK - set at the log call
    NDF_PRIORITY = 4,                               // the syslog priority (severity) - set at the log call
    NDF_ERRNO = 5,                                  // the ERRNO at the time of the log call - added automatically
    NDF_WINERROR = 6,                               // Windows GetLastError()
    NDF_INVOCATION_ID = 7,                          // the INVOCATION_ID of Netdata - added automatically
    NDF_LINE = 8,                                   // the source code file line number - added automatically
    NDF_FILE = 9,                                   // the source code filename - added automatically
    NDF_FUNC = 10,                                  // the source code function - added automatically
    NDF_TID = 11,                                   // the thread ID of the thread logging - added automatically
    NDF_THREAD_TAG = 12,                            // the thread tag of the thread logging - added automatically
    NDF_MESSAGE_ID = 13,                            // for specific events
    NDF_MODULE = 14,                                // for internal plugin module, all other get the NDF_THREAD_TAG

    NDF_NIDL_NODE = 15,                             // the node / rrdhost currently being worked
    NDF_NIDL_INSTANCE = 16,                         // the instance / rrdset currently being worked
    NDF_NIDL_CONTEXT = 17,                          // the context of the instance currently being worked
    NDF_NIDL_DIMENSION = 18,                        // the dimension / rrddim currently being worked

    // web server, aclk and stream receiver
    NDF_SRC_TRANSPORT = 19,                         // the transport we received the request, one of: http, https, pluginsd

    // Netdata Cloud Related
    NDF_ACCOUNT_ID = 20,
    NDF_USER_NAME = 21,
    NDF_USER_ROLE = 22,
    NDF_USER_ACCESS = 23,

    // web server and stream receiver
    NDF_SRC_IP = 24,                                // the streaming / web server source IP
    NDF_SRC_PORT = 25,                              // the streaming / web server source Port
    NDF_SRC_FORWARDED_HOST = 26,
    NDF_SRC_FORWARDED_FOR = 27,
    NDF_SRC_CAPABILITIES = 28,                      // the stream receiver capabilities

    // stream sender (established links)
    NDF_DST_TRANSPORT = 29,                         // the transport we send the request, one of: http, https
    NDF_DST_IP = 30,                                // the destination streaming IP
    NDF_DST_PORT = 31,                              // the destination streaming Port
    NDF_DST_CAPABILITIES = 32,                      // the destination streaming capabilities

    // web server, aclk and stream receiver
    NDF_REQUEST_METHOD = 33,                        // for http like requests, the http request method
    NDF_RESPONSE_CODE = 34,                         // for http like requests, the http response code, otherwise a status string

    // web server (all), aclk (queries)
    NDF_CONNECTION_ID = 35,                         // the web server connection ID
    NDF_TRANSACTION_ID = 36,                        // the web server and API transaction ID
    NDF_RESPONSE_SENT_BYTES = 37,                   // for http like requests, the response bytes
    NDF_RESPONSE_SIZE_BYTES = 38,                   // for http like requests, the uncompressed response size
    NDF_RESPONSE_PREPARATION_TIME_USEC = 39,        // for http like requests, the preparation time
    NDF_RESPONSE_SENT_TIME_USEC = 40,               // for http like requests, the time to send the response back
    NDF_RESPONSE_TOTAL_TIME_USEC = 41,              // for http like requests, the total time to complete the response

    // health alerts
    NDF_ALERT_ID = 42,
    NDF_ALERT_UNIQUE_ID = 43,
    NDF_ALERT_EVENT_ID = 44,
    NDF_ALERT_TRANSITION_ID = 45,
    NDF_ALERT_CONFIG_HASH = 46,
    NDF_ALERT_NAME = 47,
    NDF_ALERT_CLASS = 48,
    NDF_ALERT_COMPONENT = 49,
    NDF_ALERT_TYPE = 50,
    NDF_ALERT_EXEC = 51,
    NDF_ALERT_RECIPIENT = 52,
    NDF_ALERT_DURATION = 53,
    NDF_ALERT_VALUE = 54,
    NDF_ALERT_VALUE_OLD = 55,
    NDF_ALERT_STATUS = 56,
    NDF_ALERT_STATUS_OLD = 57,
    NDF_ALERT_SOURCE = 58,
    NDF_ALERT_UNITS = 59,
    NDF_ALERT_SUMMARY = 60,
    NDF_ALERT_INFO = 61,
    NDF_ALERT_NOTIFICATION_REALTIME_USEC = 62,
    NDF_REQUEST = 63,                               // the request we are currently working on
    NDF_MESSAGE = 64,                               // the log message, if any
    NDF_STACK_TRACE = 65,                           // stack trace of the thread logging

    // put new items here
    // NEVER RENUMBER FIELDS - RENUMBERING BREAKS EXISTING WINDOWS MESSAGES

    // terminator
    _NDF_MAX,
} ND_LOG_FIELD_ID;

typedef enum __attribute__((__packed__)) {
    NDFT_UNSET = 0,
    NDFT_TXT,
    NDFT_STR,
    NDFT_BFR,
    NDFT_U64,
    NDFT_I64,
    NDFT_DBL,
    NDFT_UUID,
    NDFT_CALLBACK,

    // terminator
    _NDFT_MAX,
} ND_LOG_STACK_FIELD_TYPE;

#endif //NETDATA_ND_LOG_COMMON_H
