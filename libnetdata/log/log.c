// SPDX-License-Identifier: GPL-3.0-or-later

#define SD_JOURNAL_SUPPRESS_LOCATION

#include "../libnetdata.h"
#include <daemon/main.h>

#ifdef HAVE_BACKTRACE
#include <execinfo.h>
#endif

#ifdef HAVE_SYSTEMD
#include <systemd/sd-journal.h>
#endif

#include <syslog.h>

const char *program_name = "";

// ----------------------------------------------------------------------------
// workaround strerror_r()

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

static const char *errno2str(int errnum, char *buf, size_t size) {
    return strerror_result(strerror_r(errnum, buf, size), buf);
}

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
// format dates

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

struct nd_log_limit {
    usec_t started_monotonic_ut;
    uint32_t counter;
    uint32_t prevented;
    bool unlimited;
};

#define ND_LOG_LIMITS_DEFAULT { .unlimited = false, }
#define ND_LOG_LIMITS_UNLIMITED { .unlimited = true, }

struct nd_log_source {
    SPINLOCK spinlock;
    ND_LOG_METHOD method;
    const char *filename;
    int fd;
    FILE *fp;
    FILE **fp_set;

    struct nd_log_limit limits;
};

static struct {
    struct nd_log_source sources[_NDLS_MAX];

    struct {
        bool initialized;
    } journal;

    struct {
        bool initialized;
        int facility;
    } syslog;

    struct {
        bool initialized;
    } stdin;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } stdout;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } stderr;

    struct {
        unsigned long throttle_period;
        unsigned long logs_per_period;
        unsigned long logs_per_period_backup;
    } limits;

} nd_log = {
        .limits = {
                .throttle_period = 1200,
                .logs_per_period = 200,
                .logs_per_period_backup = 200,
        },
        .journal = {
                .initialized = false,
        },
        .syslog = {
                .initialized = false,
                .facility = LOG_DAEMON,
        },
        .stdin = {
                .initialized = false,
        },
        .stdout = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .initialized = false,
        },
        .stderr = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .initialized = false,
        },
        .sources = {
                [NDLS_INPUT] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEVNULL,
                        .filename = "/dev/null",
                        .fd = STDIN_FILENO,
                        .fp_set = &stdin,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACCESS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/access.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACLK] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_FILE,
                        .filename = LOG_DIR "/aclk.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_COLLECTORS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/collectors.log",
                        .fd = STDERR_FILENO,
                        .fp_set = &stderr,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_DEBUG] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DISABLED,
                        .filename = LOG_DIR "/debug.log",
                        .fd = STDOUT_FILENO,
                        .fp_set = &stdout,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_DAEMON] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/error.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_HEALTH] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/health.log",
                        .fd = -1,
                        .fp_set = NULL,
                        .fp = NULL,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
        },
};

static inline void nd_log_lock(ND_LOG_SOURCES type) {
    spinlock_lock(&nd_log.sources[type].spinlock);
}

static inline void nd_log_unlock(ND_LOG_SOURCES type) {
    spinlock_unlock(&nd_log.sources[type].spinlock);
}

void nd_log_set_destination_output(ND_LOG_SOURCES type, const char *setting) {
    if(!setting || !*setting || strcmp(setting, "none") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_DISABLED;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "journal") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_JOURNAL;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "syslog") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_SYSLOG;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "/dev/null") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_DEVNULL;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "system") == 0) {
        if(nd_log.sources[type].fp_set == &stderr) {
            nd_log.sources[type].method = ND_LOG_METHOD_STDERR;
            nd_log.sources[type].filename = "stderr";
            nd_log.sources[type].fd = STDERR_FILENO;
        }
        else {
            nd_log.sources[type].method = ND_LOG_METHOD_STDOUT;
            nd_log.sources[type].filename = "stdout";
            nd_log.sources[type].fd = STDOUT_FILENO;
        }
    }
    else if(strcmp(setting, "stderr") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_STDERR;
        nd_log.sources[type].filename = "stderr";
        nd_log.sources[type].fd = STDERR_FILENO;
    }
    else if(strcmp(setting, "stdout") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_STDOUT;
        nd_log.sources[type].filename = "stdout";
        nd_log.sources[type].fd = STDOUT_FILENO;
    }
    else {
        nd_log.sources[type].method = ND_LOG_METHOD_FILE;
        nd_log.sources[type].filename = setting;
    }
}

void nd_log_set_facility(const char *facility) {
    nd_log.syslog.facility = nd_log_facility2id(facility);
}

void nd_log_set_flood_protection(time_t period, size_t logs) {
    nd_log.limits.throttle_period = period;
    nd_log.limits.logs_per_period = nd_log.limits.logs_per_period_backup = logs;
}

static void nd_log_syslog_init() {
    if(nd_log.syslog.initialized)
        return;

    openlog(program_name, LOG_PID, nd_log.syslog.facility);
    nd_log.syslog.initialized = true;
}

static bool nd_log_set_system_fd(struct nd_log_source *e, int new_fd) {
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

static void nd_log_open(struct nd_log_source *e, ND_LOG_SOURCES source) {
    if(e->method == ND_LOG_METHOD_DEFAULT)
        nd_log_set_destination_output(source, e->filename);

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
            nd_log.journal.initialized = true;
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
    for(size_t i = 0 ; i < _NDLS_MAX ; i++)
        nd_log_open(&nd_log.sources[i], i);
}

void nd_log_reopen_log_files(void) {
    netdata_log_info("Reopening all log files.");

    nd_log.stdout.initialized = false;
    nd_log.stderr.initialized = false;
    nd_log_initialize();

    netdata_log_info("Log files re-opened.");
}

void chown_open_file(int fd, uid_t uid, gid_t gid) {
    if(fd == -1) return;

    struct stat buf;

    if(fstat(fd, &buf) == -1) {
        netdata_log_error("Cannot fstat() fd %d", fd);
        return;
    }

    if((buf.st_uid != uid || buf.st_gid != gid) && S_ISREG(buf.st_mode)) {
        if(fchown(fd, uid, gid) == -1)
            netdata_log_error("Cannot fchown() fd %d.", fd);
    }
}

void nd_log_chown_log_files(uid_t uid, gid_t gid) {
    for(size_t i = 0 ; i < _NDLS_MAX ; i++) {
        if(nd_log.sources[i].fd != -1 && nd_log.sources[i].fd != STDIN_FILENO)
            chown_open_file(nd_log.sources[i].fd, uid, gid);
    }
}

// ----------------------------------------------------------------------------
// log limits

void nd_log_limits_reset(void) {
    nd_log.limits.logs_per_period = nd_log.limits.logs_per_period_backup;

    spinlock_lock(&nd_log.stdout.spinlock);
    spinlock_lock(&nd_log.stderr.spinlock);

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        spinlock_lock(&nd_log.sources[i].spinlock);
        nd_log.sources[i].limits.prevented = 0;
        nd_log.sources[i].limits.counter = 0;
        nd_log.sources[i].limits.started_monotonic_ut = 0;
        spinlock_unlock(&nd_log.sources[i].spinlock);
    }

    spinlock_unlock(&nd_log.stdout.spinlock);
    spinlock_unlock(&nd_log.stderr.spinlock);
}

void nd_log_limits_unlimited(void) {
    nd_log_limits_reset();
    nd_log.limits.logs_per_period = ((nd_log.limits.logs_per_period_backup * 10) < 10000) ? 10000 : (nd_log.limits.logs_per_period_backup * 10);
}

bool nd_log_limit_reached(struct nd_log_source *source, bool reset, FILE *fp) {
    // do not throttle if the period is 0
    if(nd_log.limits.throttle_period == 0)
        return 0;

    // prevent all logs if the errors per period is 0
    if(nd_log.limits.logs_per_period == 0)
#ifdef NETDATA_INTERNAL_CHECKS
        return false;
#else
        return true;
#endif

    if(!fp)
        fp = stderr;

    usec_t now_ut = now_monotonic_usec();
    if(!source->limits.started_monotonic_ut)
        source->limits.started_monotonic_ut = now_ut;

    if(reset) {
        if(source->limits.prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                    fp,
                    "%s: %s LOG FLOOD PROTECTION reset for process '%s' "
                    "(prevented %lu logs in the last %"PRId64" seconds).\n",
                    date,
                    program_name,
                    program_name,
                    source->limits.prevented,
                    (int64_t)((now_ut - source->limits.started_monotonic_ut) / USEC_PER_SEC));
        }

        source->limits.started_monotonic_ut = now_ut;
        source->limits.counter = 0;
        source->limits.prevented = 0;
    }

    // detect if we log too much
    source->limits.counter++;

    if(now_ut - source->limits.started_monotonic_ut > nd_log.limits.throttle_period) {
        if(source->limits.prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                    fp,
                    "%s: %s LOG FLOOD PROTECTION resuming logging from process '%s' "
                    "(prevented %lu logs in the last %"PRId64" seconds).\n",
                    date,
                    program_name,
                    program_name,
                    source->limits.prevented,
                    (int64_t)nd_log.limits.throttle_period);
        }

        // restart the period accounting
        source->limits.started_monotonic_ut = now_ut;
        source->limits.counter = 1;
        source->limits.prevented = 0;

        // log this error
        return false;
    }

    if(source->limits.counter > nd_log.limits.logs_per_period) {
        if(!source->limits.prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
            fprintf(
                    fp,
                    "%s: %s LOG FLOOD PROTECTION too many logs (%lu logs in %"PRId64" seconds, threshold is set to %lu logs "
                    "in %"PRId64" seconds). Preventing more logs from process '%s' for %"PRId64" seconds.\n",
                    date,
                    program_name,
                    source->limits.counter,
                    (int64_t)((now_ut - source->limits.started_monotonic_ut) / USEC_PER_SEC),
                    nd_log.limits.logs_per_period,
                    (int64_t)nd_log.limits.throttle_period,
                    program_name,
                    (int64_t)((source->limits.started_monotonic_ut + (nd_log.limits.throttle_period * USEC_PER_SEC) - now_ut)) / USEC_PER_SEC);
        }

        source->limits.prevented++;

        // prevent logging this error
#ifdef NETDATA_INTERNAL_CHECKS
        return false;
#else
        return true;
#endif
    }

    return false;
}

// ----------------------------------------------------------------------------
// annotators

struct log_field;
static void errno_annotator(BUFFER *wb, struct log_field *lf);
static void priority_annotator(BUFFER *wb, struct log_field *lf);

// ----------------------------------------------------------------------------

typedef void (*annotator_t)(BUFFER *wb, struct log_field *lf);

struct log_field {
    const char *journal;
    const char *logfmt;
    annotator_t logfmt_annotator;
    struct log_stack_entry entry;
};

static __thread struct log_stack_entry *thread_log_stack_base = NULL;
static __thread struct log_field thread_log_fields_daemon[_NDF_MAX] = {
        [NDF_SYSLOG_IDENTIFIER] = {
                .journal = "SYSLOG_IDENTIFIER",
                .logfmt = "app",
        },
        [NDF_TIMESTAMP_REALTIME_USEC] = {
                .journal = NULL,
                .logfmt = "ts",
        },
        [NDF_LINE] = {
                .journal = "CODE_LINE",
                .logfmt = "line",
        },
        [NDF_FILE] = {
                .journal = "CODE_FILE",
                .logfmt = "file",
        },
        [NDF_FUNC] = {
                .journal = "CODE_FUNC",
                .logfmt = "func",
        },
        [NDF_ERRNO] = {
                .journal = "ERRNO",
                .logfmt = "errno",
                .logfmt_annotator = errno_annotator,
        },
        [NDF_PRIORITY] = {
                .journal = "PRIORITY",
                .logfmt = "prio",
                .logfmt_annotator = priority_annotator,
        },
        [NDF_SESSION] = {
                .journal = "ND_SESSION",
                .logfmt = NULL,
        },
        [NDF_TID] = {
                .journal = "TID",
                .logfmt = "tid",
        },
        [NDF_THREAD] = {
                .journal = "THREAD_TAG",
                .logfmt = "th",
        },
        [NDF_PLUGIN] = {
                .journal = "ND_PLUGIN",
                .logfmt = "plugin",
        },
        [NDF_MODULE] = {
                .journal = "ND_MODULE",
                .logfmt = "module",
        },
        [NDF_JOB] = {
                .journal = "ND_JOB",
                .logfmt = "job",
        },
        [NDF_NIDL_NODE] = {
                .journal = "NIDL_NODE",
                .logfmt = "host",
        },
        [NDF_NIDL_INSTANCE] = {
                .journal = "NIDL_INSTANCE",
                .logfmt = "st",
        },
        [NDF_NIDL_DIMENSION] = {
                .journal = "NIDL_DIMENSION",
                .logfmt = "rd",
        },
        [NDF_MESSAGE] = {
                .journal = "MESSAGE",
                .logfmt = "msg",
        },
};

void log_stack_pop(void **ptr) {
    struct log_stack_entry *lgs = (struct log_stack_entry *)(*ptr);

    if(!lgs || !lgs->prev || !lgs->next)
        return;

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(thread_log_stack_base, lgs, prev, next);
}

void log_stack_push(struct log_stack_entry *lgs) {
    if(!lgs)
        return;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(thread_log_stack_base, lgs, prev, next);
}

static bool nd_logger_journal(struct log_field *fields, size_t fields_max) {
#ifdef HAVE_SYSTEMD
    struct iovec iov[fields_max];
    int iov_count = 0;

    memset(iov, 0, sizeof(iov));

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].journal)
            continue;

        const char *key = fields[i].journal;
        char *value = NULL;
        switch (fields[i].entry.type) {
            case NDFT_TXT:
                asprintf(&value, "%s=%s", key, fields[i].entry.str);
                break;
            case NDFT_U32:
                asprintf(&value, "%s=%u", key, fields[i].entry.u32);
                break;
            case NDFT_I32:
                asprintf(&value, "%s=%d", key, fields[i].entry.i32);
                break;
            case NDFT_U64:
            case NDFT_TIMESTAMP:
                asprintf(&value, "%s=%" PRIu64, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                asprintf(&value, "%s=%" PRId64, key, fields[i].entry.i64);
                break;
            case NDFT_PRIORITY:
                asprintf(&value, "%s=%d", key, (int)fields[i].entry.priority);
                break;
            default:
                // Handle other types as needed
                break;
        }

        if (value) {
            iov[iov_count].iov_base = value;
            iov[iov_count].iov_len = strlen(value);
            iov_count++;
        }
    }

    int r = sd_journal_sendv(iov, iov_count);

    // Clean up allocated memory
    for (int i = 0; i < iov_count; i++) {
        if (iov[i].iov_base != NULL) {
            free(iov[i].iov_base);
        }
    }

    return r == 0;
#else
    return false;
#endif
}

static void errno_annotator(BUFFER *wb, struct log_field *lf) {
    char buf[1024];
    const char *s = errno2str(lf->entry.i32, buf, sizeof(buf));

    buffer_fast_strcat(wb, "\"", 1);
    buffer_print_int64(wb, lf->entry.i32);
    buffer_fast_strcat(wb, ", ", 2);
    buffer_json_strcat(wb, s);
    buffer_fast_strcat(wb, "\"", 1);
}

static void priority_annotator(BUFFER *wb, struct log_field *lf) {
    static char *priorities[] = {
            [NDLP_ALERT] = "ALERT",
            [NDLP_CRIT] = "CRITICAL",
            [NDLP_EMERG] = "EMERGENCY",
            [NDLP_ERR] = "ERROR",
            [NDLP_WARNING] = "WARNING",
            [NDLP_INFO] = "INFO",
            [NDLP_NOTICE] = "NOTICE",
            [NDLP_DEBUG] = "DEBUG",
    };

    size_t entries = sizeof(priorities) / sizeof(priorities[0]);
    if(lf->entry.u64 < entries)
        buffer_strcat(wb, priorities[lf->entry.u64]);
    else
        buffer_print_uint64(wb, lf->entry.u64);
}

static void nd_logger_logfmt(BUFFER *wb, struct log_field *fields, size_t fields_max) {
    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        if(buffer_strlen(wb))
            buffer_fast_strcat(wb, " ", 1);

        const char *key = fields[i].logfmt;
        buffer_strcat(wb, key);
        buffer_fast_strcat(wb, "=", 1);

        if(fields[i].logfmt_annotator)
            fields[i].logfmt_annotator(wb, &fields[i]);
        else {
            switch(fields[i].entry.type) {
                case NDFT_TXT:
                    buffer_fast_strcat(wb, "\"", 1);
                    buffer_json_strcat(wb, fields[i].entry.str);
                    buffer_fast_strcat(wb, "\"", 1);
                    break;
                case NDFT_U32:
                    buffer_print_uint64(wb, fields[i].entry.u32);
                    break;
                case NDFT_I32:
                    buffer_print_int64(wb, fields[i].entry.i32);
                    break;
                case NDFT_U64:
                case NDFT_TIMESTAMP:
                    buffer_print_uint64(wb, fields[i].entry.u64);
                    break;
                case NDFT_I64:
                    buffer_print_int64(wb, fields[i].entry.i64);
                    break;
                case NDFT_PRIORITY:
                    buffer_print_uint64(wb, fields[i].entry.priority);
                    break;
                default:
                    // Handle other types as needed
                    break;
            }
        }
    }
}

static bool nd_logger_syslog(int priority, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(256, NULL);

    nd_logger_logfmt(wb, fields, fields_max);
    syslog(priority, "%s", buffer_tostring(wb));

    buffer_free(wb);
    return true;
}

static bool nd_logger_file(FILE *fp, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(256, NULL);

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
    buffer_strcat(wb, date);
    buffer_fast_strcat(wb, ":", 1);

    nd_logger_logfmt(wb, fields, fields_max);
    int r = fprintf(fp, "%s\n", buffer_tostring(wb));

    buffer_free(wb);
    return r > 0;
}

static ND_LOG_METHOD nd_logger_select_method(ND_LOG_SOURCES source, FILE **fpp, SPINLOCK **spinlock) {
    *spinlock = NULL;
    ND_LOG_METHOD method = nd_log.sources[source].method;

    switch(method) {
#ifdef HAVE_SYSTEMD
        case ND_LOG_METHOD_JOURNAL:
            if(unlikely(!nd_log.journal.initialized)) {
                method = ND_LOG_METHOD_FILE;
                *fpp = stderr;
                *spinlock = &nd_log.stderr.spinlock;
            }
            else {
                *fpp = NULL;
                *spinlock = NULL;
            }
            break;
#endif

        case ND_LOG_METHOD_SYSLOG:
            if(unlikely(!nd_log.syslog.initialized)) {
                method = ND_LOG_METHOD_FILE;
                *spinlock = &nd_log.stderr.spinlock;
                *fpp = stderr;
            }
            else {
                *spinlock = NULL;
                *fpp = NULL;
            }
            break;

        case ND_LOG_METHOD_FILE:
            if(!nd_log.sources[source].fp) {
                *fpp = stderr;
                *spinlock = &nd_log.stderr.spinlock;
            }
            else {
                *fpp = nd_log.sources[source].fp;
                *spinlock = &nd_log.sources[source].spinlock;
            }
            break;

        case ND_LOG_METHOD_STDOUT:
            method = ND_LOG_METHOD_FILE;
            *fpp = stdout;
            *spinlock = &nd_log.stdout.spinlock;
            break;

        default:
        case ND_LOG_METHOD_DEFAULT:
        case ND_LOG_METHOD_STDERR:
            method = ND_LOG_METHOD_FILE;
            *fpp = stderr;
            *spinlock = &nd_log.stderr.spinlock;
            break;

        case ND_LOG_METHOD_DISABLED:
        case ND_LOG_METHOD_DEVNULL:
            method = ND_LOG_METHOD_DISABLED;
            *fpp = NULL;
            *spinlock = NULL;
            break;
    }

    return method;
}

static void nd_logger(const char *file, const char *function, const unsigned long line,
               ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, bool limit,
               const char *fmt, va_list ap) {

    SPINLOCK *spinlock;
    FILE *fp;
    ND_LOG_METHOD method = nd_logger_select_method(source, &fp, &spinlock);
    if(method != ND_LOG_METHOD_FILE && method != ND_LOG_METHOD_JOURNAL && method != ND_LOG_METHOD_SYSLOG)
        return;

    // mark all fields as unset
    size_t fields_max = sizeof(thread_log_fields_daemon) / sizeof(thread_log_fields_daemon[0]);
    for(size_t i = 0; i < fields_max ; i++)
        thread_log_fields_daemon[i].entry.set = false;

    // flatten the log stack into the fields
    for(struct log_stack_entry *l = thread_log_stack_base; l ; l = l->next) {
        for(size_t i = 0; l[i].id != NDF_STOP ;i++) {
            if(l[i].id >= _NDF_MAX)
                continue;

            thread_log_fields_daemon[l[i].id].entry = l[i];
            thread_log_fields_daemon[l[i].id].entry.set = true;
        }
    }

    // set the common fields that are automatically set by the logging subsystem

    if(!thread_log_fields_daemon[NDF_SYSLOG_IDENTIFIER].entry.set) {
        thread_log_fields_daemon[NDF_SYSLOG_IDENTIFIER].entry = ND_LOG_FIELD_STR(NDF_SYSLOG_IDENTIFIER, program_name);
    }

    if(!thread_log_fields_daemon[NDF_LINE].entry.set) {
        thread_log_fields_daemon[NDF_LINE].entry = ND_LOG_FIELD_U64(NDF_LINE, line);
        thread_log_fields_daemon[NDF_FILE].entry = ND_LOG_FIELD_STR(NDF_FILE, file);
        thread_log_fields_daemon[NDF_FUNC].entry = ND_LOG_FIELD_STR(NDF_FUNC, function);
    }

    if(!thread_log_fields_daemon[NDF_PRIORITY].entry.set) {
        thread_log_fields_daemon[NDF_PRIORITY].entry = ND_LOG_FIELD_U64(NDF_PRIORITY, priority);
    }

    if(!thread_log_fields_daemon[NDF_TID].entry.set) {
        thread_log_fields_daemon[NDF_TID].entry = ND_LOG_FIELD_U64(NDF_TID, gettid());
        thread_log_fields_daemon[NDF_THREAD].entry = ND_LOG_FIELD_STR(NDF_THREAD, netdata_thread_tag());
    }

    if(!thread_log_fields_daemon[NDF_TIMESTAMP_REALTIME_USEC].entry.set) {
        thread_log_fields_daemon[NDF_TIMESTAMP_REALTIME_USEC].entry = ND_LOG_FIELD_TMT(NDF_TIMESTAMP_REALTIME_USEC, now_realtime_usec());
    }

    if(!thread_log_fields_daemon[NDF_ERRNO].entry.set) {
        thread_log_fields_daemon[NDF_ERRNO].entry = ND_LOG_FIELD_I32(NDF_ERRNO, errno);
    }

    BUFFER *wb = buffer_create(0, NULL);
    buffer_vsprintf(wb, fmt, ap);
    thread_log_fields_daemon[NDF_MESSAGE].entry = ND_LOG_FIELD_STR(NDF_MESSAGE, buffer_tostring(wb));

    if(spinlock)
        spinlock_lock(spinlock);

    // check the limits
    if(limit && nd_log_limit_reached(&nd_log.sources[source], false, fp))
        goto cleanup;

    if(method == ND_LOG_METHOD_JOURNAL) {
        if(!nd_logger_journal(thread_log_fields_daemon, fields_max)) {
            // we can't log to journal, let's log to stderr
            method = ND_LOG_METHOD_FILE;
            spinlock = &nd_log.stderr.spinlock;
            fp = stderr;
        }
    }

    if(method == ND_LOG_METHOD_SYSLOG)
        nd_logger_syslog(priority, thread_log_fields_daemon, fields_max);

    if(method == ND_LOG_METHOD_FILE)
        nd_logger_file(fp, thread_log_fields_daemon, fields_max);


cleanup:
    if(spinlock)
        spinlock_unlock(spinlock);

    buffer_free(wb);

    errno = 0;
}




// ----------------------------------------------------------------------------

uint64_t debug_flags = 0;

int access_log_syslog = 0;
int error_log_syslog = 0;
int collector_log_syslog = 0;
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
// error log

void netdata_logger(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) {
#if !defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_DEV_MODE)
    if (NETDATA_LOG_LEVEL_ERROR > global_log_severity_level)
        return;
#endif

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority, true, fmt, args);
    va_end(args);
}

void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
    if(erl->sleep_ut)
        sleep_usec(erl->sleep_ut);

    spinlock_lock(&erl->spinlock);

    erl->count++;
    time_t now = now_boottime_sec();
    if(now - erl->last_logged < erl->log_every) {
        spinlock_unlock(&erl->spinlock);
        return;
    }

    spinlock_unlock(&erl->spinlock);

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority, true, fmt, args);
    va_end(args);
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

    nd_log_lock(NDLS_DAEMON);

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

    nd_log_unlock(NDLS_DAEMON);

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
        nd_log_lock(NDLS_ACCESS);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(stdaccess, "%s: ", date);

        va_start( args, fmt );
        vfprintf( stdaccess, fmt, args );
        va_end( args );
        fputc('\n', stdaccess);

        nd_log_unlock(NDLS_ACCESS);
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
        nd_log_lock(NDLS_HEALTH);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(stdhealth, "%s: ", date);

        va_start( args, fmt );
        vfprintf( stdhealth, fmt, args );
        va_end( args );
        fputc('\n', stdhealth);

        nd_log_unlock(NDLS_HEALTH);
    }
}

#ifdef ENABLE_ACLK
void log_aclk_message_bin( const char *data, const size_t data_len, int tx, const char *mqtt_topic, const char *message_name) {
    if (aclklog) {
        nd_log_lock(NDLS_ACLK);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH, now_realtime_sec());
        fprintf(aclklog, "%s: %s Msg:\"%s\", MQTT-topic:\"%s\": ", date, tx ? "OUTGOING" : "INCOMING", message_name, mqtt_topic);

        fwrite(data, data_len, 1, aclklog);

        fputc('\n', aclklog);

        nd_log_unlock(NDLS_ACLK);
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
