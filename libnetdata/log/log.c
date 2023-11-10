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

uint64_t debug_flags = 0;

#ifdef ENABLE_ACLK
int aclklog_enabled = 0;
#endif

// ----------------------------------------------------------------------------

typedef enum  __attribute__((__packed__)) {
    ND_LOG_METHOD_DISABLED,
    ND_LOG_METHOD_DEVNULL,
    ND_LOG_METHOD_DEFAULT,
    ND_LOG_METHOD_JOURNAL,
    ND_LOG_METHOD_SYSLOG,
    ND_LOG_METHOD_STDOUT,
    ND_LOG_METHOD_STDERR,
    ND_LOG_METHOD_FILE,
} ND_LOG_METHOD;

struct nd_log_source;
static bool nd_log_limit_reached(struct nd_log_source *source);

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
} nd_log_facilities[] = {
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
    size_t entries = sizeof(nd_log_facilities) / sizeof(nd_log_facilities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_facilities[i].name, facility) == 0)
            return nd_log_facilities[i].facility;
    }

    return LOG_DAEMON;
}

static const char *nd_log_id2facility(int facility) {
    size_t entries = sizeof(nd_log_facilities) / sizeof(nd_log_facilities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(nd_log_facilities[i].facility == facility)
            return nd_log_facilities[i].name;
    }

    return "daemon";
}

// ----------------------------------------------------------------------------
// priorities

static struct {
    ND_LOG_FIELD_PRIORITY priority;
    const char *name;
} nd_log_priorities[] = {
        { .priority = NDLP_EMERG, .name = "emergency" },
        { .priority = NDLP_EMERG, .name = "emerg" },
        { .priority = NDLP_ALERT, .name = "alert" },
        { .priority = NDLP_CRIT, .name = "critical" },
        { .priority = NDLP_CRIT, .name = "crit" },
        { .priority = NDLP_ERR, .name = "error" },
        { .priority = NDLP_ERR, .name = "err" },
        { .priority = NDLP_WARNING, .name = "warning" },
        { .priority = NDLP_WARNING, .name = "warn" },
        { .priority = NDLP_NOTICE, .name = "notice" },
        { .priority = NDLP_INFO, .name = NDLP_INFO_STR },
        { .priority = NDLP_DEBUG, .name = "debug" },
};

static int nd_log_priority2id(const char *priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_priorities[i].name, priority) == 0)
            return nd_log_priorities[i].priority;
    }

    return NDLP_INFO;
}

static const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(priority == nd_log_priorities[i].priority)
            return nd_log_priorities[i].name;
    }

    return NDLP_INFO_STR;
}

// ----------------------------------------------------------------------------
// log sources

const char *log_sources_str[] = {
        [NDLS_UNSET] = "UNSET",
        [NDLS_ACCESS] = "ACCESS",
        [NDLS_ACLK] = "ACLK",
        [NDLS_COLLECTORS] = "COLLECTORS",
        [NDLS_DAEMON] = "DAEMON",
        [NDLS_HEALTH] = "HEALTH",
        [NDLS_DEBUG] = "DEBUG",
};

static const char *nd_log_source2str(ND_LOG_SOURCES source) {
    size_t entries = sizeof(log_sources_str) / sizeof(log_sources_str[0]);
    if(source < entries)
        return log_sources_str[source];

    return "UNKNOWN";
}

// ----------------------------------------------------------------------------
// format dates

void log_date(char *buffer, size_t len, time_t now) {
    if(unlikely(!buffer || !len))
        return;

    time_t t = now;
    struct tm *tmp, tmbuf;

    tmp = localtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return;
    }

    if (unlikely(strftime(buffer, len, "%Y-%m-%d %H:%M:%S", tmp) == 0))
        buffer[0] = '\0';

    buffer[len - 1] = '\0';
}

#define ISO8601_MAX_LENGTH 64

void iso8601_datetime_utc(char *buffer, size_t len, usec_t now_ut) {
    if(unlikely(!buffer || !len))
        return;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    // Use gmtime_r for UTC time conversion.
    tmp = gmtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return;
    }

    // Format the date and time according to the ISO 8601 format with a 'Z' designator for UTC.
    if (unlikely(strftime(buffer, len, "%Y-%m-%dT%H:%M:%SZ", tmp) == 0))
        buffer[0] = '\0';

    buffer[len - 1] = '\0';
}

void iso8601_datetime_with_local_timezone(char *buffer, size_t len, usec_t now_ut) {
    if(unlikely(!buffer || len == 0))
        return;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    // Use localtime_r for local time conversion.
    tmp = localtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return;
    }

    // Format the date and time according to the ISO 8601 format.
    size_t used_length = strftime(buffer, len, "%Y-%m-%dT%H:%M:%S", tmp);
    if (unlikely(used_length == 0)) {
        buffer[0] = '\0';
        return;
    }

    // Calculate the timezone offset in hours and minutes from UTC.
    long offset = tmbuf.tm_gmtoff;
    int hours = (int)(offset / 3600); // Convert offset seconds to hours.
    int minutes = (int)((offset % 3600) / 60); // Convert remainder to minutes (keep the sign for minutes).

    // Check if timezone is UTC.
    if (hours == 0 && minutes == 0) {
        // For UTC, append 'Z' to the timestamp.
        if (used_length + 1 < len) {
            buffer[used_length] = 'Z';
            buffer[used_length + 1] = '\0'; // null-terminate the string.
        }
    }
    else {
        // For non-UTC, format the timezone offset. Omit minutes if they are zero.
        if (minutes == 0) {
            // Check enough space is available for the timezone offset string.
            if (used_length + 3 < len) // "+hh\0"
                snprintf(buffer + used_length, len - used_length, "%+03d", hours);
        }
        else {
            // Check enough space is available for the timezone offset string.
            if (used_length + 6 < len) // "+hh:mm\0"
                snprintf(buffer + used_length, len - used_length, "%+03d:%02d", hours, abs(minutes));
        }
    }
}

// ----------------------------------------------------------------------------

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

    ND_LOG_FIELD_PRIORITY min_priority;
    const char *pending_msg;
    struct nd_log_limit limits;
};

static __thread ND_LOG_SOURCES overwrite_thread_source = 0;

void nd_log_set_thread_source(ND_LOG_SOURCES source) {
    overwrite_thread_source = source;
}

static struct {
    ND_LOG_SOURCES overwrite_process_source;

    struct nd_log_source sources[_NDLS_MAX];

    struct {
        bool initialized;
    } journal;

    struct {
        bool initialized;
        int facility;
    } syslog;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } std_output;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } std_error;

    struct {
        uint32_t throttle_period;
        uint32_t logs_per_period;
        uint32_t logs_per_period_backup;
    } limits;

} nd_log = {
        .overwrite_process_source = 0,
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
        .std_output = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .initialized = false,
        },
        .std_error = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .initialized = false,
        },
        .sources = {
                [NDLS_UNSET] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DISABLED,
                        .filename = NULL,
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_EMERG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACCESS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/access.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACLK] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_FILE,
                        .filename = LOG_DIR "/aclk.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_COLLECTORS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/collectors.log",
                        .fd = STDERR_FILENO,
                        .fp = NULL,
                        .min_priority = NDLP_INFO,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_DEBUG] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DISABLED,
                        .filename = LOG_DIR "/debug.log",
                        .fd = STDOUT_FILENO,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_DAEMON] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/error.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_INFO,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_HEALTH] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = ND_LOG_METHOD_DEFAULT,
                        .filename = LOG_DIR "/health.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
        },
};

void nd_log_set_destination_output(ND_LOG_SOURCES type, const char *setting) {
    if(!setting || !*setting || strcmp(setting, "none") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_DISABLED;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "journal") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_JOURNAL;
        nd_log.sources[type].filename = NULL;
    }
    else if(strcmp(setting, "syslog") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_SYSLOG;
        nd_log.sources[type].filename = NULL;
    }
    else if(strcmp(setting, "/dev/null") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_DEVNULL;
        nd_log.sources[type].filename = "/dev/null";
    }
    else if(strcmp(setting, "system") == 0) {
        if(nd_log.sources[type].fd == STDERR_FILENO) {
            nd_log.sources[type].method = ND_LOG_METHOD_STDERR;
            nd_log.sources[type].filename = NULL;
            nd_log.sources[type].fd = STDERR_FILENO;
        }
        else {
            nd_log.sources[type].method = ND_LOG_METHOD_STDOUT;
            nd_log.sources[type].filename = NULL;
            nd_log.sources[type].fd = STDOUT_FILENO;
        }
    }
    else if(strcmp(setting, "stderr") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_STDERR;
        nd_log.sources[type].filename = NULL;
        nd_log.sources[type].fd = STDERR_FILENO;
    }
    else if(strcmp(setting, "stdout") == 0) {
        nd_log.sources[type].method = ND_LOG_METHOD_STDOUT;
        nd_log.sources[type].filename = NULL;
        nd_log.sources[type].fd = STDOUT_FILENO;
    }
    else {
        nd_log.sources[type].method = ND_LOG_METHOD_FILE;
        nd_log.sources[type].filename = setting;
    }
}

void nd_log_set_severity_level(const char *severity) {
    if(!severity || !*severity)
        severity = "info";

    ND_LOG_FIELD_PRIORITY priority = nd_log_priority2id(severity);
    nd_log.sources[NDLS_DAEMON].min_priority = priority;
    nd_log.sources[NDLS_COLLECTORS].min_priority = priority;
    setenv("NETDATA_LOG_SEVERITY_LEVEL", nd_log_id2priority(priority), 1);
}

void nd_log_set_facility(const char *facility) {
    if(!facility || !*facility)
        facility = "daemon";

    nd_log.syslog.facility = nd_log_facility2id(facility);
    setenv("NETDATA_SYSLOG_FACILITY", nd_log_id2facility(nd_log.syslog.facility), 1);
}

void nd_log_set_flood_protection(time_t period, size_t logs) {
    nd_log.limits.throttle_period = period;
    nd_log.limits.logs_per_period = nd_log.limits.logs_per_period_backup = logs;

    char buf[100];
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t )period);
    setenv("NETDATA_ERRORS_THROTTLE_PERIOD", buf, 1);
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t )logs);
    setenv("NETDATA_ERRORS_PER_PERIOD", buf, 1);
}

void nd_log_initialize_for_external_plugins(const char *name) {
    program_name = name;

    nd_log_set_severity_level(getenv("NETDATA_LOG_SEVERITY_LEVEL"));
    nd_log_set_facility(getenv("NETDATA_SYSLOG_FACILITY"));

    time_t period = 1200;
    size_t logs = 200;
    const char *s = getenv("NETDATA_ERRORS_THROTTLE_PERIOD");
    if(s && *s >= '0' && *s <= '9') {
        period = str2l(s);
        if(period < 0) period = 0;
    }

    s = getenv("NETDATA_ERRORS_PER_PERIOD");
    if(s && *s >= '0' && *s <= '9')
        logs = str2u(s);

    nd_log_set_flood_protection(period, logs);

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].method = ND_LOG_METHOD_STDERR;
        nd_log.sources[i].fd = -1;
        nd_log.sources[i].fp = NULL;
    }

    nd_log.overwrite_process_source = NDLS_COLLECTORS;
}

static void nd_log_syslog_init() {
    if(nd_log.syslog.initialized)
        return;

    openlog(program_name, LOG_PID, nd_log.syslog.facility);
    nd_log.syslog.initialized = true;
}

static bool nd_log_replace_existing_fd(struct nd_log_source *e, int new_fd) {
    if(new_fd == -1 || e->fd == -1 ||
            (e->fd == STDOUT_FILENO && nd_log.std_output.initialized) ||
            (e->fd == STDERR_FILENO && nd_log.std_error.initialized))
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

        if(e->fd == STDOUT_FILENO)
            nd_log.std_output.initialized = true;
        else if(e->fd == STDERR_FILENO)
            nd_log.std_error.initialized = true;

        return ret;
    }

    return false;
}

static void nd_log_open(struct nd_log_source *e, ND_LOG_SOURCES source) {
    if(e->method == ND_LOG_METHOD_DEFAULT)
        nd_log_set_destination_output(source, e->filename);

    if((e->method == ND_LOG_METHOD_FILE && !e->filename) ||
       (e->method == ND_LOG_METHOD_DEVNULL && e->fd == -1))
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
                if(e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO) {
                    e->fd = STDERR_FILENO;
                    e->method = ND_LOG_METHOD_STDERR;
                    netdata_log_error("Cannot open log file '%s'. Falling back to stderr.", e->filename);
                }
                else
                    netdata_log_error("Cannot open log file '%s'. Leaving fd %d as-is.", e->filename, e->fd);
            }
            else {
                if (!nd_log_replace_existing_fd(e, fd)) {
                    if(e->fd == STDOUT_FILENO || e->fd == STDERR_FILENO) {
                        if(e->fd == STDOUT_FILENO)
                            e->method = ND_LOG_METHOD_STDOUT;
                        else if(e->fd == STDERR_FILENO)
                            e->method = ND_LOG_METHOD_STDERR;

                        // we have dup2() fd, so we can close the one we opened
                        if(fd != STDOUT_FILENO && fd != STDERR_FILENO)
                            close(fd);
                    }
                    else
                        e->fd = fd;
                }
            }

            // at this point we have e->fd set properly

            if(e->fd == STDOUT_FILENO)
                e->fp = stdout;
            else if(e->fd == STDERR_FILENO)
                e->fp = stderr;

            if(!e->fp) {
                e->fp = fdopen(e->fd, "a");
                if (!e->fp) {
                    netdata_log_error("Cannot fdopen() fd %d ('%s')", e->fd, e->filename);

                    if(e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO)
                        close(e->fd);

                    e->fp = stderr;
                    e->fd = STDERR_FILENO;
                }
            }
            else {
                if (setvbuf(e->fp, NULL, _IOLBF, 0) != 0)
                    netdata_log_error("Cannot set line buffering on fd %d ('%s')", e->fd, e->filename);
            }
        }
        break;
    }
}

static void nd_log_stdin_init(int fd, const char *filename) {
    int f = open(filename, O_WRONLY | O_APPEND | O_CREAT, 0664);
    if(f == -1)
        return;

    if(f != fd) {
        dup2(f, fd);
        close(f);
    }
}

void nd_log_initialize(void) {
    nd_log_stdin_init(STDIN_FILENO, "/dev/null");

    for(size_t i = 0 ; i < _NDLS_MAX ; i++)
        nd_log_open(&nd_log.sources[i], i);
}

void nd_log_reopen_log_files(void) {
    netdata_log_info("Reopening all log files.");

    nd_log.std_output.initialized = false;
    nd_log.std_error.initialized = false;
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
// annotators
struct log_field;
static void errno_annotator(BUFFER *wb, const char *key, struct log_field *lf);
static void priority_annotator(BUFFER *wb, const char *key, struct log_field *lf);
static void timestamp_annotator(BUFFER *wb, const char *key, struct log_field *lf);

// ----------------------------------------------------------------------------

typedef void (*annotator_t)(BUFFER *wb, const char *key, struct log_field *lf);

struct log_field {
    const char *journal;
    const char *logfmt;
    annotator_t logfmt_annotator;
    struct log_stack_entry entry;
};

static __thread struct log_stack_entry *thread_log_stack_base = NULL;
static __thread struct log_field thread_log_fields_daemon[_NDF_MAX] = {
        [NDF_TIMESTAMP_REALTIME_USEC] = {
                .journal = NULL,
                .logfmt = "timestamp",
                .logfmt_annotator = timestamp_annotator,
        },
        [NDF_LOG_SOURCE] = {
                .journal = "ND_LOG_SOURCE",
                .logfmt = "source",
        },
        [NDF_SYSLOG_IDENTIFIER] = {
                .journal = "SYSLOG_IDENTIFIER",
                .logfmt = "comm",
        },
        [NDF_LINE] = {
                .journal = "CODE_LINE",
                .logfmt = NULL,
        },
        [NDF_FILE] = {
                .journal = "CODE_FILE",
                .logfmt = NULL,
        },
        [NDF_FUNC] = {
                .journal = "CODE_FUNC",
                .logfmt = NULL,
        },
        [NDF_ERRNO] = {
                .journal = "ERRNO",
                .logfmt = "errno",
                .logfmt_annotator = errno_annotator,
        },
        [NDF_PRIORITY] = {
                .journal = "PRIORITY",
                .logfmt = "level",
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
                .logfmt = "thread",
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
        [NDF_CONNECTION_ID] = {
                .journal = "ND_CONNECTION_ID",
                .logfmt = "conn",
        },
        [NDF_SRC_TRANSPORT] = {
                .journal = "ND_SRC_TRANSPORT",
                .logfmt = "src_transport",
        },
        [NDF_SRC_IP] = {
                .journal = "ND_SRC_IP",
                .logfmt = "src_ip",
        },
        [NDF_SRC_PORT] = {
                .journal = "ND_SRC_PORT",
                .logfmt = "src_port",
        },
        [NDF_SRC_METHOD] = {
                .journal = "ND_SRC_METHOD",
                .logfmt = "src_method",
        },
        [NDF_HANDLER] = {
                .journal = "ND_HANDLER",
                .logfmt = "handler",
        },
        [NDF_REQUEST_MODE] = {
                .journal = "ND_REQUEST_MODE",
                .logfmt = "mode",
        },
        [NDF_STATUS] = {
                .journal = "ND_STATUS",
                .logfmt = "status",
        },
        [NDF_RESPONSE_CODE] = {
                .journal = "ND_RESPONSE_CODE",
                .logfmt = "code",
        },
        [NDF_RESPONSE_BYTES] = {
                .journal = "ND_RESPONSE_SENT_BYTES",
                .logfmt = "sent_bytes",
        },
        [NDF_RESPONSE_SIZE_BYTES] = {
                .journal = "ND_RESPONSE_SIZE_BYTES",
                .logfmt = "size_bytes",
        },
        [NDF_RESPONSE_PREPARATION_TIME_USEC] = {
                .journal = "ND_RESPONSE_PREP_TIME_USEC",
                .logfmt = "prep_ut",
        },
        [NDF_RESPONSE_SENT_TIME_USEC] = {
                .journal = "ND_RESPONSE_SENT_TIME_USEC",
                .logfmt = "sent_ut",
        },
        [NDF_RESPONSE_TOTAL_TIME_USEC] = {
                .journal = "ND_RESPONSE_TOTAL_TIME_USEC",
                .logfmt = "total_ut",
        },

        // put new items here
        // leave the request URL and the message last

        [NDF_REQUEST_URL] = {
                .journal = "ND_REQUEST_URL",
                .logfmt = "url",
        },
        [NDF_MESSAGE] = {
                .journal = "MESSAGE",
                .logfmt = "msg",
        },
};

#define THREAD_FIELDS_MAX (sizeof(thread_log_fields_daemon) / sizeof(thread_log_fields_daemon[0]))

void log_stack_pop(void *ptr) {
    if(!ptr)
        return;

    struct log_stack_entry *lgs = *(struct log_stack_entry (*)[])ptr;

    if(!lgs->prev)
        return;

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(thread_log_stack_base, lgs, prev, next);
}

void log_stack_push(struct log_stack_entry *lgs) {
    if(!lgs)
        return;

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(thread_log_stack_base, lgs, prev, next);
}

// ----------------------------------------------------------------------------
// logfmt formatter

static void timestamp_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    usec_t ut = lf->entry.u64;

    if(!ut)
        return;

    char datetime[ISO8601_MAX_LENGTH];
    iso8601_datetime_utc(datetime, sizeof(datetime), ut);

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);
    buffer_json_strcat(wb, datetime);
}

static void errno_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    int errnum = lf->entry.i32;

    if(errnum == 0)
        return;

    char buf[1024];
    const char *s = errno2str(errnum, buf, sizeof(buf));

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=\"", 2);
    buffer_print_int64(wb, errnum);
    buffer_fast_strcat(wb, ", ", 2);
    buffer_json_strcat(wb, s);
    buffer_fast_strcat(wb, "\"", 1);
}

static void priority_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    static char *priorities[] = {
            [NDLP_ALERT] = "alert",
            [NDLP_CRIT] = "critical",
            [NDLP_EMERG] = "emergency",
            [NDLP_ERR] = "error",
            [NDLP_WARNING] = "warning",
            [NDLP_INFO] = "info",
            [NDLP_NOTICE] = "notice",
            [NDLP_DEBUG] = "debug",
    };

    uint64_t pri = lf->entry.u64;

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);

    size_t entries = sizeof(priorities) / sizeof(priorities[0]);
    if(pri < entries)
        buffer_strcat(wb, priorities[pri]);
    else
        buffer_print_uint64(wb, pri);
}

static bool string_has_spaces(const char *s) {
    while(*s) {
        if(isspace(*s))
            return true;

        s++;
    }

    return false;
}

static void string_to_logfmt(BUFFER *wb, const char *s) {
    bool spaces = string_has_spaces(s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);

    buffer_json_strcat(wb, s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);
}

static void nd_logger_logfmt(BUFFER *wb, struct log_field *fields, size_t fields_max) {
    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        const char *key = fields[i].logfmt;

        if(fields[i].logfmt_annotator)
            fields[i].logfmt_annotator(wb, key, &fields[i]);
        else {
            if(buffer_strlen(wb))
                buffer_fast_strcat(wb, " ", 1);

            buffer_strcat(wb, key);
            buffer_fast_strcat(wb, "=", 1);
            switch(fields[i].entry.type) {
                case NDFT_TXT:
                    string_to_logfmt(wb, fields[i].entry.txt);
                    break;
                case NDFT_STR:
                    string_to_logfmt(wb, string2str(fields[i].entry.str));
                    break;
                case NDFT_BFR:
                    string_to_logfmt(wb, buffer_tostring(fields[i].entry.bfr));
                    break;
                case NDFT_U32:
                    buffer_print_uint64(wb, fields[i].entry.u32);
                    break;
                case NDFT_I32:
                    buffer_print_int64(wb, fields[i].entry.i32);
                    break;
                case NDFT_U64:
                case NDFT_TIMESTAMP_USEC:
                    buffer_print_uint64(wb, fields[i].entry.u64);
                    break;
                case NDFT_I64:
                    buffer_print_int64(wb, fields[i].entry.i64);
                    break;
                case NDFT_DBL:
                    buffer_print_netdata_double(wb, fields[i].entry.dbl);
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

// ----------------------------------------------------------------------------
// journal logger

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
                asprintf(&value, "%s=%s", key, fields[i].entry.txt);
                break;
            case NDFT_STR:
                asprintf(&value, "%s=%s", key, string2str(fields[i].entry.str));
                break;
            case NDFT_BFR:
                asprintf(&value, "%s=%s", key, buffer_tostring(fields[i].entry.bfr));
                break;
            case NDFT_U32:
                asprintf(&value, "%s=%u", key, fields[i].entry.u32);
                break;
            case NDFT_I32:
                asprintf(&value, "%s=%d", key, fields[i].entry.i32);
                break;
            case NDFT_U64:
            case NDFT_TIMESTAMP_USEC:
                asprintf(&value, "%s=%" PRIu64, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                asprintf(&value, "%s=%" PRId64, key, fields[i].entry.i64);
                break;
            case NDFT_DBL:
                asprintf(&value, "%s=%f", key, fields[i].entry.dbl);
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

// ----------------------------------------------------------------------------
// syslog logger - uses logfmt

static bool nd_logger_syslog(int priority, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(1024, NULL);

    nd_logger_logfmt(wb, fields, fields_max);
    syslog(priority, "%s", buffer_tostring(wb));

    buffer_free(wb);
    return true;
}

// ----------------------------------------------------------------------------
// file logger - uses logfmt

static bool nd_logger_file(FILE *fp, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(1024, NULL);

    nd_logger_logfmt(wb, fields, fields_max);
    int r = fprintf(fp, "%s\n", buffer_tostring(wb));

    buffer_free(wb);
    return r > 0;
}

// ----------------------------------------------------------------------------
// logger router

static ND_LOG_METHOD nd_logger_select_method(ND_LOG_SOURCES source, FILE **fpp, SPINLOCK **spinlock) {
    *spinlock = NULL;
    ND_LOG_METHOD method = nd_log.sources[source].method;

    switch(method) {
#ifdef HAVE_SYSTEMD
        case ND_LOG_METHOD_JOURNAL:
            if(unlikely(!nd_log.journal.initialized)) {
                method = ND_LOG_METHOD_FILE;
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
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
                *spinlock = &nd_log.std_error.spinlock;
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
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = nd_log.sources[source].fp;
                *spinlock = &nd_log.sources[source].spinlock;
            }
            break;

        case ND_LOG_METHOD_STDOUT:
            method = ND_LOG_METHOD_FILE;
            *fpp = stdout;
            *spinlock = &nd_log.std_output.spinlock;
            break;

        default:
        case ND_LOG_METHOD_DEFAULT:
        case ND_LOG_METHOD_STDERR:
            method = ND_LOG_METHOD_FILE;
            *fpp = stderr;
            *spinlock = &nd_log.std_error.spinlock;
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

// ----------------------------------------------------------------------------
// high level logger

static void nd_logger_log_fields(SPINLOCK *spinlock, FILE *fp, bool limit, ND_LOG_FIELD_PRIORITY priority,
                                 ND_LOG_METHOD method, struct nd_log_source *source,
                                 struct log_field *fields, size_t fields_max) {
    if(spinlock)
        spinlock_lock(spinlock);

    // check the limits
    if(limit && nd_log_limit_reached(source))
        goto cleanup;

    if(method == ND_LOG_METHOD_JOURNAL) {
        if(!nd_logger_journal(fields, fields_max)) {
            // we can't log to journal, let's log to stderr
            if(spinlock)
                spinlock_unlock(spinlock);

            method = ND_LOG_METHOD_FILE;
            spinlock = &nd_log.std_error.spinlock;
            fp = stderr;

            if(spinlock)
                spinlock_lock(spinlock);
        }
    }

    if(method == ND_LOG_METHOD_SYSLOG)
        nd_logger_syslog(priority, fields, fields_max);

    if(method == ND_LOG_METHOD_FILE)
        nd_logger_file(fp, fields, fields_max);


cleanup:
    if(spinlock)
        spinlock_unlock(spinlock);
}

static void nd_logger_unset_all_thread_fields(void) {
    size_t fields_max = THREAD_FIELDS_MAX;
    for(size_t i = 0; i < fields_max ; i++)
        thread_log_fields_daemon[i].entry.set = false;
}

static void nd_logger_merge_log_stack_to_thread_fields(void) {
    for(struct log_stack_entry *l = thread_log_stack_base; l ; l = l->next) {
        for(size_t i = 0; l[i].id != NDF_STOP ;i++) {
            if(l[i].id >= _NDF_MAX)
                continue;

            thread_log_fields_daemon[l[i].id].entry = l[i];
            thread_log_fields_daemon[l[i].id].entry.set = true;
        }
    }
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
    nd_logger_unset_all_thread_fields();

    // flatten the log stack into the fields
    nd_logger_merge_log_stack_to_thread_fields();

    // set the common fields that are automatically set by the logging subsystem

    if(!thread_log_fields_daemon[NDF_LOG_SOURCE].entry.set)
        thread_log_fields_daemon[NDF_LOG_SOURCE].entry = ND_LOG_FIELD_TXT(NDF_LOG_SOURCE, nd_log_source2str(source));

    if(!thread_log_fields_daemon[NDF_SYSLOG_IDENTIFIER].entry.set)
        thread_log_fields_daemon[NDF_SYSLOG_IDENTIFIER].entry = ND_LOG_FIELD_TXT(NDF_SYSLOG_IDENTIFIER, program_name);

    if(!thread_log_fields_daemon[NDF_LINE].entry.set) {
        thread_log_fields_daemon[NDF_LINE].entry = ND_LOG_FIELD_U64(NDF_LINE, line);
        thread_log_fields_daemon[NDF_FILE].entry = ND_LOG_FIELD_TXT(NDF_FILE, file);
        thread_log_fields_daemon[NDF_FUNC].entry = ND_LOG_FIELD_TXT(NDF_FUNC, function);
    }

    if(!thread_log_fields_daemon[NDF_PRIORITY].entry.set) {
        thread_log_fields_daemon[NDF_PRIORITY].entry = ND_LOG_FIELD_U64(NDF_PRIORITY, priority);
    }

    if(!thread_log_fields_daemon[NDF_TID].entry.set) {
        thread_log_fields_daemon[NDF_TID].entry = ND_LOG_FIELD_U64(NDF_TID, gettid());

        char os_threadname[NETDATA_THREAD_NAME_MAX + 1];
        const char *thread_tag = netdata_thread_tag();
        if(!netdata_thread_tag_exists()) {
            if (!netdata_thread_tag_exists()) {
                os_thread_get_current_name_np(os_threadname);
                if ('\0' != os_threadname[0])
                    /* If it is not an empty string replace "MAIN" thread_tag */
                    thread_tag = os_threadname;
            }
        }
        thread_log_fields_daemon[NDF_THREAD].entry = ND_LOG_FIELD_TXT(NDF_THREAD, thread_tag);
    }

    if(!thread_log_fields_daemon[NDF_TIMESTAMP_REALTIME_USEC].entry.set) {
        thread_log_fields_daemon[NDF_TIMESTAMP_REALTIME_USEC].entry = ND_LOG_FIELD_TMT(NDF_TIMESTAMP_REALTIME_USEC, now_realtime_usec());
    }

    if(!thread_log_fields_daemon[NDF_ERRNO].entry.set) {
        thread_log_fields_daemon[NDF_ERRNO].entry = ND_LOG_FIELD_I32(NDF_ERRNO, errno);
    }

    BUFFER *wb = NULL;
    if(fmt) {
        wb = buffer_create(0, NULL);
        buffer_vsprintf(wb, fmt, ap);
        thread_log_fields_daemon[NDF_MESSAGE].entry = ND_LOG_FIELD_TXT(NDF_MESSAGE, buffer_tostring(wb));
    }

    nd_logger_log_fields(spinlock, fp, limit, priority, method, &nd_log.sources[source],
                         thread_log_fields_daemon , THREAD_FIELDS_MAX);

    buffer_free(wb);

    if(nd_log.sources[source].pending_msg) {
        // log a pending message

        nd_logger_unset_all_thread_fields();

        thread_log_fields_daemon[NDF_TIMESTAMP_REALTIME_USEC].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TIMESTAMP_USEC,
                .u64 = now_realtime_usec(),
        };

        thread_log_fields_daemon[NDF_LOG_SOURCE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log_source2str(source),
        };

        thread_log_fields_daemon[NDF_SYSLOG_IDENTIFIER].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = program_name,
        };

        thread_log_fields_daemon[NDF_MESSAGE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log.sources[source].pending_msg,
        };

        nd_logger_log_fields(spinlock, fp, false, priority, method,
                             &nd_log.sources[source],
                             thread_log_fields_daemon , THREAD_FIELDS_MAX);

        freez((void *)nd_log.sources[source].pending_msg);
        nd_log.sources[source].pending_msg = NULL;
    }

    errno = 0;
}

static ND_LOG_SOURCES nd_log_validate_source(ND_LOG_SOURCES source) {
    if(source >= _NDLS_MAX)
        source = NDLS_DAEMON;

    if(overwrite_thread_source)
        source = overwrite_thread_source;

    if(nd_log.overwrite_process_source)
        source = overwrite_thread_source;

    return source;
}

// ----------------------------------------------------------------------------
// public API for loggers

void netdata_logger(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... ) {
    source = nd_log_validate_source(source);

#if !defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_DEV_MODE)
    if((source == NDLS_DAEMON || source == NDLS_COLLECTORS) && priority > nd_log.sources[source].min_priority)
        return;
#endif

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority,
              source == NDLS_DAEMON || source == NDLS_COLLECTORS,
              fmt, args);
    va_end(args);
}

void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
    source = nd_log_validate_source(source);

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
    nd_logger(file, function, line, source, priority,
            source == NDLS_DAEMON || source == NDLS_COLLECTORS,
            fmt, args);
    va_end(args);
}

void netdata_logger_fatal( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    ND_LOG_SOURCES source = NDLS_DAEMON;
    source = nd_log_validate_source(source);

    int __errno = errno;

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, NDLP_CRIT, true, fmt, args);
    va_end(args);

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    char action_data[70+1];
    snprintfz(action_data, 70, "%04lu@%-10.10s:%-15.15s/%d", line, file, function, __errno);
    char action_result[60+1];

    const char *thread_tag = thread_log_fields_daemon[NDF_THREAD].entry.txt;
    if(!thread_tag)
        thread_tag = "UNKNOWN";

    const char *tag_to_send =  thread_tag;

    // anonymize thread names
    if(strncmp(thread_tag, THREAD_TAG_STREAM_RECEIVER, strlen(THREAD_TAG_STREAM_RECEIVER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_RECEIVER;
    if(strncmp(thread_tag, THREAD_TAG_STREAM_SENDER, strlen(THREAD_TAG_STREAM_SENDER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_SENDER;

    snprintfz(action_result, 60, "%s:%s", program_name, tag_to_send);
    send_statistics("FATAL", action_result, action_data);

#ifdef HAVE_BACKTRACE
    int fd = nd_log.sources[NDLS_DAEMON].fd;
    if(fd == -1)
        fd = STDERR_FILENO;

    int nptrs;
    void *buffer[10000];

    nptrs = backtrace(buffer, sizeof(buffer));
    if(nptrs)
        backtrace_symbols_fd(buffer, nptrs, fd);
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    abort();
#endif

    netdata_cleanup_and_exit(1);
}

// ----------------------------------------------------------------------------
// log limits

void nd_log_limits_reset(void) {
    usec_t now_ut = now_monotonic_usec();

    nd_log.limits.logs_per_period = nd_log.limits.logs_per_period_backup;

    spinlock_lock(&nd_log.std_output.spinlock);
    spinlock_lock(&nd_log.std_error.spinlock);

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        spinlock_lock(&nd_log.sources[i].spinlock);
        nd_log.sources[i].limits.prevented = 0;
        nd_log.sources[i].limits.counter = 0;
        nd_log.sources[i].limits.started_monotonic_ut = now_ut;
        spinlock_unlock(&nd_log.sources[i].spinlock);
    }

    spinlock_unlock(&nd_log.std_output.spinlock);
    spinlock_unlock(&nd_log.std_error.spinlock);
}

void nd_log_limits_unlimited(void) {
    nd_log_limits_reset();
    nd_log.limits.logs_per_period = ((nd_log.limits.logs_per_period_backup * 10) < 10000) ? 10000 : (nd_log.limits.logs_per_period_backup * 10);
}

static bool nd_log_limit_reached(struct nd_log_source *source) {
    if(nd_log.limits.throttle_period == 0 || nd_log.limits.logs_per_period == 0)
        return false;

    usec_t now_ut = now_monotonic_usec();
    if(!source->limits.started_monotonic_ut)
        source->limits.started_monotonic_ut = now_ut;

    source->limits.counter++;

    if(now_ut - source->limits.started_monotonic_ut > (usec_t)nd_log.limits.throttle_period) {
        if(source->limits.prevented) {
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                           "LOG FLOOD PROTECTION: resuming logging "
                           "(prevented %"PRIu32" logs in the last %"PRIu32" seconds).",
                           source->limits.prevented,
                           nd_log.limits.throttle_period);

            if(source->pending_msg)
                freez((void *)source->pending_msg);

            source->pending_msg = strdupz(buffer_tostring(wb));

            buffer_free(wb);
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
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                    "LOG FLOOD PROTECTION: too many logs (%"PRIu32" logs in %"PRId64" seconds, threshold is set to %"PRIu32" logs "
                    "in %"PRIu32" seconds). Preventing more logs from process '%s' for %"PRId64" seconds.",
                    source->limits.counter,
                    (int64_t)((now_ut - source->limits.started_monotonic_ut) / USEC_PER_SEC),
                    nd_log.limits.logs_per_period,
                    nd_log.limits.throttle_period,
                    program_name,
                    (int64_t)((source->limits.started_monotonic_ut + (nd_log.limits.throttle_period * USEC_PER_SEC) - now_ut)) / USEC_PER_SEC);

            if(source->pending_msg)
                freez((void *)source->pending_msg);

            source->pending_msg = strdupz(buffer_tostring(wb));

            buffer_free(wb);
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
