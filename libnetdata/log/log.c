// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include <daemon/main.h>

#ifdef HAVE_BACKTRACE
#include <execinfo.h>
#endif

// ----------------------------------------------------------------------------
// facilities
//
// sys/syslog.h (Linux)
// sys/sys/syslog.h (FreeBSD)
// bsd/sys/syslog.h (darwin-xnu)

static struct {
    int facility;
    const char *name;
} facilities[] = {
        { LOG_AUTH, "auth" },
        { LOG_AUTHPRIV, "authpriv" },
        { LOG_CRON, "cron" },
        { LOG_DAEMON, "daemon" },
        { LOG_FTP, "ftp" },
        { LOG_KERN, "kern" },
        { LOG_LPR, "lpr" },
        { LOG_MAIL, "mail" },
        { LOG_NEWS, "news" },
        { LOG_SYSLOG, "syslog" },
        { LOG_USER, "user" },
        { LOG_UUCP, "uucp" },
        { LOG_LOCAL0, "local0" },
        { LOG_LOCAL1, "local1" },
        { LOG_LOCAL2, "local2" },
        { LOG_LOCAL3, "local3" },
        { LOG_LOCAL4, "local4" },
        { LOG_LOCAL5, "local5" },
        { LOG_LOCAL6, "local6" },
        { LOG_LOCAL7, "local7" },

#ifdef __FreeBSD__
        { LOG_CONSOLE, "console" },
        { LOG_NTP, "ntp" },

        // FreeBSD does not consider 'security' as deprecated.
        { LOG_SECURITY, "security" },
#else
        // For all other O/S 'security' is mapped to 'auth'.
        { LOG_AUTH, "security" },
#endif

#ifdef __APPLE__
        { LOG_INSTALL, "install" },
        { LOG_NETINFO, "netinfo" },
        { LOG_RAS,     "ras" },
        { LOG_REMOTEAUTH, "remoteauth" },
        { LOG_LAUNCHD, "launchd" },

#endif
};

static int nd_log_facility2id(const char *facility) {
    size_t entries = sizeof(facilities) / sizeof(facilities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(facilities[i].name, facility) == 0)
            return facilities[i].facility;
    }

    return LOG_DAEMON;
}

// ----------------------------------------------------------------------------

typedef enum {
    ND_LOG_METHOD_DISABLED,
    ND_LOG_METHOD_DEVNULL,
    ND_LOG_METHOD_DEFAULT,
    ND_LOG_METHOD_JOURNAL,
    ND_LOG_METHOD_SYSLOG,
    ND_LOG_METHOD_STDOUT,
    ND_LOG_METHOD_STDERR,
    ND_LOG_METHOD_FILE,
} ND_LOG_METHOD;

struct nd_log_entry {
    SPINLOCK spinlock;
    ND_LOG_METHOD method;
    const char *filename;
    int fd;
    FILE *fp;
    FILE **fp_set;
};

static struct {
    struct nd_log_entry types[ND_LOG_TYPES_MAX];

    struct {
        bool initialized;
        int facility;
    } syslog;

    struct {
        bool initialized;
    } stdin;

    struct {
        bool initialized;
    } stdout;

    struct {
        bool initialized;
    } stderr;

} nd_log = {
        .syslog = {
                .initialized = false,
                .facility = LOG_DAEMON,
        },
        .stdin = {
                .initialized = false,
        },
        .stdout = {
                .initialized = false,
        },
        .stderr = {
                .initialized = false,
        },
        .types = {
                [ND_LOG_INPUT] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEVNULL,
                        .filename = "/dev/null",
                        .fd = STDIN_FILENO,
                        .fp_set = &stdin,
                        .fp = NULL,
                },
                [ND_LOG_ACCESS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/access.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                },
                [ND_LOG_ACLK] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_FILE,
                        .filename = LOG_DIR "/aclk.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                },
                [ND_LOG_COLLECTORS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/collectors.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                },
                [ND_LOG_DEBUG] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DISABLED,
                        .filename = LOG_DIR "/debug.log",
                        .fd = STDOUT_FILENO,
                        .fp_set = &stdout,
                        .fp = NULL,
                },
                [ND_LOG_ERROR] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/error.log",
                        .fd = STDERR_FILENO,
                        .fp_set = &stderr,
                        .fp = NULL,
                },
                [ND_LOG_HEALTH] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/health.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                },
        },
};

static inline void nd_log_lock(ND_LOG_TYPE type) {
    spinlock_lock(&nd_log.types[type].spinlock);
}

static inline void nd_log_unlock(ND_LOG_TYPE type) {
    spinlock_unlock(&nd_log.types[type].spinlock);
}

void nd_log_set_output(ND_LOG_TYPE type, const char *setting) {
    if(!setting || !*setting || strcmp(setting, "none") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_DISABLED;
        nd_log.types[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "journal") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_JOURNAL;
        nd_log.types[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "syslog") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_SYSLOG;
        nd_log.types[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "/dev/null") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_DEVNULL;
        nd_log.types[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "system") == 0) {
        if(nd_log.types[type].fp_set == &stderr) {
            nd_log.types[type].method = ND_LOG_METHOD_STDERR;
            nd_log.types[type].filename = "stderr";
            nd_log.types[type].fd = STDERR_FILENO;
        }
        else {
            nd_log.types[type].method = ND_LOG_METHOD_STDOUT;
            nd_log.types[type].filename = "stdout";
            nd_log.types[type].fd = STDOUT_FILENO;
        }
    }
    else if(strcmp(setting, "stderr") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_STDERR;
        nd_log.types[type].filename = "stderr";
        nd_log.types[type].fd = STDERR_FILENO;
    }
    else if(strcmp(setting, "stdout") == 0) {
        nd_log.types[type].method = ND_LOG_METHOD_STDOUT;
        nd_log.types[type].filename = "stdout";
        nd_log.types[type].fd = STDOUT_FILENO;
    }
    else {
        nd_log.types[type].method = ND_LOG_METHOD_FILE;
        nd_log.types[type].filename = setting;
    }
}

void nd_log_set_facility(const char *facility) {
    nd_log.syslog.facility = nd_log_facility2id(facility);
}

static void nd_log_syslog_init() {
    if(nd_log.syslog.initialized)
        return;

    openlog(program_name, LOG_PID, nd_log.syslog.facility);
    nd_log.syslog.initialized = true;
}

static bool nd_log_set_system_fd(struct nd_log_entry *e, int new_fd) {
    if(new_fd == -1 || e->fd == -1 ||
            (e->fd == STDIN_FILENO && nd_log.stdin.initialized) ||
            (e->fd == STDOUT_FILENO && nd_log.stdout.initialized) ||
            (e->fd == STDERR_FILENO && nd_log.stderr.initialized) ||
            (e->fd != STDIN_FILENO && e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO))
        return false;

    if(new_fd != e->fd) {
        int t = dup2(new_fd, e->fd);

        bool ret = true;
        if (t == -1) {
            netdata_log_error("Cannot dup2() new fd %d to old fd %d for '%s'", new_fd, e->fd, e->filename);
            ret = false;
        }
        else
            close(new_fd);

        if(e->fd == STDIN_FILENO)
            nd_log.stdin.initialized = true;
        else if(e->fd == STDOUT_FILENO)
            nd_log.stdout.initialized = true;
        else if(e->fd == STDERR_FILENO)
            nd_log.stderr.initialized = true;

        return ret;
    }

    return false;
}

static void nd_log_open(struct nd_log_entry *e, ND_LOG_TYPE type) {
    if(e->method == ND_LOG_METHOD_DEFAULT)
        nd_log_set_output(type, e->filename);

    if((e->method == ND_LOG_METHOD_FILE && !e->filename) ||
       (e->method == ND_LOG_METHOD_DEVNULL && !e->fp_set))
        e->method = ND_LOG_METHOD_DISABLED;

    if(e->fp)
        fflush(e->fp);

    switch(e->method) {
        case ND_LOG_METHOD_SYSLOG:
            nd_log_syslog_init();
            break;

        case ND_LOG_METHOD_JOURNAL:
            break;

        case ND_LOG_METHOD_STDOUT:
            e->fp = stdout;
            e->fd = STDOUT_FILENO;
            break;

        default:
        case ND_LOG_METHOD_DEFAULT:
        case ND_LOG_METHOD_STDERR:
            e->method = ND_LOG_METHOD_STDERR;
            e->fp = stderr;
            e->fd = STDERR_FILENO;
            break;

        case ND_LOG_METHOD_DEVNULL:
        case ND_LOG_METHOD_FILE: {
            int fd = open(e->filename, O_WRONLY | O_APPEND | O_CREAT, 0664);
            if(fd == -1) {
                // we cannot open the file, fallback to stderr

                if(e->fd != STDIN_FILENO && e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO) {
                    e->fd = STDERR_FILENO;
                    e->method = ND_LOG_METHOD_STDERR;
                }

                netdata_log_error("Cannot open file '%s'. Leaving %d to its default.", e->filename, e->fd);
            }
            else {
                if (!nd_log_set_system_fd(e, fd)) {
                    if(e->fd == STDIN_FILENO || e->fd == STDOUT_FILENO || e->fd == STDERR_FILENO) {
                        if(e->fd == STDOUT_FILENO)
                            e->method = ND_LOG_METHOD_STDOUT;
                        else if(e->fd == STDERR_FILENO)
                            e->method = ND_LOG_METHOD_STDERR;

                        close(fd);
                    }
                    else
                        e->fd = fd;
                }
            }

            // at this point we have e->fd set properly

            if(e->fd == STDIN_FILENO)
                e->fp = stdin;
            else if(e->fd == STDOUT_FILENO)
                e->fp = stdout;
            else if(e->fd == STDERR_FILENO)
                e->fp = stderr;
            else if(e->fp == stdin || e->fp == stdout || e->fp == stderr)
                e->fp = NULL;

            if(!e->fp) {
                e->fp = fdopen(e->fd, "a");
                if (!e->fp) {
                    netdata_log_error("Cannot fdopen() fd %d ('%s')", e->fd, e->filename);

                    if(e->fd != STDIN_FILENO && e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO)
                        close(e->fd);

                    e->fp = stderr;
                    e->fd = STDOUT_FILENO;
                }
            }

            // at this point we have e->fp set properly

            if(e->fp && e->fp != stdin) {
                if (setvbuf(e->fp, NULL, _IOLBF, 0) != 0)
                    netdata_log_error("Cannot set line buffering on fd %d ('%s')", e->fd, e->filename);
            }
        }
        break;
    }
}

void nd_log_initialize(void) {
    for(int i = 0 ; i < ND_LOG_TYPES_MAX ;i++)
        nd_log_open(&nd_log.types[i], i);
}

void nd_log_reopen_log_files(void) {
    netdata_log_info("Reopening all log files.");

    nd_log.stdout.initialized = false;
    nd_log.stderr.initialized = false;
    nd_log_initialize();

    netdata_log_info("Log files re-opened.");
}

const char *program_name = "";
uint64_t debug_flags = 0;

int access_log_syslog = 0;
int error_log_syslog = 0;
int collector_log_syslog = 0;
int output_log_syslog = 0;  // debug log
int health_log_syslog = 0;

int stdaccess_fd = -1;
FILE *stdaccess = NULL;
FILE *stdhealth = NULL;
FILE *stderror = NULL;

netdata_log_level_t global_log_severity_level = NETDATA_LOG_LEVEL_INFO;

#ifdef ENABLE_ACLK
FILE *aclklog = NULL;
int aclklog_enabled = 0;
#endif

// ----------------------------------------------------------------------------

void log_date(char *buffer, size_t len, time_t now) {
    if(unlikely(!buffer || !len))
        return;

    time_t t = now;
    struct tm *tmp, tmbuf;

    tmp = localtime_r(&t, &tmbuf);

    if (tmp == NULL) {
        buffer[0] = '\0';
        return;
    }

    if (unlikely(strftime(buffer, len, "%Y-%m-%d %H:%M:%S", tmp) == 0))
        buffer[0] = '\0';

    buffer[len - 1] = '\0';
}

// ----------------------------------------------------------------------------
// error log throttling

time_t error_log_throttle_period = 1200;
unsigned long error_log_errors_per_period = 200;
unsigned long error_log_errors_per_period_backup = 0;

int error_log_limit(int reset) {
    static time_t start = 0;
    static unsigned long counter = 0, prevented = 0;

    FILE *fp = stderror ? stderror : stderr;

    // fprintf(fp, "FLOOD: counter=%lu, allowed=%lu, backup=%lu, period=%llu\n", counter, error_log_errors_per_period, error_log_errors_per_period_backup, (unsigned long long)error_log_throttle_period);

    // do not throttle if the period is 0
    if(error_log_throttle_period == 0)
        return 0;

    // prevent all logs if the errors per period is 0
    if(error_log_errors_per_period == 0)
#ifdef NETDATA_INTERNAL_CHECKS
        return 0;
#else
        return 1;
#endif

    time_t now = now_monotonic_sec();
    if(!start) start = now;

    if(reset) {
        if(prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                fp,
                "%s: %s LOG FLOOD PROTECTION reset for process '%s' "
                "(prevented %lu logs in the last %"PRId64" seconds).\n",
                date,
                program_name,
                program_name,
                prevented,
                (int64_t)(now - start));
        }

        start = now;
        counter = 0;
        prevented = 0;
    }

    // detect if we log too much
    counter++;

    if(now - start > error_log_throttle_period) {
        if(prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                fp,
                "%s: %s LOG FLOOD PROTECTION resuming logging from process '%s' "
                "(prevented %lu logs in the last %"PRId64" seconds).\n",
                date,
                program_name,
                program_name,
                prevented,
                (int64_t)error_log_throttle_period);
        }

        // restart the period accounting
        start = now;
        counter = 1;
        prevented = 0;

        // log this error
        return 0;
    }

    if(counter > error_log_errors_per_period) {
        if(!prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                fp,
                "%s: %s LOG FLOOD PROTECTION too many logs (%lu logs in %"PRId64" seconds, threshold is set to %lu logs "
                "in %"PRId64" seconds). Preventing more logs from process '%s' for %"PRId64" seconds.\n",
                date,
                program_name,
                counter,
                (int64_t)(now - start),
                error_log_errors_per_period,
                (int64_t)error_log_throttle_period,
                program_name,
                (int64_t)(start + error_log_throttle_period - now));
        }

        prevented++;

        // prevent logging this error
#ifdef NETDATA_INTERNAL_CHECKS
        return 0;
#else
        return 1;
#endif
    }

    return 0;
}

void error_log_limit_reset(void) {
    nd_log_lock(ND_LOG_ERROR);

    error_log_errors_per_period = error_log_errors_per_period_backup;
    error_log_limit(1);

    nd_log_unlock(ND_LOG_ERROR);
}

void error_log_limit_unlimited(void) {
    nd_log_lock(ND_LOG_ERROR);

    error_log_errors_per_period = error_log_errors_per_period_backup;
    error_log_limit(1);

    error_log_errors_per_period = ((error_log_errors_per_period_backup * 10) < 10000) ? 10000 : (error_log_errors_per_period_backup * 10);

    nd_log_unlock(ND_LOG_ERROR);
}

// ----------------------------------------------------------------------------
// debug log

void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    va_list args;

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    va_start( args, fmt );
    printf("%s: %s DEBUG : %s : (%04lu@%-20.20s:%-15.15s): ", date, program_name, netdata_thread_tag(), line, file, function);
    vprintf(fmt, args);
    va_end( args );
    putchar('\n');

    if(output_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_ERR,  fmt, args );
        va_end( args );
    }

    fflush(stdout);
}

// ----------------------------------------------------------------------------
// info log

void info_int( int is_collector, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... )
{
#if !defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_DEV_MODE)
    if (NETDATA_LOG_LEVEL_INFO > global_log_severity_level)
        return;
#endif

    va_list args;
    FILE *fp = (is_collector || !stderror) ? stderr : stderror;

    nd_log_lock(ND_LOG_ERROR);

    // prevent logging too much
    if (error_log_limit(0)) {
        nd_log_unlock(ND_LOG_ERROR);
        return;
    }

    if(collector_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_INFO,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    va_start( args, fmt );
#ifdef NETDATA_INTERNAL_CHECKS
    fprintf(fp, "%s: %s INFO  : %s : (%04lu@%-20.20s:%-15.15s): ",
            date, program_name, netdata_thread_tag(), line, file, function);
#else
    fprintf(fp, "%s: %s INFO  : %s : ", date, program_name, netdata_thread_tag());
#endif
    vfprintf(fp, fmt, args );
    va_end( args );

    fputc('\n', fp);

    nd_log_unlock(ND_LOG_ERROR);
}

// ----------------------------------------------------------------------------
// error log

#if defined(STRERROR_R_CHAR_P)
// GLIBC version of strerror_r
static const char *strerror_result(const char *a, const char *b) { (void)b; return a; }
#elif defined(HAVE_STRERROR_R)
// POSIX version of strerror_r
static const char *strerror_result(int a, const char *b) { (void)a; return b; }
#elif defined(HAVE_C__GENERIC)

// what a trick!
// http://stackoverflow.com/questions/479207/function-overloading-in-c
static const char *strerror_result_int(int a, const char *b) { (void)a; return b; }
static const char *strerror_result_string(const char *a, const char *b) { (void)b; return a; }

#define strerror_result(a, b) _Generic((a), \
    int: strerror_result_int, \
    char *: strerror_result_string \
    )(a, b)

#else
#error "cannot detect the format of function strerror_r()"
#endif

void error_limit_int(ERROR_LIMIT *erl, const char *prefix, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
    FILE *fp = stderror ? stderror : stderr;

    if(erl->sleep_ut)
        sleep_usec(erl->sleep_ut);

    // save a copy of errno - just in case this function generates a new error
    int __errno = errno;

    va_list args;

    nd_log_lock(ND_LOG_ERROR);

    erl->count++;
    time_t now = now_boottime_sec();
    if(now - erl->last_logged < erl->log_every) {
        nd_log_unlock(ND_LOG_ERROR);
        return;
    }

    // prevent logging too much
    if (error_log_limit(0)) {
        nd_log_unlock(ND_LOG_ERROR);
        return;
    }

    if(collector_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_ERR,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    va_start( args, fmt );
#ifdef NETDATA_INTERNAL_CHECKS
    fprintf(fp, "%s: %s %-5.5s : %s : (%04lu@%-20.20s:%-15.15s): ",
            date, program_name, prefix, netdata_thread_tag(), line, file, function);
#else
    fprintf(fp, "%s: %s %-5.5s : %s : ", date, program_name, prefix, netdata_thread_tag());
#endif
    vfprintf(fp, fmt, args );
    va_end( args );

    if(erl->count > 1)
        fprintf(fp, " (similar messages repeated %zu times in the last %llu secs)",
                erl->count, (unsigned long long)(erl->last_logged ? now - erl->last_logged : 0));

    if(erl->sleep_ut)
        fprintf(fp, " (sleeping for %"PRIu64" microseconds every time this happens)", erl->sleep_ut);

    if(__errno) {
        char buf[1024];
        fprintf(fp,
                " (errno %d, %s)\n", __errno, strerror_result(strerror_r(__errno, buf, 1023), buf));
        errno = 0;
    }
    else
        fputc('\n', fp);

    erl->last_logged = now;
    erl->count = 0;

    nd_log_unlock(ND_LOG_ERROR);
}

void error_int(int is_collector, const char *prefix, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
#if !defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_DEV_MODE)
    if (NETDATA_LOG_LEVEL_ERROR > global_log_severity_level)
        return;
#endif

    // save a copy of errno - just in case this function generates a new error
    int __errno = errno;
    FILE *fp = (is_collector || !stderror) ? stderr : stderror;

    va_list args;

    nd_log_lock(ND_LOG_ERROR);

    // prevent logging too much
    if (error_log_limit(0)) {
        nd_log_unlock(ND_LOG_ERROR);
        return;
    }

    if(collector_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_ERR,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    va_start( args, fmt );
#ifdef NETDATA_INTERNAL_CHECKS
    fprintf(fp, "%s: %s %-5.5s : %s : (%04lu@%-20.20s:%-15.15s): ",
            date, program_name, prefix, netdata_thread_tag(), line, file, function);
#else
    fprintf(fp, "%s: %s %-5.5s : %s : ", date, program_name, prefix, netdata_thread_tag());
#endif
    vfprintf(fp, fmt, args );
    va_end( args );

    if(__errno) {
        char buf[1024];
        fprintf(fp,
                " (errno %d, %s)\n", __errno, strerror_result(strerror_r(__errno, buf, 1023), buf));
        errno = 0;
    }
    else
        fputc('\n', fp);

    nd_log_unlock(ND_LOG_ERROR);
}

#ifdef NETDATA_INTERNAL_CHECKS
static void crash_netdata(void) {
    // make Netdata core dump
    abort();
}
#endif

#ifdef HAVE_BACKTRACE
#define BT_BUF_SIZE 100
static void print_call_stack(void) {
    FILE *fp = (!stderror) ? stderr : stderror;

    int nptrs;
    void *buffer[BT_BUF_SIZE];

    nptrs = backtrace(buffer, BT_BUF_SIZE);
    if(nptrs)
        backtrace_symbols_fd(buffer, nptrs, fileno(fp));
}
#endif

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    FILE *fp = stderror ? stderror : stderr;

    // save a copy of errno - just in case this function generates a new error
    int __errno = errno;
    va_list args;
    const char *thread_tag;
    char os_threadname[NETDATA_THREAD_NAME_MAX + 1];

    if(collector_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_CRIT,  fmt, args );
        va_end( args );
    }

    thread_tag = netdata_thread_tag();
    if (!netdata_thread_tag_exists()) {
        os_thread_get_current_name_np(os_threadname);
        if ('\0' != os_threadname[0]) { /* If it is not an empty string replace "MAIN" thread_tag */
            thread_tag = os_threadname;
        }
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    nd_log_lock(ND_LOG_ERROR);

    va_start( args, fmt );
#ifdef NETDATA_INTERNAL_CHECKS
    fprintf(fp,
            "%s: %s FATAL : %s : (%04lu@%-20.20s:%-15.15s): ", date, program_name, thread_tag, line, file, function);
#else
    fprintf(fp, "%s: %s FATAL : %s : ", date, program_name, thread_tag);
#endif
    vfprintf(fp, fmt, args );
    va_end( args );

    perror(" # ");
    fputc('\n', fp);

    nd_log_unlock(ND_LOG_ERROR);

    char action_data[70+1];
    snprintfz(action_data, 70, "%04lu@%-10.10s:%-15.15s/%d", line, file, function, __errno);
    char action_result[60+1];

    const char *tag_to_send =  thread_tag;

    // anonymize thread names
    if(strncmp(thread_tag, THREAD_TAG_STREAM_RECEIVER, strlen(THREAD_TAG_STREAM_RECEIVER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_RECEIVER;
    if(strncmp(thread_tag, THREAD_TAG_STREAM_SENDER, strlen(THREAD_TAG_STREAM_SENDER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_SENDER;

    snprintfz(action_result, 60, "%s:%s", program_name, tag_to_send);
    send_statistics("FATAL", action_result, action_data);

#ifdef HAVE_BACKTRACE
    print_call_stack();
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    crash_netdata();
#endif

    netdata_cleanup_and_exit(1);
}

// ----------------------------------------------------------------------------
// access log

void netdata_log_access( const char *fmt, ... ) {
    va_list args;

    if(access_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_INFO,  fmt, args );
        va_end( args );
    }

    if(stdaccess) {
        nd_log_lock(ND_LOG_ACCESS);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(stdaccess, "%s: ", date);

        va_start( args, fmt );
        vfprintf( stdaccess, fmt, args );
        va_end( args );
        fputc('\n', stdaccess);

        nd_log_unlock(ND_LOG_ACCESS);
    }
}

// ----------------------------------------------------------------------------
// health log

void netdata_log_health( const char *fmt, ... ) {
    va_list args;

    if(health_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_INFO,  fmt, args );
        va_end( args );
    }

    if(stdhealth) {
        nd_log_lock(ND_LOG_HEALTH);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(stdhealth, "%s: ", date);

        va_start( args, fmt );
        vfprintf( stdhealth, fmt, args );
        va_end( args );
        fputc('\n', stdhealth);

        nd_log_unlock(ND_LOG_HEALTH);
    }
}

#ifdef ENABLE_ACLK
void log_aclk_message_bin( const char *data, const size_t data_len, int tx, const char *mqtt_topic, const char *message_name) {
    if (aclklog) {
        nd_log_lock(ND_LOG_ACLK);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(aclklog, "%s: %s Msg:\"%s\", MQTT-topic:\"%s\": ", date, tx ? "OUTGOING" : "INCOMING", message_name, mqtt_topic);

        fwrite(data, data_len, 1, aclklog);

        fputc('\n', aclklog);

        nd_log_unlock(ND_LOG_ACLK);
    }
}
#endif

void log_set_global_severity_level(netdata_log_level_t value)
{
    global_log_severity_level = value;
}

netdata_log_level_t log_severity_string_to_severity_level(char *level)
{
    if (!strcmp(level, NETDATA_LOG_LEVEL_INFO_STR))
        return NETDATA_LOG_LEVEL_INFO;
    if (!strcmp(level, NETDATA_LOG_LEVEL_ERROR_STR) || !strcmp(level, NETDATA_LOG_LEVEL_ERROR_SHORT_STR))
        return NETDATA_LOG_LEVEL_ERROR;

    return NETDATA_LOG_LEVEL_INFO;
}

char *log_severity_level_to_severity_string(netdata_log_level_t level)
{
    switch (level) {
        case NETDATA_LOG_LEVEL_ERROR:
            return NETDATA_LOG_LEVEL_ERROR_STR;
        case NETDATA_LOG_LEVEL_INFO:
        default:
            return NETDATA_LOG_LEVEL_INFO_STR;
    }
}

void log_set_global_severity_for_external_plugins() {
    char *s = getenv("NETDATA_LOG_SEVERITY_LEVEL");
    if (!s)
        return;
    netdata_log_level_t level = log_severity_string_to_severity_level(s);
    log_set_global_severity_level(level);
}
