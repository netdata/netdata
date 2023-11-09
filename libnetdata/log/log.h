// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG_H
#define NETDATA_LOG_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "../libnetdata.h"

typedef enum {
    NDLS_INPUT = 0,   // internal use only
    NDLS_ACCESS,      // access.log
    NDLS_ACLK,        // aclk.log
    NDLS_COLLECTORS,  // collectors.log
    NDLS_DAEMON,      // error.log
    NDLS_HEALTH,      // health.log
    NDLS_DEBUG,       // debug.log

    // terminator
    _NDLS_MAX,
} ND_LOG_SOURCES;

typedef enum {
    NDLP_ALERT      = LOG_ALERT,
    NDLP_CRIT       = LOG_CRIT,
    NDLP_EMERG      = LOG_EMERG,
    NDLP_ERR        = LOG_ERR,
    NDLP_WARNING    = LOG_WARNING,
    NDLP_INFO       = LOG_INFO,
    NDLP_NOTICE     = LOG_NOTICE,
    NDLP_DEBUG      = LOG_DEBUG,
} ND_LOG_FIELD_PRIORITY;

typedef enum {
    NDF_STOP = 0,
    NDF_TIMESTAMP_REALTIME_USEC,
    NDF_SYSLOG_IDENTIFIER,
    NDF_LINE,
    NDF_FILE,
    NDF_FUNC,
    NDF_ERRNO,
    NDF_PRIORITY,
    NDF_SESSION,
    NDF_TID,
    NDF_THREAD,
    NDF_PLUGIN,
    NDF_MODULE,
    NDF_JOB,
    NDF_NIDL_NODE,
    NDF_NIDL_INSTANCE,
    NDF_NIDL_DIMENSION,
    NDF_MESSAGE,

    // terminator
    _NDF_MAX,
} ND_LOG_FIELD_ID;

typedef enum {
    NDFT_UNSET = 0,
    NDFT_TXT,
    NDFT_U32,
    NDFT_I32,
    NDFT_U64,
    NDFT_I64,
    NDFT_PRIORITY,
    NDFT_TIMESTAMP,
} ND_LOG_STACK_FIELD_TYPE;

void nd_log_set_destination_output(ND_LOG_SOURCES type, const char *setting);
void nd_log_set_facility(const char *facility);
void nd_log_initialize(void);
void nd_log_reopen_log_files(void);
void chown_open_file(int fd, uid_t uid, gid_t gid);
void nd_log_chown_log_files(uid_t uid, gid_t gid);
void nd_log_set_flood_protection(time_t period, size_t logs);

struct log_stack_entry {
    int id;
    ND_LOG_STACK_FIELD_TYPE type;
    bool set;
    union {
        const char *str;
        uint32_t u32;
        uint64_t u64;
        int32_t i32;
        int64_t i64;
        ND_LOG_FIELD_PRIORITY priority;
    };
    struct log_stack_entry *prev, *next;
};

#define ND_LOG_STACK _cleanup_(log_stack_pop) struct log_stack_entry
#define ND_LOG_STACK_PUSH(lgs) log_stack_push(lgs)

#define ND_LOG_FIELD_STR(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_TXT, .str = (value), .set = true, }
#define ND_LOG_FIELD_U64(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_U64, .u64 = (value), .set = true, }
#define ND_LOG_FIELD_U32(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_U32, .u32 = (value), .set = true, }
#define ND_LOG_FIELD_I64(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_I64, .i64 = (value), .set = true, }
#define ND_LOG_FIELD_I32(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_I32, .i32 = (value), .set = true, }
#define ND_LOG_FIELD_PRI(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_PRIORITY, .priority = (value), .set = true, }
#define ND_LOG_FIELD_TMT(field, value) (struct log_stack_entry){ .id = (field), .type = NDFT_TIMESTAMP, .u64 = (value), .set = true, }
#define ND_LOG_FIELD_END() { .id = NDF_STOP, .type = NDFT_UNSET, .set = false, }

void log_stack_pop(void **ptr);
void log_stack_push(struct log_stack_entry *lgs);

//void example(void) {
//    ND_LOG_STACK lgs[] = {
//            ND_LOG_FIELD_STR(NDF_HOST, "hostname"),
//            ND_LOG_FIELD_END(),
//    };
//    ND_LOG_STACK_PUSH(lgs);
//
//    netdata_log(NDLS_DAEMON, NDLP_CRITICAL, "%s", "blabla");
//}



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

extern FILE *stdaccess;
extern FILE *stdhealth;
extern FILE *stderror;

#ifdef ENABLE_ACLK
extern FILE *aclklog;
extern int aclklog_enabled;
#endif

extern int access_log_syslog;
extern int error_log_syslog;
extern int health_log_syslog;

#define LOG_DATE_LENGTH 26
void log_date(char *buffer, size_t len, time_t now);

static inline void debug_dummy(void) {}

void nd_log_limits_reset(void);
void nd_log_limits_unlimited(void);

typedef enum netdata_log_level {
    NETDATA_LOG_LEVEL_ERROR,
    NETDATA_LOG_LEVEL_INFO,

    NETDATA_LOG_LEVEL_END
} netdata_log_level_t;

#define NETDATA_LOG_LEVEL_INFO_STR "info"
#define NETDATA_LOG_LEVEL_ERROR_STR "error"
#define NETDATA_LOG_LEVEL_ERROR_SHORT_STR "err"

extern netdata_log_level_t global_log_severity_level;
netdata_log_level_t log_severity_string_to_severity_level(char *level);
char *log_severity_level_to_severity_string(netdata_log_level_t level);
void log_set_global_severity_level(netdata_log_level_t value);
void log_set_global_severity_for_external_plugins();

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

#define netdata_log_error(args...)  netdata_logger(NDLS_DAEMON,     NDLP_ERR,   __FILE__, __FUNCTION__, __LINE__, ##args)
#define netdata_log_info(args...)   netdata_logger(NDLS_DAEMON,     NDLP_INFO,  __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_info(args...)     netdata_logger(NDLS_COLLECTORS, NDLP_ERR,   __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_error(args...)    netdata_logger(NDLS_COLLECTORS, NDLP_ERR,   __FILE__, __FUNCTION__, __LINE__, ##args)

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
void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) PRINTFLIKE(7, 8);;
#define nd_log_limit(erl, NDLS, NDLP, args...)   netdata_logger_with_limit(erl, NDLS, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)

// ----------------------------------------------------------------------------

void send_statistics(const char *action, const char *action_result, const char *action_data);
void netdata_logger_fatal( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) NORETURN PRINTFLIKE(4, 5);
void netdata_log_access( const char *fmt, ... ) PRINTFLIKE(1, 2);
void netdata_log_health( const char *fmt, ... ) PRINTFLIKE(1, 2);

#ifdef ENABLE_ACLK
void log_aclk_message_bin( const char *data, const size_t data_len, int tx, const char *mqtt_topic, const char *message_name);
#endif

# ifdef __cplusplus
}
# endif

#endif /* NETDATA_LOG_H */
