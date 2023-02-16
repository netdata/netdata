// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG_H
#define NETDATA_LOG_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "../libnetdata.h"

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
#define D_METADATALOG       0x0000000400000000
#define D_ACLK_SYNC         0x0000000800000000
#define D_META_SYNC         0x0000001000000000
#define D_REPLICATION       0x0000002000000000
#define D_SYSTEM            0x8000000000000000

extern int web_server_is_multithreaded;

extern uint64_t debug_flags;

extern const char *program_name;

extern int stdaccess_fd;
extern FILE *stdaccess;

extern int stdhealth_fd;
extern FILE *stdhealth;

extern int stdcollector_fd;
extern FILE *stderror;

extern const char *stdaccess_filename;
extern const char *stderr_filename;
extern const char *stdout_filename;
extern const char *stdhealth_filename;
extern const char *stdcollector_filename;
extern const char *facility_log;

#ifdef ENABLE_ACLK
extern const char *aclklog_filename;
extern int aclklog_fd;
extern FILE *aclklog;
extern int aclklog_enabled;
#endif

extern int access_log_syslog;
extern int error_log_syslog;
extern int output_log_syslog;
extern int health_log_syslog;

extern time_t error_log_throttle_period;
extern unsigned long error_log_errors_per_period, error_log_errors_per_period_backup;
int error_log_limit(int reset);

void open_all_log_files();
void reopen_all_log_files();

#define LOG_DATE_LENGTH 26
void log_date(char *buffer, size_t len, time_t now);

static inline void debug_dummy(void) {}

void error_log_limit_reset(void);
void error_log_limit_unlimited(void);

typedef struct error_with_limit {
    time_t log_every;
    size_t count;
    time_t last_logged;
    usec_t sleep_ut;
} ERROR_LIMIT;

#define error_limit_static_global_var(var, log_every_secs, sleep_usecs) static ERROR_LIMIT var = { .last_logged = 0, .count = 0, .log_every = (log_every_secs), .sleep_ut = (sleep_usecs) }
#define error_limit_static_thread_var(var, log_every_secs, sleep_usecs) static __thread ERROR_LIMIT var = { .last_logged = 0, .count = 0, .log_every = (log_every_secs), .sleep_ut = (sleep_usecs) }

#ifdef NETDATA_INTERNAL_CHECKS
#define debug(type, args...) do { if(unlikely(debug_flags & type)) debug_int(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#define internal_error(condition, args...) do { if(unlikely(condition)) error_int(0, "IERR", __FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#define internal_fatal(condition, args...) do { if(unlikely(condition)) fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#else
#define debug(type, args...) debug_dummy()
#define internal_error(args...) debug_dummy()
#define internal_fatal(args...) debug_dummy()
#endif

#define info(args...)    info_int(0, __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_info(args...)    info_int(1, __FILE__, __FUNCTION__, __LINE__, ##args)
#define infoerr(args...) error_int(0, "INFO", __FILE__, __FUNCTION__, __LINE__, ##args)
#define error(args...)   error_int(0, "ERROR", __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_infoerr(args...) error_int(1, "INFO", __FILE__, __FUNCTION__, __LINE__, ##args)
#define collector_error(args...)   error_int(1, "ERROR", __FILE__, __FUNCTION__, __LINE__, ##args)
#define error_limit(erl, args...)   error_limit_int(erl, "ERROR", __FILE__, __FUNCTION__, __LINE__, ##args)
#define fatal(args...)   fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)
#define fatal_assert(expr) ((expr) ? (void)(0) : fatal_int(__FILE__, __FUNCTION__, __LINE__, "Assertion `%s' failed", #expr))

void send_statistics(const char *action, const char *action_result, const char *action_data);
void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(4, 5);
void info_int( int is_collector, const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(5, 6);
void error_int( int is_collector, const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(6, 7);
void error_limit_int(ERROR_LIMIT *erl, const char *prefix, const char *file __maybe_unused, const char *function __maybe_unused, unsigned long line __maybe_unused, const char *fmt, ... ) PRINTFLIKE(6, 7);;
void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) NORETURN PRINTFLIKE(4, 5);
void log_access( const char *fmt, ... ) PRINTFLIKE(1, 2);
void log_health( const char *fmt, ... ) PRINTFLIKE(1, 2);

#ifdef ENABLE_ACLK
void log_aclk_message_bin( const char *data, const size_t data_len, int tx, const char *mqtt_topic, const char *message_name);
#endif

# ifdef __cplusplus
}
# endif

#endif /* NETDATA_LOG_H */
