// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_H
#define NETDATA_ND_LOG_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "../libnetdata.h"

#define ND_LOG_DEFAULT_THROTTLE_LOGS 1000
#define ND_LOG_DEFAULT_THROTTLE_PERIOD 60

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
    NDLP_EMERG      = LOG_EMERG,
    NDLP_ALERT      = LOG_ALERT,
    NDLP_CRIT       = LOG_CRIT,
    NDLP_ERR        = LOG_ERR,
    NDLP_WARNING    = LOG_WARNING,
    NDLP_NOTICE     = LOG_NOTICE,
    NDLP_INFO       = LOG_INFO,
    NDLP_DEBUG      = LOG_DEBUG,
} ND_LOG_FIELD_PRIORITY;

typedef enum __attribute__((__packed__)) {
    // KEEP THESE IN THE SAME ORDER AS in thread_log_fields (log.c)
    // so that it easy to audit for missing fields

    // NEVER RENUMBER THIS LIST
    // The Windows Events Log has them at fixed positions

    NDF_STOP = 0,
    NDF_TIMESTAMP_REALTIME_USEC = 1,                // the timestamp of the log message - added automatically
    NDF_SYSLOG_IDENTIFIER = 2,                      // the syslog identifier of the application - added automatically
    NDF_LOG_SOURCE = 3,                             // DAEMON, COLLECTORS, HEALTH, ACCESS, ACLK - set at the log call
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
    // NDF_ALERT_FLAGS,

    // put new items here
    // leave the request URL and the message last

    NDF_REQUEST = 63,                               // the request we are currently working on
    NDF_MESSAGE = 64,                               // the log message, if any

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
} ND_LOG_STACK_FIELD_TYPE;

void errno_clear(void);
void nd_log_set_user_settings(ND_LOG_SOURCES source, const char *setting);
void nd_log_set_facility(const char *facility);
void nd_log_set_priority_level(const char *setting);
void nd_log_initialize(void);
void nd_log_reopen_log_files(bool log);
void chown_open_file(int fd, uid_t uid, gid_t gid);
void nd_log_chown_log_files(uid_t uid, gid_t gid);
void nd_log_set_flood_protection(size_t logs, time_t period);
void nd_log_initialize_for_external_plugins(const char *name);
void nd_log_reopen_log_files_for_spawn_server(void);
bool nd_log_journal_socket_available(void);
ND_LOG_FIELD_ID nd_log_field_id_by_journal_name(const char *field, size_t len);
int nd_log_priority2id(const char *priority);
const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority);
const char *nd_log_method_for_external_plugins(const char *s);

int nd_log_health_fd(void);
int nd_log_collectors_fd(void);
typedef bool (*log_formatter_callback_t)(BUFFER *wb, void *data);

struct log_stack_entry {
    ND_LOG_FIELD_ID id;
    ND_LOG_STACK_FIELD_TYPE type;
    bool set;
    union {
        const char *txt;
        struct netdata_string *str;
        BUFFER *bfr;
        uint64_t u64;
        int64_t i64;
        double dbl;
        const nd_uuid_t *uuid;
        struct {
            log_formatter_callback_t formatter;
            void *formatter_data;
        } cb;
    };
};

#define ND_LOG_STACK _cleanup_(log_stack_pop) struct log_stack_entry
#define ND_LOG_STACK_PUSH(lgs) log_stack_push(lgs)

#define ND_LOG_FIELD_TXT(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_TXT, .txt = (value), .set = true, }
#define ND_LOG_FIELD_STR(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_STR, .str = (value), .set = true, }
#define ND_LOG_FIELD_BFR(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_BFR, .bfr = (value), .set = true, }
#define ND_LOG_FIELD_U64(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_U64, .u64 = (value), .set = true, }
#define ND_LOG_FIELD_I64(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_I64, .i64 = (value), .set = true, }
#define ND_LOG_FIELD_DBL(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_DBL, .dbl = (value), .set = true, }
#define ND_LOG_FIELD_CB(field, func, data) (struct log_stack_entry){ .id = (field), .type = NDFT_CALLBACK, .cb = { .formatter = (func), .formatter_data = (data) }, .set = true, }
#define ND_LOG_FIELD_UUID(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_UUID, .uuid = (value), .set = true, }
#define ND_LOG_FIELD_END() (struct log_stack_entry){ .id = NDF_STOP, .type = NDFT_UNSET, .set = false, }

void log_stack_pop(void *ptr);
void log_stack_push(struct log_stack_entry *lgs);

#define D_WEB_BUFFER        0x0000000000000001
#define D_WEB_CLIENT        0x0000000000000002
#define D_LISTENER          0x0000000000000004
#define D_WEB_DATA          0x0000000000000008
#define D_OPTIONS           0x0000000000000010
#define D_PROCNETDEV_LOOP   0x0000000000000020
#define D_RRD_STATS         0x0000000000000040
#define D_WEB_CLIENT_ACCESS 0x0000000000000080
#define D_TC_LOOP           0x0000000000000100
#define D_DEFLATE           0x0000000000000200
#define D_CONFIG            0x0000000000000400
#define D_PLUGINSD          0x0000000000000800
#define D_CHILDS            0x0000000000001000
#define D_EXIT              0x0000000000002000
#define D_CHECKS            0x0000000000004000
#define D_NFACCT_LOOP       0x0000000000008000
#define D_PROCFILE          0x0000000000010000
#define D_RRD_CALLS         0x0000000000020000
#define D_DICTIONARY        0x0000000000040000
#define D_MEMORY            0x0000000000080000
#define D_CGROUP            0x0000000000100000
#define D_REGISTRY          0x0000000000200000
#define D_VARIABLES         0x0000000000400000
#define D_HEALTH            0x0000000000800000
#define D_CONNECT_TO        0x0000000001000000
#define D_RRDHOST           0x0000000002000000
#define D_LOCKS             0x0000000004000000
#define D_EXPORTING         0x0000000008000000
#define D_STATSD            0x0000000010000000
#define D_POLLFD            0x0000000020000000
#define D_STREAM            0x0000000040000000
#define D_ANALYTICS         0x0000000080000000
#define D_RRDENGINE         0x0000000100000000
#define D_ACLK              0x0000000200000000
#define D_REPLICATION       0x0000002000000000
#define D_SYSTEM            0x8000000000000000

extern uint64_t debug_flags;
extern const char *program_name;
extern int aclklog_enabled;

#define LOG_DATE_LENGTH 26
void log_date(char *buffer, size_t len, time_t now);

static inline void debug_dummy(void) {}

void nd_log_limits_reset(void);
void nd_log_limits_unlimited(void);

#define NDLP_INFO_STR "info"

#ifdef NETDATA_INTERNAL_CHECKS
#define netdata_log_debug(type, args...) do { if(unlikely(debug_flags & type)) netdata_logger(NDLS_DEBUG, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#define internal_error(condition, args...) do { if(unlikely(condition)) netdata_logger(NDLS_DAEMON, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#define internal_fatal(condition, args...) do { if(unlikely(condition)) netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#else
#define netdata_log_debug(type, args...) debug_dummy()
#define internal_error(args...) debug_dummy()
#define internal_fatal(args...) debug_dummy()
#endif

#define fatal(args...)   netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, ##args)
#define fatal_assert(expr) ((expr) ? (void)(0) : netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, "Assertion `%s' failed", #expr))

// ----------------------------------------------------------------------------
// normal logging

void netdata_logger(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) PRINTFLIKE(6, 7);
#define nd_log(NDLS, NDLP, args...) netdata_logger(NDLS, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)
#define nd_log_daemon(NDLP, args...) netdata_logger(NDLS_DAEMON, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)
#define nd_log_collector(NDLP, args...) netdata_logger(NDLS_COLLECTORS, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)

#define netdata_log_info(args...)   netdata_logger(NDLS_DAEMON,     NDLP_INFO,  __FILE__, __FUNCTION__, __LINE__, ##args)
#define netdata_log_error(args...)  netdata_logger(NDLS_DAEMON,     NDLP_ERR,   __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_info(args...)     netdata_logger(NDLS_COLLECTORS, NDLP_INFO,  __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_error(args...)    netdata_logger(NDLS_COLLECTORS, NDLP_ERR,   __FILE__, __FUNCTION__, __LINE__, ##args)

#define log_aclk_message_bin(__data, __data_len, __tx, __mqtt_topic, __message_name) \
    nd_log(NDLS_ACLK, NDLP_INFO, \
        "direction:%s message:'%s' topic:'%s' json:'%.*s'", \
        (__tx) ? "OUTGOING" : "INCOMING", __message_name, __mqtt_topic, (int)(__data_len), __data)

// ----------------------------------------------------------------------------
// logging with limits

typedef struct error_with_limit {
    SPINLOCK spinlock;
    time_t log_every;
    size_t count;
    time_t last_logged;
    usec_t sleep_ut;
} ERROR_LIMIT;

#define nd_log_limit_static_global_var(var, log_every_secs, sleep_usecs) static ERROR_LIMIT var = { .last_logged = 0, .count = 0, .log_every = (log_every_secs), .sleep_ut = (sleep_usecs) }
#define nd_log_limit_static_thread_var(var, log_every_secs, sleep_usecs) static __thread ERROR_LIMIT var = { .last_logged = 0, .count = 0, .log_every = (log_every_secs), .sleep_ut = (sleep_usecs) }
void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) PRINTFLIKE(7, 8);
#define nd_log_limit(erl, NDLS, NDLP, args...)   netdata_logger_with_limit(erl, NDLS, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)

// ----------------------------------------------------------------------------

void netdata_logger_fatal( const char *file, const char *function, unsigned long line, const char *fmt, ... ) NORETURN PRINTFLIKE(4, 5);

# ifdef __cplusplus
}
# endif

#endif /* NETDATA_ND_LOG_H */
