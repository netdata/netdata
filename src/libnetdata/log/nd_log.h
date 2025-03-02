// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_H
#define NETDATA_ND_LOG_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "../libnetdata.h"
#include "nd_log-common.h"
#include "nd_log-fatal.h"

#define ND_LOG_DEFAULT_THROTTLE_LOGS 1000
#define ND_LOG_DEFAULT_THROTTLE_PERIOD 60

void errno_clear(void);
int nd_log_systemd_journal_fd(void);
void nd_log_set_user_settings(ND_LOG_SOURCES source, const char *setting);
void nd_log_set_facility(const char *facility);
void nd_log_set_priority_level(const char *setting);
void nd_log_initialize(void);
void nd_log_reopen_log_files(bool log);
void chown_open_file(int fd, uid_t uid, gid_t gid);
void nd_log_chown_log_files(uid_t uid, gid_t gid);
void nd_log_set_flood_protection(size_t logs, time_t period);
void nd_log_initialize_for_external_plugins(const char *name);
void nd_log_reopen_log_files_for_spawn_server(const char *name);
bool nd_log_journal_socket_available(void);
ND_LOG_FIELD_ID nd_log_field_id_by_journal_name(const char *field, size_t len);
int nd_log_priority2id(const char *priority);
const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority);
const char *nd_log_method_for_external_plugins(const char *s);
ND_UUID nd_log_get_invocation_id(void);
void capture_stack_trace(BUFFER *wb);

typedef void (*log_event_t)(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line);
void nd_log_register_fatal_data_cb(log_event_t cb);

typedef void (*fatal_event_t)(void);
void nd_log_register_fatal_final_cb(fatal_event_t cb);

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
#else
#define netdata_log_debug(type, args...) debug_dummy()
#define internal_error(args...) debug_dummy()
#endif

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

#define nd_log_limit_static_global_var(var, log_every_secs, sleep_usecs) static ERROR_LIMIT var = { .spinlock = SPINLOCK_INITIALIZER, .log_every = (log_every_secs), .count = 0, .last_logged = 0, .sleep_ut = (sleep_usecs) }
#define nd_log_limit_static_thread_var(var, log_every_secs, sleep_usecs) static __thread ERROR_LIMIT var = { .last_logged = 0, .count = 0, .log_every = (log_every_secs), .sleep_ut = (sleep_usecs) }
void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) PRINTFLIKE(7, 8);
#define nd_log_limit(erl, NDLS, NDLP, args...)   netdata_logger_with_limit(erl, NDLS, NDLP, __FILE__, __FUNCTION__, __LINE__, ##args)

// ----------------------------------------------------------------------------


#define error_report(x, args...) do { errno_clear(); netdata_log_error(x, ##args); } while(0)

# ifdef __cplusplus
}
# endif

#endif /* NETDATA_ND_LOG_H */
