// SPDX-License-Identifier: GPL-3.0-or-later

// do not REMOVE this, it is used by systemd-journal includes to prevent saving the file, function, line of the
// source code that makes the calls, allowing our loggers to log the lines of source code that actually log
#define SD_JOURNAL_SUPPRESS_LOCATION

#include "../libnetdata.h"

#ifdef __FreeBSD__
#include <sys/endian.h>
#endif

#ifdef __APPLE__
#include <machine/endian.h>
#endif

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

struct nd_log_source;
static bool nd_log_limit_reached(struct nd_log_source *source);

// ----------------------------------------------------------------------------
// logging method

typedef enum  __attribute__((__packed__)) {
    NDLM_DISABLED = 0,
    NDLM_DEVNULL,
    NDLM_DEFAULT,
    NDLM_JOURNAL,
    NDLM_SYSLOG,
    NDLM_STDOUT,
    NDLM_STDERR,
    NDLM_FILE,
} ND_LOG_METHOD;

static struct {
    ND_LOG_METHOD method;
    const char *name;
} nd_log_methods[] = {
        { .method = NDLM_DISABLED, .name = "none" },
        { .method = NDLM_DEVNULL, .name = "/dev/null" },
        { .method = NDLM_DEFAULT, .name = "default" },
        { .method = NDLM_JOURNAL, .name = "journal" },
        { .method = NDLM_SYSLOG, .name = "syslog" },
        { .method = NDLM_STDOUT, .name = "stdout" },
        { .method = NDLM_STDERR, .name = "stderr" },
        { .method = NDLM_FILE, .name = "file" },
};

static ND_LOG_METHOD nd_log_method2id(const char *method) {
    if(!method || !*method)
        return NDLM_DEFAULT;

    size_t entries = sizeof(nd_log_methods) / sizeof(nd_log_methods[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_methods[i].name, method) == 0)
            return nd_log_methods[i].method;
    }

    return NDLM_FILE;
}

static const char *nd_log_id2method(ND_LOG_METHOD method) {
    size_t entries = sizeof(nd_log_methods) / sizeof(nd_log_methods[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(method == nd_log_methods[i].method)
            return nd_log_methods[i].name;
    }

    return "unknown";
}

#define IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(ndlo) ((ndlo) == NDLM_JOURNAL || (ndlo) == NDLM_SYSLOG || (ndlo) == NDLM_STDERR)

const char *nd_log_method_for_external_plugins(const char *s) {
    if(s && *s) {
        ND_LOG_METHOD method = nd_log_method2id(s);
        if(IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(method))
            return nd_log_id2method(method);
    }

    return nd_log_id2method(NDLM_STDERR);
}

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

int nd_log_priority2id(const char *priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_priorities[i].name, priority) == 0)
            return nd_log_priorities[i].priority;
    }

    return NDLP_INFO;
}

const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(priority == nd_log_priorities[i].priority)
            return nd_log_priorities[i].name;
    }

    return NDLP_INFO_STR;
}

// ----------------------------------------------------------------------------
// log sources

const char *nd_log_sources[] = {
        [NDLS_UNSET] = "UNSET",
        [NDLS_ACCESS] = "access",
        [NDLS_ACLK] = "aclk",
        [NDLS_COLLECTORS] = "collector",
        [NDLS_DAEMON] = "daemon",
        [NDLS_HEALTH] = "health",
        [NDLS_DEBUG] = "debug",
};

size_t nd_log_source2id(const char *source, ND_LOG_SOURCES def) {
    size_t entries = sizeof(nd_log_sources) / sizeof(nd_log_sources[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_sources[i], source) == 0)
            return i;
    }

    return def;
}


static const char *nd_log_id2source(ND_LOG_SOURCES source) {
    size_t entries = sizeof(nd_log_sources) / sizeof(nd_log_sources[0]);
    if(source < entries)
        return nd_log_sources[source];

    return nd_log_sources[NDLS_COLLECTORS];
}

// ----------------------------------------------------------------------------
// log output formats

typedef enum __attribute__((__packed__)) {
    NDLF_JOURNAL,
    NDLF_LOGFMT,
    NDLF_JSON,
} ND_LOG_FORMAT;

static struct {
    ND_LOG_FORMAT format;
    const char *name;
} nd_log_formats[] = {
        { .format = NDLF_JOURNAL, .name = "journal" },
        { .format = NDLF_LOGFMT, .name = "logfmt" },
        { .format = NDLF_JSON, .name = "json" },
};

static ND_LOG_FORMAT nd_log_format2id(const char *format) {
    if(!format || !*format)
        return NDLF_LOGFMT;

    size_t entries = sizeof(nd_log_formats) / sizeof(nd_log_formats[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_formats[i].name, format) == 0)
            return nd_log_formats[i].format;
    }

    return NDLF_LOGFMT;
}

static const char *nd_log_id2format(ND_LOG_FORMAT format) {
    size_t entries = sizeof(nd_log_formats) / sizeof(nd_log_formats[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(format == nd_log_formats[i].format)
            return nd_log_formats[i].name;
    }

    return "logfmt";
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

// ----------------------------------------------------------------------------

struct nd_log_limit {
    usec_t started_monotonic_ut;
    uint32_t counter;
    uint32_t prevented;

    uint32_t throttle_period;
    uint32_t logs_per_period;
    uint32_t logs_per_period_backup;
};

#define ND_LOG_LIMITS_DEFAULT (struct nd_log_limit){ .logs_per_period = ND_LOG_DEFAULT_THROTTLE_LOGS, .logs_per_period_backup = ND_LOG_DEFAULT_THROTTLE_LOGS, .throttle_period = ND_LOG_DEFAULT_THROTTLE_PERIOD, }
#define ND_LOG_LIMITS_UNLIMITED (struct nd_log_limit){  .logs_per_period = 0, .logs_per_period_backup = 0, .throttle_period = 0, }

struct nd_log_source {
    SPINLOCK spinlock;
    ND_LOG_METHOD method;
    ND_LOG_FORMAT format;
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
    uuid_t invocation_id;

    ND_LOG_SOURCES overwrite_process_source;

    struct nd_log_source sources[_NDLS_MAX];

    struct {
        bool initialized;
    } journal;

    struct {
        bool initialized;
        int fd;
        char filename[FILENAME_MAX + 1];
    } journal_direct;

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

} nd_log = {
        .overwrite_process_source = 0,
        .journal = {
                .initialized = false,
        },
        .journal_direct = {
                .initialized = false,
                .fd = -1,
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
                        .method = NDLM_DISABLED,
                        .format = NDLF_JOURNAL,
                        .filename = NULL,
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_EMERG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACCESS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_DEFAULT,
                        .format = NDLF_LOGFMT,
                        .filename = LOG_DIR "/access.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_ACLK] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_FILE,
                        .format = NDLF_LOGFMT,
                        .filename = LOG_DIR "/aclk.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_COLLECTORS] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_DEFAULT,
                        .format = NDLF_LOGFMT,
                        .filename = LOG_DIR "/collectors.log",
                        .fd = STDERR_FILENO,
                        .fp = NULL,
                        .min_priority = NDLP_INFO,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_DEBUG] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_DISABLED,
                        .format = NDLF_LOGFMT,
                        .filename = LOG_DIR "/debug.log",
                        .fd = STDOUT_FILENO,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
                [NDLS_DAEMON] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_DEFAULT,
                        .filename = LOG_DIR "/daemon.log",
                        .format = NDLF_LOGFMT,
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_INFO,
                        .limits = ND_LOG_LIMITS_DEFAULT,
                },
                [NDLS_HEALTH] = {
                        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                        .method = NDLM_DEFAULT,
                        .format = NDLF_LOGFMT,
                        .filename = LOG_DIR "/health.log",
                        .fd = -1,
                        .fp = NULL,
                        .min_priority = NDLP_DEBUG,
                        .limits = ND_LOG_LIMITS_UNLIMITED,
                },
        },
};

__attribute__((constructor)) void initialize_invocation_id(void) {
    // check for a NETDATA_INVOCATION_ID
    if(uuid_parse_flexi(getenv("NETDATA_INVOCATION_ID"), nd_log.invocation_id) != 0) {
        // not found, check for systemd set INVOCATION_ID
        if(uuid_parse_flexi(getenv("INVOCATION_ID"), nd_log.invocation_id) != 0) {
            // not found, generate a new one
            uuid_generate_random(nd_log.invocation_id);
        }
    }

    char uuid[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(nd_log.invocation_id, uuid);
    setenv("NETDATA_INVOCATION_ID", uuid, 1);
}

int nd_log_health_fd(void) {
    if(nd_log.sources[NDLS_HEALTH].method == NDLM_FILE && nd_log.sources[NDLS_HEALTH].fd != -1)
        return nd_log.sources[NDLS_HEALTH].fd;

    return STDERR_FILENO;
}

void nd_log_set_user_settings(ND_LOG_SOURCES source, const char *setting) {
    char buf[FILENAME_MAX + 100];
    if(setting && *setting)
        strncpyz(buf, setting, sizeof(buf) - 1);
    else
        buf[0] = '\0';

    struct nd_log_source *ls = &nd_log.sources[source];
    char *output = strrchr(buf, '@');

    if(!output)
        // all of it is the output
        output = buf;
    else {
        // we found an '@', the next char is the output
        *output = '\0';
        output++;

        // parse the other params
        char *remaining = buf;
        while(remaining) {
            char *value = strsep_skip_consecutive_separators(&remaining, ",");
            if (!value || !*value) continue;

            char *name = strsep_skip_consecutive_separators(&value, "=");
            if (!name || !*name) continue;

            if(strcmp(name, "logfmt") == 0)
                ls->format = NDLF_LOGFMT;
            else if(strcmp(name, "json") == 0)
                ls->format = NDLF_JSON;
            else if(strcmp(name, "journal") == 0)
                ls->format = NDLF_JOURNAL;
            else if(strcmp(name, "level") == 0 && value && *value)
                ls->min_priority = nd_log_priority2id(value);
            else if(strcmp(name, "protection") == 0 && value && *value) {
                if(strcmp(value, "off") == 0 || strcmp(value, "none") == 0) {
                    ls->limits = ND_LOG_LIMITS_UNLIMITED;
                    ls->limits.counter = 0;
                    ls->limits.prevented = 0;
                }
                else {
                    ls->limits = ND_LOG_LIMITS_DEFAULT;

                    char *slash = strchr(value, '/');
                    if(slash) {
                        *slash = '\0';
                        slash++;
                        ls->limits.logs_per_period = ls->limits.logs_per_period_backup = str2u(value);
                        ls->limits.throttle_period = str2u(slash);
                    }
                    else {
                        ls->limits.logs_per_period = ls->limits.logs_per_period_backup = str2u(value);
                        ls->limits.throttle_period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
                    }
                }
            }
            else
                nd_log(NDLS_DAEMON, NDLP_ERR, "Error while parsing configuration of log source '%s'. "
                                              "In config '%s', '%s' is not understood.",
                       nd_log_id2source(source), setting, name);
        }
    }

    if(!output || !*output || strcmp(output, "none") == 0 || strcmp(output, "off") == 0) {
        ls->method = NDLM_DISABLED;
        ls->filename = "/dev/null";
    }
    else if(strcmp(output, "journal") == 0) {
        ls->method = NDLM_JOURNAL;
        ls->filename = NULL;
    }
    else if(strcmp(output, "syslog") == 0) {
        ls->method = NDLM_SYSLOG;
        ls->filename = NULL;
    }
    else if(strcmp(output, "/dev/null") == 0) {
        ls->method = NDLM_DEVNULL;
        ls->filename = "/dev/null";
    }
    else if(strcmp(output, "system") == 0) {
        if(ls->fd == STDERR_FILENO) {
            ls->method = NDLM_STDERR;
            ls->filename = NULL;
            ls->fd = STDERR_FILENO;
        }
        else {
            ls->method = NDLM_STDOUT;
            ls->filename = NULL;
            ls->fd = STDOUT_FILENO;
        }
    }
    else if(strcmp(output, "stderr") == 0) {
        ls->method = NDLM_STDERR;
        ls->filename = NULL;
        ls->fd = STDERR_FILENO;
    }
    else if(strcmp(output, "stdout") == 0) {
        ls->method = NDLM_STDOUT;
        ls->filename = NULL;
        ls->fd = STDOUT_FILENO;
    }
    else {
        ls->method = NDLM_FILE;
        ls->filename = strdupz(output);
    }

#if defined(NETDATA_INTERNAL_CHECKS) || defined(NETDATA_DEV_MODE)
    ls->min_priority = NDLP_DEBUG;
#endif

    if(source == NDLS_COLLECTORS) {
        // set the method for the collector processes we will spawn

        ND_LOG_METHOD method;
        ND_LOG_FORMAT format = ls->format;
        ND_LOG_FIELD_PRIORITY priority = ls->min_priority;

        if(ls->method == NDLM_SYSLOG || ls->method == NDLM_JOURNAL)
            method = ls->method;
        else
            method = NDLM_STDERR;

        setenv("NETDATA_LOG_METHOD", nd_log_id2method(method), 1);
        setenv("NETDATA_LOG_FORMAT", nd_log_id2format(format), 1);
        setenv("NETDATA_LOG_LEVEL", nd_log_id2priority(priority), 1);
    }
}

void nd_log_set_priority_level(const char *setting) {
    if(!setting || !*setting)
        setting = "info";

    ND_LOG_FIELD_PRIORITY priority = nd_log_priority2id(setting);

#if defined(NETDATA_INTERNAL_CHECKS) || defined(NETDATA_DEV_MODE)
    priority = NDLP_DEBUG;
#endif

    nd_log.sources[NDLS_DAEMON].min_priority = priority;
    nd_log.sources[NDLS_COLLECTORS].min_priority = priority;

    // the right one
    setenv("NETDATA_LOG_LEVEL", nd_log_id2priority(priority), 1);
}

void nd_log_set_facility(const char *facility) {
    if(!facility || !*facility)
        facility = "daemon";

    nd_log.syslog.facility = nd_log_facility2id(facility);
    setenv("NETDATA_SYSLOG_FACILITY", nd_log_id2facility(nd_log.syslog.facility), 1);
}

void nd_log_set_flood_protection(size_t logs, time_t period) {
    nd_log.sources[NDLS_DAEMON].limits.logs_per_period =
            nd_log.sources[NDLS_DAEMON].limits.logs_per_period_backup;
            nd_log.sources[NDLS_COLLECTORS].limits.logs_per_period =
            nd_log.sources[NDLS_COLLECTORS].limits.logs_per_period_backup = logs;

    nd_log.sources[NDLS_DAEMON].limits.throttle_period =
            nd_log.sources[NDLS_COLLECTORS].limits.throttle_period = period;

    char buf[100];
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t )period);
    setenv("NETDATA_ERRORS_THROTTLE_PERIOD", buf, 1);
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t )logs);
    setenv("NETDATA_ERRORS_PER_PERIOD", buf, 1);
}

static bool nd_log_journal_systemd_init(void) {
#ifdef HAVE_SYSTEMD
    nd_log.journal.initialized = true;
#else
    nd_log.journal.initialized = false;
#endif

    return nd_log.journal.initialized;
}

static void nd_log_journal_direct_set_env(void) {
    if(nd_log.sources[NDLS_COLLECTORS].method == NDLM_JOURNAL)
        setenv("NETDATA_SYSTEMD_JOURNAL_PATH", nd_log.journal_direct.filename, 1);
}

static bool nd_log_journal_direct_init(const char *path) {
    if(nd_log.journal_direct.initialized) {
        nd_log_journal_direct_set_env();
        return true;
    }

    int fd;
    char filename[FILENAME_MAX + 1];
    if(!is_path_unix_socket(path)) {

        journal_construct_path(filename, sizeof(filename), netdata_configured_host_prefix, "netdata");
        if (!is_path_unix_socket(filename) || (fd = journal_direct_fd(filename)) == -1) {

            journal_construct_path(filename, sizeof(filename), netdata_configured_host_prefix, NULL);
            if (!is_path_unix_socket(filename) || (fd = journal_direct_fd(filename)) == -1) {

                journal_construct_path(filename, sizeof(filename), NULL, "netdata");
                if (!is_path_unix_socket(filename) || (fd = journal_direct_fd(filename)) == -1) {

                    journal_construct_path(filename, sizeof(filename), NULL, NULL);
                    if (!is_path_unix_socket(filename) || (fd = journal_direct_fd(filename)) == -1)
                        return false;
                }
            }
        }
    }
    else {
        snprintfz(filename, sizeof(filename), "%s", path);
        fd = journal_direct_fd(filename);
    }

    if(fd < 0)
        return false;

    nd_log.journal_direct.fd = fd;
    nd_log.journal_direct.initialized = true;

    strncpyz(nd_log.journal_direct.filename, filename, sizeof(nd_log.journal_direct.filename) - 1);
    nd_log_journal_direct_set_env();

    return true;
}

static void nd_log_syslog_init() {
    if(nd_log.syslog.initialized)
        return;

    openlog(program_name, LOG_PID, nd_log.syslog.facility);
    nd_log.syslog.initialized = true;
}

void nd_log_initialize_for_external_plugins(const char *name) {
    // if we don't run under Netdata, log to stderr,
    // otherwise, use the logging method Netdata wants us to use.
    setenv("NETDATA_LOG_METHOD", "stderr", 0);
    setenv("NETDATA_LOG_FORMAT", "logfmt", 0);

    nd_log.overwrite_process_source = NDLS_COLLECTORS;
    program_name = name;

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].method = STDERR_FILENO;
        nd_log.sources[i].fd = -1;
        nd_log.sources[i].fp = NULL;
    }

    nd_log_set_priority_level(getenv("NETDATA_LOG_LEVEL"));
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

    nd_log_set_flood_protection(logs, period);

    if(!netdata_configured_host_prefix) {
        s = getenv("NETDATA_HOST_PREFIX");
        if(s && *s)
            netdata_configured_host_prefix = (char *)s;
    }

    ND_LOG_METHOD method = nd_log_method2id(getenv("NETDATA_LOG_METHOD"));
    ND_LOG_FORMAT format = nd_log_format2id(getenv("NETDATA_LOG_FORMAT"));

    if(!IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(method)) {
        if(is_stderr_connected_to_journal()) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING, "NETDATA_LOG_METHOD is not set. Using journal.");
            method = NDLM_JOURNAL;
        }
        else {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING, "NETDATA_LOG_METHOD is not set. Using stderr.");
            method = NDLM_STDERR;
        }
    }

    switch(method) {
        case NDLM_JOURNAL:
            if(!nd_log_journal_direct_init(getenv("NETDATA_SYSTEMD_JOURNAL_PATH")) ||
               !nd_log_journal_direct_init(NULL) || !nd_log_journal_systemd_init()) {
                nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to initialize journal. Using stderr.");
                method = NDLM_STDERR;
            }
            break;

        case NDLM_SYSLOG:
            nd_log_syslog_init();
            break;

        default:
            method = NDLM_STDERR;
            break;
    }

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].method = method;
        nd_log.sources[i].format = format;
        nd_log.sources[i].fd = -1;
        nd_log.sources[i].fp = NULL;
    }

//    nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "FINAL_LOG_METHOD: %s", nd_log_id2method(method));
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
    if(e->method == NDLM_DEFAULT)
        nd_log_set_user_settings(source, e->filename);

    if((e->method == NDLM_FILE && !e->filename) ||
       (e->method == NDLM_DEVNULL && e->fd == -1))
        e->method = NDLM_DISABLED;

    if(e->fp)
        fflush(e->fp);

    switch(e->method) {
        case NDLM_SYSLOG:
            nd_log_syslog_init();
            break;

        case NDLM_JOURNAL:
            nd_log_journal_direct_init(NULL);
            nd_log_journal_systemd_init();
            break;

        case NDLM_STDOUT:
            e->fp = stdout;
            e->fd = STDOUT_FILENO;
            break;

        case NDLM_DISABLED:
            break;

        case NDLM_DEFAULT:
        case NDLM_STDERR:
            e->method = NDLM_STDERR;
            e->fp = stderr;
            e->fd = STDERR_FILENO;
            break;

        case NDLM_DEVNULL:
        case NDLM_FILE: {
            int fd = open(e->filename, O_WRONLY | O_APPEND | O_CREAT, 0664);
            if(fd == -1) {
                if(e->fd != STDOUT_FILENO && e->fd != STDERR_FILENO) {
                    e->fd = STDERR_FILENO;
                    e->method = NDLM_STDERR;
                    netdata_log_error("Cannot open log file '%s'. Falling back to stderr.", e->filename);
                }
                else
                    netdata_log_error("Cannot open log file '%s'. Leaving fd %d as-is.", e->filename, e->fd);
            }
            else {
                if (!nd_log_replace_existing_fd(e, fd)) {
                    if(e->fd == STDOUT_FILENO || e->fd == STDERR_FILENO) {
                        if(e->fd == STDOUT_FILENO)
                            e->method = NDLM_STDOUT;
                        else if(e->fd == STDERR_FILENO)
                            e->method = NDLM_STDERR;

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
static void timestamp_usec_annotator(BUFFER *wb, const char *key, struct log_field *lf);

// ----------------------------------------------------------------------------

typedef void (*annotator_t)(BUFFER *wb, const char *key, struct log_field *lf);

struct log_field {
    const char *journal;
    const char *logfmt;
    annotator_t logfmt_annotator;
    struct log_stack_entry entry;
};

#define THREAD_LOG_STACK_MAX 50

static __thread struct log_stack_entry *thread_log_stack_base[THREAD_LOG_STACK_MAX];
static __thread size_t thread_log_stack_next = 0;

static __thread struct log_field thread_log_fields[_NDF_MAX] = {
        // THE ORDER DEFINES THE ORDER FIELDS WILL APPEAR IN logfmt

        [NDF_STOP] = { // processing will not stop on this - so it is ok to be first
                .journal = NULL,
                .logfmt = NULL,
                .logfmt_annotator = NULL,
        },
        [NDF_TIMESTAMP_REALTIME_USEC] = {
                .journal = NULL,
                .logfmt = "time",
                .logfmt_annotator = timestamp_usec_annotator,
        },
        [NDF_SYSLOG_IDENTIFIER] = {
                .journal = "SYSLOG_IDENTIFIER", // standard journald field
                .logfmt = "comm",
        },
        [NDF_LOG_SOURCE] = {
                .journal = "ND_LOG_SOURCE",
                .logfmt = "source",
        },
        [NDF_PRIORITY] = {
                .journal = "PRIORITY", // standard journald field
                .logfmt = "level",
                .logfmt_annotator = priority_annotator,
        },
        [NDF_ERRNO] = {
                .journal = "ERRNO", // standard journald field
                .logfmt = "errno",
                .logfmt_annotator = errno_annotator,
        },
        [NDF_INVOCATION_ID] = {
                .journal = "INVOCATION_ID", // standard journald field
                .logfmt = NULL,
        },
        [NDF_LINE] = {
                .journal = "CODE_LINE", // standard journald field
                .logfmt = NULL,
        },
        [NDF_FILE] = {
                .journal = "CODE_FILE", // standard journald field
                .logfmt = NULL,
        },
        [NDF_FUNC] = {
                .journal = "CODE_FUNC", // standard journald field
                .logfmt = NULL,
        },
        [NDF_TID] = {
                .journal = "TID", // standard journald field
                .logfmt = "tid",
        },
        [NDF_THREAD_TAG] = {
                .journal = "THREAD_TAG",
                .logfmt = "thread",
        },
        [NDF_MESSAGE_ID] = {
                .journal = "MESSAGE_ID",
                .logfmt = "msg_id",
        },
        [NDF_MODULE] = {
                .journal = "ND_MODULE",
                .logfmt = "module",
        },
        [NDF_NIDL_NODE] = {
                .journal = "ND_NIDL_NODE",
                .logfmt = "node",
        },
        [NDF_NIDL_INSTANCE] = {
                .journal = "ND_NIDL_INSTANCE",
                .logfmt = "instance",
        },
        [NDF_NIDL_CONTEXT] = {
                .journal = "ND_NIDL_CONTEXT",
                .logfmt = "context",
        },
        [NDF_NIDL_DIMENSION] = {
                .journal = "ND_NIDL_DIMENSION",
                .logfmt = "dimension",
        },
        [NDF_SRC_TRANSPORT] = {
                .journal = "ND_SRC_TRANSPORT",
                .logfmt = "src_transport",
        },
        [NDF_ACCOUNT_ID] = {
            .journal = "ND_ACCOUNT_ID",
            .logfmt = "account",
        },
        [NDF_USER_NAME] = {
            .journal = "ND_USER_NAME",
            .logfmt = "user",
        },
        [NDF_USER_ROLE] = {
            .journal = "ND_USER_ROLE",
            .logfmt = "role",
        },
        [NDF_USER_ACCESS] = {
            .journal = "ND_USER_PERMISSIONS",
            .logfmt = "permissions",
        },
        [NDF_SRC_IP] = {
            .journal = "ND_SRC_IP",
            .logfmt = "src_ip",
        },
        [NDF_SRC_FORWARDED_HOST] = {
                .journal = "ND_SRC_FORWARDED_HOST",
                .logfmt = "src_forwarded_host",
        },
        [NDF_SRC_FORWARDED_FOR] = {
                .journal = "ND_SRC_FORWARDED_FOR",
                .logfmt = "src_forwarded_for",
        },
        [NDF_SRC_PORT] = {
                .journal = "ND_SRC_PORT",
                .logfmt = "src_port",
        },
        [NDF_SRC_CAPABILITIES] = {
                .journal = "ND_SRC_CAPABILITIES",
                .logfmt = "src_capabilities",
        },
        [NDF_DST_TRANSPORT] = {
                .journal = "ND_DST_TRANSPORT",
                .logfmt = "dst_transport",
        },
        [NDF_DST_IP] = {
                .journal = "ND_DST_IP",
                .logfmt = "dst_ip",
        },
        [NDF_DST_PORT] = {
                .journal = "ND_DST_PORT",
                .logfmt = "dst_port",
        },
        [NDF_DST_CAPABILITIES] = {
                .journal = "ND_DST_CAPABILITIES",
                .logfmt = "dst_capabilities",
        },
        [NDF_REQUEST_METHOD] = {
                .journal = "ND_REQUEST_METHOD",
                .logfmt = "req_method",
        },
        [NDF_RESPONSE_CODE] = {
                .journal = "ND_RESPONSE_CODE",
                .logfmt = "code",
        },
        [NDF_CONNECTION_ID] = {
                .journal = "ND_CONNECTION_ID",
                .logfmt = "conn",
        },
        [NDF_TRANSACTION_ID] = {
                .journal = "ND_TRANSACTION_ID",
                .logfmt = "transaction",
        },
        [NDF_RESPONSE_SENT_BYTES] = {
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
        [NDF_ALERT_ID] = {
                .journal = "ND_ALERT_ID",
                .logfmt = "alert_id",
        },
        [NDF_ALERT_UNIQUE_ID] = {
                .journal = "ND_ALERT_UNIQUE_ID",
                .logfmt = "alert_unique_id",
        },
        [NDF_ALERT_TRANSITION_ID] = {
                .journal = "ND_ALERT_TRANSITION_ID",
                .logfmt = "alert_transition_id",
        },
        [NDF_ALERT_EVENT_ID] = {
                .journal = "ND_ALERT_EVENT_ID",
                .logfmt = "alert_event_id",
        },
        [NDF_ALERT_CONFIG_HASH] = {
                .journal = "ND_ALERT_CONFIG",
                .logfmt = "alert_config",
        },
        [NDF_ALERT_NAME] = {
                .journal = "ND_ALERT_NAME",
                .logfmt = "alert",
        },
        [NDF_ALERT_CLASS] = {
                .journal = "ND_ALERT_CLASS",
                .logfmt = "alert_class",
        },
        [NDF_ALERT_COMPONENT] = {
                .journal = "ND_ALERT_COMPONENT",
                .logfmt = "alert_component",
        },
        [NDF_ALERT_TYPE] = {
                .journal = "ND_ALERT_TYPE",
                .logfmt = "alert_type",
        },
        [NDF_ALERT_EXEC] = {
                .journal = "ND_ALERT_EXEC",
                .logfmt = "alert_exec",
        },
        [NDF_ALERT_RECIPIENT] = {
                .journal = "ND_ALERT_RECIPIENT",
                .logfmt = "alert_recipient",
        },
        [NDF_ALERT_VALUE] = {
                .journal = "ND_ALERT_VALUE",
                .logfmt = "alert_value",
        },
        [NDF_ALERT_VALUE_OLD] = {
                .journal = "ND_ALERT_VALUE_OLD",
                .logfmt = "alert_value_old",
        },
        [NDF_ALERT_STATUS] = {
                .journal = "ND_ALERT_STATUS",
                .logfmt = "alert_status",
        },
        [NDF_ALERT_STATUS_OLD] = {
                .journal = "ND_ALERT_STATUS_OLD",
                .logfmt = "alert_value_old",
        },
        [NDF_ALERT_UNITS] = {
                .journal = "ND_ALERT_UNITS",
                .logfmt = "alert_units",
        },
        [NDF_ALERT_SUMMARY] = {
                .journal = "ND_ALERT_SUMMARY",
                .logfmt = "alert_summary",
        },
        [NDF_ALERT_INFO] = {
                .journal = "ND_ALERT_INFO",
                .logfmt = "alert_info",
        },
        [NDF_ALERT_DURATION] = {
                .journal = "ND_ALERT_DURATION",
                .logfmt = "alert_duration",
        },
        [NDF_ALERT_NOTIFICATION_REALTIME_USEC] = {
                .journal = "ND_ALERT_NOTIFICATION_TIMESTAMP_USEC",
                .logfmt = "alert_notification_timestamp",
                .logfmt_annotator = timestamp_usec_annotator,
        },

        // put new items here
        // leave the request URL and the message last

        [NDF_REQUEST] = {
                .journal = "ND_REQUEST",
                .logfmt = "request",
        },
        [NDF_MESSAGE] = {
                .journal = "MESSAGE",
                .logfmt = "msg",
        },
};

#define THREAD_FIELDS_MAX (sizeof(thread_log_fields) / sizeof(thread_log_fields[0]))

ND_LOG_FIELD_ID nd_log_field_id_by_name(const char *field, size_t len) {
    for(size_t i = 0; i < THREAD_FIELDS_MAX ;i++) {
        if(thread_log_fields[i].journal && strlen(thread_log_fields[i].journal) == len && strncmp(field, thread_log_fields[i].journal, len) == 0)
            return i;
    }

    return NDF_STOP;
}

void log_stack_pop(void *ptr) {
    if(!ptr) return;

    struct log_stack_entry *lgs = *(struct log_stack_entry (*)[])ptr;

    if(unlikely(!thread_log_stack_next || lgs != thread_log_stack_base[thread_log_stack_next - 1])) {
        fatal("You cannot pop in the middle of the stack, or an item not in the stack");
        return;
    }

    thread_log_stack_next--;
}

void log_stack_push(struct log_stack_entry *lgs) {
    if(!lgs || thread_log_stack_next >= THREAD_LOG_STACK_MAX) return;
    thread_log_stack_base[thread_log_stack_next++] = lgs;
}

// ----------------------------------------------------------------------------
// json formatter

static void nd_logger_json(BUFFER *wb, struct log_field *fields, size_t fields_max) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        const char *key = fields[i].logfmt;

        const char *s = NULL;
        switch(fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                buffer_json_member_add_uint64(wb, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                buffer_json_member_add_int64(wb, key, fields[i].entry.i64);
                break;
            case NDFT_DBL:
                buffer_json_member_add_double(wb, key, fields[i].entry.dbl);
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    buffer_json_member_add_string(wb, key, u);
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
            }
                break;
            default:
                s = "UNHANDLED";
                break;
        }

        if(s && *s)
            buffer_json_member_add_string(wb, key, s);
    }

    buffer_json_finalize(wb);
}

// ----------------------------------------------------------------------------
// logfmt formatter


static int64_t log_field_to_int64(struct log_field *lf) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *tmp = NULL;
    const char *s = NULL;

    switch(lf->entry.type) {
        case NDFT_UUID:
        case NDFT_UNSET:
            return 0;

        case NDFT_TXT:
            s = lf->entry.txt;
            break;

        case NDFT_STR:
            s = string2str(lf->entry.str);
            break;

        case NDFT_BFR:
            s = buffer_tostring(lf->entry.bfr);
            break;

        case NDFT_CALLBACK:
            tmp = buffer_create(0, NULL);

            if(lf->entry.cb.formatter(tmp, lf->entry.cb.formatter_data))
                s = buffer_tostring(tmp);
            else
                s = NULL;
            break;

        case NDFT_U64:
            return (int64_t)lf->entry.u64;

        case NDFT_I64:
            return (int64_t)lf->entry.i64;

        case NDFT_DBL:
            return (int64_t)lf->entry.dbl;
    }

    if(s && *s)
        return str2ll(s, NULL);

    return 0;
}

static uint64_t log_field_to_uint64(struct log_field *lf) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *tmp = NULL;
    const char *s = NULL;

    switch(lf->entry.type) {
        case NDFT_UUID:
        case NDFT_UNSET:
            return 0;

        case NDFT_TXT:
            s = lf->entry.txt;
            break;

        case NDFT_STR:
            s = string2str(lf->entry.str);
            break;

        case NDFT_BFR:
            s = buffer_tostring(lf->entry.bfr);
            break;

        case NDFT_CALLBACK:
            tmp = buffer_create(0, NULL);

            if(lf->entry.cb.formatter(tmp, lf->entry.cb.formatter_data))
                s = buffer_tostring(tmp);
            else
                s = NULL;
            break;

        case NDFT_U64:
            return lf->entry.u64;

        case NDFT_I64:
            return lf->entry.i64;

        case NDFT_DBL:
            return (uint64_t) lf->entry.dbl;
    }

    if(s && *s)
        return str2uint64_t(s, NULL);

    return 0;
}

static void timestamp_usec_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    usec_t ut = log_field_to_uint64(lf);

    if(!ut)
        return;

    char datetime[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(datetime, sizeof(datetime), ut, 3, false);

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);
    buffer_json_strcat(wb, datetime);
}

static void errno_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    int64_t errnum = log_field_to_int64(lf);

    if(errnum == 0)
        return;

    char buf[1024];
    const char *s = errno2str((int)errnum, buf, sizeof(buf));

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
    uint64_t pri = log_field_to_uint64(lf);

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);
    buffer_strcat(wb, nd_log_id2priority(pri));
}

static bool needs_quotes_for_logfmt(const char *s)
{
    static bool safe_for_logfmt[256] = {
            [' '] =  true, ['!'] =  true, ['"'] =  false, ['#'] =  true, ['$'] =  true, ['%'] =  true, ['&'] =  true,
            ['\''] = true, ['('] =  true, [')'] =  true, ['*'] =  true, ['+'] =  true, [','] =  true, ['-'] =  true,
            ['.'] =  true, ['/'] =  true, ['0'] =  true, ['1'] =  true, ['2'] =  true, ['3'] =  true, ['4'] =  true,
            ['5'] =  true, ['6'] =  true, ['7'] =  true, ['8'] =  true, ['9'] =  true, [':'] =  true, [';'] =  true,
            ['<'] =  true, ['='] =  true, ['>'] =  true, ['?'] =  true, ['@'] =  true, ['A'] =  true, ['B'] =  true,
            ['C'] =  true, ['D'] =  true, ['E'] =  true, ['F'] =  true, ['G'] =  true, ['H'] =  true, ['I'] =  true,
            ['J'] =  true, ['K'] =  true, ['L'] =  true, ['M'] =  true, ['N'] =  true, ['O'] =  true, ['P'] =  true,
            ['Q'] =  true, ['R'] =  true, ['S'] =  true, ['T'] =  true, ['U'] =  true, ['V'] =  true, ['W'] =  true,
            ['X'] =  true, ['Y'] =  true, ['Z'] =  true, ['['] =  true, ['\\'] = false, [']'] =  true, ['^'] =  true,
            ['_'] =  true, ['`'] =  true, ['a'] =  true, ['b'] =  true, ['c'] =  true, ['d'] =  true, ['e'] =  true,
            ['f'] =  true, ['g'] =  true, ['h'] =  true, ['i'] =  true, ['j'] =  true, ['k'] =  true, ['l'] =  true,
            ['m'] =  true, ['n'] =  true, ['o'] =  true, ['p'] =  true, ['q'] =  true, ['r'] =  true, ['s'] =  true,
            ['t'] =  true, ['u'] =  true, ['v'] =  true, ['w'] =  true, ['x'] =  true, ['y'] =  true, ['z'] =  true,
            ['{'] =  true, ['|'] =  true, ['}'] =  true, ['~'] =  true, [0x7f] = true,
    };

    if(!*s)
        return true;

    while(*s) {
        if(*s == '=' || isspace(*s) || !safe_for_logfmt[(uint8_t)*s])
            return true;

        s++;
    }

    return false;
}

static void string_to_logfmt(BUFFER *wb, const char *s)
{
    bool spaces = needs_quotes_for_logfmt(s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);

    buffer_json_strcat(wb, s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);
}

static void nd_logger_logfmt(BUFFER *wb, struct log_field *fields, size_t fields_max)
{

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        const char *key = fields[i].logfmt;

        if(fields[i].logfmt_annotator)
            fields[i].logfmt_annotator(wb, key, &fields[i]);
        else {
            if(buffer_strlen(wb))
                buffer_fast_strcat(wb, " ", 1);

            switch(fields[i].entry.type) {
                case NDFT_TXT:
                    if(*fields[i].entry.txt) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, fields[i].entry.txt);
                    }
                    break;
                case NDFT_STR:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    string_to_logfmt(wb, string2str(fields[i].entry.str));
                    break;
                case NDFT_BFR:
                    if(buffer_strlen(fields[i].entry.bfr)) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, buffer_tostring(fields[i].entry.bfr));
                    }
                    break;
                case NDFT_U64:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_uint64(wb, fields[i].entry.u64);
                    break;
                case NDFT_I64:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_int64(wb, fields[i].entry.i64);
                    break;
                case NDFT_DBL:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_netdata_double(wb, fields[i].entry.dbl);
                    break;
                case NDFT_UUID:
                    if(!uuid_is_null(*fields[i].entry.uuid)) {
                        char u[UUID_COMPACT_STR_LEN];
                        uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        buffer_fast_strcat(wb, u, sizeof(u) - 1);
                    }
                    break;
                case NDFT_CALLBACK: {
                    if(!tmp)
                        tmp = buffer_create(1024, NULL);
                    else
                        buffer_flush(tmp);
                    if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data)) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, buffer_tostring(tmp));
                    }
                }
                    break;
                default:
                    buffer_strcat(wb, "UNHANDLED");
                    break;
            }
        }
    }
}

// ----------------------------------------------------------------------------
// journal logger

bool nd_log_journal_socket_available(void) {
    if(netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        char filename[FILENAME_MAX + 1];

        snprintfz(filename, sizeof(filename), "%s%s",
                  netdata_configured_host_prefix, "/run/systemd/journal/socket");

        if(is_path_unix_socket(filename))
            return true;
    }

    return is_path_unix_socket("/run/systemd/journal/socket");
}

static bool nd_logger_journal_libsystemd(struct log_field *fields, size_t fields_max) {
#ifdef HAVE_SYSTEMD

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    struct iovec iov[fields_max];
    int iov_count = 0;

    memset(iov, 0, sizeof(iov));

    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].journal)
            continue;

        const char *key = fields[i].journal;
        char *value = NULL;
        int rc = 0;
        switch (fields[i].entry.type) {
            case NDFT_TXT:
                if(*fields[i].entry.txt)
                    rc = asprintf(&value, "%s=%s", key, fields[i].entry.txt);
                break;
            case NDFT_STR:
                rc = asprintf(&value, "%s=%s", key, string2str(fields[i].entry.str));
                break;
            case NDFT_BFR:
                if(buffer_strlen(fields[i].entry.bfr))
                    rc = asprintf(&value, "%s=%s", key, buffer_tostring(fields[i].entry.bfr));
                break;
            case NDFT_U64:
                rc = asprintf(&value, "%s=%" PRIu64, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                rc = asprintf(&value, "%s=%" PRId64, key, fields[i].entry.i64);
                break;
            case NDFT_DBL:
                rc = asprintf(&value, "%s=%f", key, fields[i].entry.dbl);
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    rc = asprintf(&value, "%s=%s", key, u);
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    rc = asprintf(&value, "%s=%s", key, buffer_tostring(tmp));
            }
                break;
            default:
                rc = asprintf(&value, "%s=%s", key, "UNHANDLED");
                break;
        }

        if (rc != -1 && value) {
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

static bool nd_logger_journal_direct(struct log_field *fields, size_t fields_max) {
    if(!nd_log.journal_direct.initialized)
        return false;

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *wb = buffer_create(4096, NULL);
    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].journal)
            continue;

        const char *key = fields[i].journal;

        const char *s = NULL;
        switch(fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_uint64(wb, fields[i].entry.u64);
                buffer_putc(wb, '\n');
                break;
            case NDFT_I64:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_int64(wb, fields[i].entry.i64);
                buffer_putc(wb, '\n');
                break;
            case NDFT_DBL:
                buffer_strcat(wb, key);
                buffer_putc(wb, '=');
                buffer_print_netdata_double(wb, fields[i].entry.dbl);
                buffer_putc(wb, '\n');
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    buffer_strcat(wb, key);
                    buffer_putc(wb, '=');
                    buffer_fast_strcat(wb, u, sizeof(u) - 1);
                    buffer_putc(wb, '\n');
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
            }
                break;
            default:
                s = "UNHANDLED";
                break;
        }

        if(s && *s) {
            buffer_strcat(wb, key);
            if(!strchr(s, '\n')) {
                buffer_putc(wb, '=');
                buffer_strcat(wb, s);
                buffer_putc(wb, '\n');
            }
            else {
                buffer_putc(wb, '\n');
                size_t size = strlen(s);
                uint64_t le_size = htole64(size);
                buffer_memcat(wb, &le_size, sizeof(le_size));
                buffer_memcat(wb, s, size);
                buffer_putc(wb, '\n');
            }
        }
    }

    return journal_direct_send(nd_log.journal_direct.fd, buffer_tostring(wb), buffer_strlen(wb));
}

// ----------------------------------------------------------------------------
// syslog logger - uses logfmt

static bool nd_logger_syslog(int priority, ND_LOG_FORMAT format __maybe_unused, struct log_field *fields, size_t fields_max) {
    CLEAN_BUFFER *wb = buffer_create(1024, NULL);

    nd_logger_logfmt(wb, fields, fields_max);
    syslog(priority, "%s", buffer_tostring(wb));

    return true;
}

// ----------------------------------------------------------------------------
// file logger - uses logfmt

static bool nd_logger_file(FILE *fp, ND_LOG_FORMAT format, struct log_field *fields, size_t fields_max) {
    BUFFER *wb = buffer_create(1024, NULL);

    if(format == NDLF_JSON)
        nd_logger_json(wb, fields, fields_max);
    else
        nd_logger_logfmt(wb, fields, fields_max);

    int r = fprintf(fp, "%s\n", buffer_tostring(wb));
    fflush(fp);

    buffer_free(wb);
    return r > 0;
}

// ----------------------------------------------------------------------------
// logger router

static ND_LOG_METHOD nd_logger_select_output(ND_LOG_SOURCES source, FILE **fpp, SPINLOCK **spinlock) {
    *spinlock = NULL;
    ND_LOG_METHOD output = nd_log.sources[source].method;

    switch(output) {
        case NDLM_JOURNAL:
            if(unlikely(!nd_log.journal_direct.initialized && !nd_log.journal.initialized)) {
                output = NDLM_FILE;
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = NULL;
                *spinlock = NULL;
            }
            break;

        case NDLM_SYSLOG:
            if(unlikely(!nd_log.syslog.initialized)) {
                output = NDLM_FILE;
                *spinlock = &nd_log.std_error.spinlock;
                *fpp = stderr;
            }
            else {
                *spinlock = NULL;
                *fpp = NULL;
            }
            break;

        case NDLM_FILE:
            if(!nd_log.sources[source].fp) {
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = nd_log.sources[source].fp;
                *spinlock = &nd_log.sources[source].spinlock;
            }
            break;

        case NDLM_STDOUT:
            output = NDLM_FILE;
            *fpp = stdout;
            *spinlock = &nd_log.std_output.spinlock;
            break;

        default:
        case NDLM_DEFAULT:
        case NDLM_STDERR:
            output = NDLM_FILE;
            *fpp = stderr;
            *spinlock = &nd_log.std_error.spinlock;
            break;

        case NDLM_DISABLED:
        case NDLM_DEVNULL:
            output = NDLM_DISABLED;
            *fpp = NULL;
            *spinlock = NULL;
            break;
    }

    return output;
}

// ----------------------------------------------------------------------------
// high level logger

static void nd_logger_log_fields(SPINLOCK *spinlock, FILE *fp, bool limit, ND_LOG_FIELD_PRIORITY priority,
                                 ND_LOG_METHOD output, struct nd_log_source *source,
                                 struct log_field *fields, size_t fields_max) {
    if(spinlock)
        spinlock_lock(spinlock);

    // check the limits
    if(limit && nd_log_limit_reached(source))
        goto cleanup;

    if(output == NDLM_JOURNAL) {
        if(!nd_logger_journal_direct(fields, fields_max) && !nd_logger_journal_libsystemd(fields, fields_max)) {
            // we can't log to journal, let's log to stderr
            if(spinlock)
                spinlock_unlock(spinlock);

            output = NDLM_FILE;
            spinlock = &nd_log.std_error.spinlock;
            fp = stderr;

            if(spinlock)
                spinlock_lock(spinlock);
        }
    }

    if(output == NDLM_SYSLOG)
        nd_logger_syslog(priority, source->format, fields, fields_max);

    if(output == NDLM_FILE)
        nd_logger_file(fp, source->format, fields, fields_max);


cleanup:
    if(spinlock)
        spinlock_unlock(spinlock);
}

static void nd_logger_unset_all_thread_fields(void) {
    size_t fields_max = THREAD_FIELDS_MAX;
    for(size_t i = 0; i < fields_max ; i++)
        thread_log_fields[i].entry.set = false;
}

static void nd_logger_merge_log_stack_to_thread_fields(void) {
    for(size_t c = 0; c < thread_log_stack_next ;c++) {
        struct log_stack_entry *lgs = thread_log_stack_base[c];

        for(size_t i = 0; lgs[i].id != NDF_STOP ; i++) {
            if(lgs[i].id >= _NDF_MAX || !lgs[i].set)
                continue;

            struct log_stack_entry *e = &lgs[i];
            ND_LOG_STACK_FIELD_TYPE type = lgs[i].type;

            // do not add empty / unset fields
            if((type == NDFT_TXT && (!e->txt || !*e->txt)) ||
                (type == NDFT_BFR && (!e->bfr || !buffer_strlen(e->bfr))) ||
                (type == NDFT_STR && !e->str) ||
                (type == NDFT_UUID && (!e->uuid || uuid_is_null(*e->uuid))) ||
                (type == NDFT_CALLBACK && !e->cb.formatter) ||
                type == NDFT_UNSET)
                continue;

            thread_log_fields[lgs[i].id].entry = *e;
        }
    }
}

static void nd_logger(const char *file, const char *function, const unsigned long line,
               ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, bool limit, int saved_errno,
               const char *fmt, va_list ap) {

    SPINLOCK *spinlock;
    FILE *fp;
    ND_LOG_METHOD output = nd_logger_select_output(source, &fp, &spinlock);
    if(output != NDLM_FILE && output != NDLM_JOURNAL && output != NDLM_SYSLOG)
        return;

    // mark all fields as unset
    nd_logger_unset_all_thread_fields();

    // flatten the log stack into the fields
    nd_logger_merge_log_stack_to_thread_fields();

    // set the common fields that are automatically set by the logging subsystem

    if(likely(!thread_log_fields[NDF_INVOCATION_ID].entry.set))
        thread_log_fields[NDF_INVOCATION_ID].entry = ND_LOG_FIELD_UUID(NDF_INVOCATION_ID, &nd_log.invocation_id);

    if(likely(!thread_log_fields[NDF_LOG_SOURCE].entry.set))
        thread_log_fields[NDF_LOG_SOURCE].entry = ND_LOG_FIELD_TXT(NDF_LOG_SOURCE, nd_log_id2source(source));
    else {
        ND_LOG_SOURCES src = source;

        if(thread_log_fields[NDF_LOG_SOURCE].entry.type == NDFT_TXT)
            src = nd_log_source2id(thread_log_fields[NDF_LOG_SOURCE].entry.txt, source);
        else if(thread_log_fields[NDF_LOG_SOURCE].entry.type == NDFT_U64)
            src = thread_log_fields[NDF_LOG_SOURCE].entry.u64;

        if(src != source && src < _NDLS_MAX) {
            source = src;
            output = nd_logger_select_output(source, &fp, &spinlock);
            if(output != NDLM_FILE && output != NDLM_JOURNAL && output != NDLM_SYSLOG)
                return;
        }
    }

    if(likely(!thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry.set))
        thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry = ND_LOG_FIELD_TXT(NDF_SYSLOG_IDENTIFIER, program_name);

    if(likely(!thread_log_fields[NDF_LINE].entry.set)) {
        thread_log_fields[NDF_LINE].entry = ND_LOG_FIELD_U64(NDF_LINE, line);
        thread_log_fields[NDF_FILE].entry = ND_LOG_FIELD_TXT(NDF_FILE, file);
        thread_log_fields[NDF_FUNC].entry = ND_LOG_FIELD_TXT(NDF_FUNC, function);
    }

    if(likely(!thread_log_fields[NDF_PRIORITY].entry.set)) {
        thread_log_fields[NDF_PRIORITY].entry = ND_LOG_FIELD_U64(NDF_PRIORITY, priority);
    }

    if(likely(!thread_log_fields[NDF_TID].entry.set))
        thread_log_fields[NDF_TID].entry = ND_LOG_FIELD_U64(NDF_TID, gettid());

    char os_threadname[NETDATA_THREAD_NAME_MAX + 1];
    if(likely(!thread_log_fields[NDF_THREAD_TAG].entry.set)) {
        const char *thread_tag = netdata_thread_tag();
        if(!netdata_thread_tag_exists()) {
            if (!netdata_thread_tag_exists()) {
                os_thread_get_current_name_np(os_threadname);
                if ('\0' != os_threadname[0])
                    /* If it is not an empty string replace "MAIN" thread_tag */
                    thread_tag = os_threadname;
            }
        }
        thread_log_fields[NDF_THREAD_TAG].entry = ND_LOG_FIELD_TXT(NDF_THREAD_TAG, thread_tag);

        // TODO: fix the ND_MODULE in logging by setting proper module name in threads
//        if(!thread_log_fields[NDF_MODULE].entry.set)
//            thread_log_fields[NDF_MODULE].entry = ND_LOG_FIELD_CB(NDF_MODULE, thread_tag_to_module, (void *)thread_tag);
    }

    if(likely(!thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry.set))
        thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry = ND_LOG_FIELD_U64(NDF_TIMESTAMP_REALTIME_USEC, now_realtime_usec());

    if(saved_errno != 0 && !thread_log_fields[NDF_ERRNO].entry.set)
        thread_log_fields[NDF_ERRNO].entry = ND_LOG_FIELD_I64(NDF_ERRNO, saved_errno);

    CLEAN_BUFFER *wb = NULL;
    if(fmt && !thread_log_fields[NDF_MESSAGE].entry.set) {
        wb = buffer_create(1024, NULL);
        buffer_vsprintf(wb, fmt, ap);
        thread_log_fields[NDF_MESSAGE].entry = ND_LOG_FIELD_TXT(NDF_MESSAGE, buffer_tostring(wb));
    }

    nd_logger_log_fields(spinlock, fp, limit, priority, output, &nd_log.sources[source],
                         thread_log_fields, THREAD_FIELDS_MAX);

    if(nd_log.sources[source].pending_msg) {
        // log a pending message

        nd_logger_unset_all_thread_fields();

        thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_U64,
                .u64 = now_realtime_usec(),
        };

        thread_log_fields[NDF_LOG_SOURCE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log_id2source(source),
        };

        thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = program_name,
        };

        thread_log_fields[NDF_MESSAGE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log.sources[source].pending_msg,
        };

        nd_logger_log_fields(spinlock, fp, false, priority, output,
                             &nd_log.sources[source],
                             thread_log_fields, THREAD_FIELDS_MAX);

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
        source = nd_log.overwrite_process_source;

    return source;
}

// ----------------------------------------------------------------------------
// public API for loggers

void netdata_logger(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... )
{
    int saved_errno = errno;
    source = nd_log_validate_source(source);

    if((source == NDLS_DAEMON || source == NDLS_COLLECTORS) && priority > nd_log.sources[source].min_priority)
        return;

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority,
              source == NDLS_DAEMON || source == NDLS_COLLECTORS,
              saved_errno, fmt, args);
    va_end(args);
}

void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
    int saved_errno = errno;
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
            saved_errno, fmt, args);
    va_end(args);
    erl->last_logged = now;
    erl->count = 0;
}

void netdata_logger_fatal( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    int saved_errno = errno;
    ND_LOG_SOURCES source = NDLS_DAEMON;
    source = nd_log_validate_source(source);

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, NDLP_ALERT, true, saved_errno, fmt, args);
    va_end(args);

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    char action_data[70+1];
    snprintfz(action_data, 70, "%04lu@%-10.10s:%-15.15s/%d", line, file, function, saved_errno);

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
    if(!thread_tag)
        thread_tag = "UNKNOWN";

    const char *tag_to_send =  thread_tag;

    // anonymize thread names
    if(strncmp(thread_tag, THREAD_TAG_STREAM_RECEIVER, strlen(THREAD_TAG_STREAM_RECEIVER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_RECEIVER;
    if(strncmp(thread_tag, THREAD_TAG_STREAM_SENDER, strlen(THREAD_TAG_STREAM_SENDER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_SENDER;

    char action_result[60+1];
    snprintfz(action_result, 60, "%s:%s", program_name, tag_to_send);

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

    netdata_cleanup_and_exit(1, "FATAL", action_result, action_data);
}

// ----------------------------------------------------------------------------
// log limits

void nd_log_limits_reset(void) {
    usec_t now_ut = now_monotonic_usec();

    spinlock_lock(&nd_log.std_output.spinlock);
    spinlock_lock(&nd_log.std_error.spinlock);

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        spinlock_lock(&nd_log.sources[i].spinlock);
        nd_log.sources[i].limits.prevented = 0;
        nd_log.sources[i].limits.counter = 0;
        nd_log.sources[i].limits.started_monotonic_ut = now_ut;
        nd_log.sources[i].limits.logs_per_period = nd_log.sources[i].limits.logs_per_period_backup;
        spinlock_unlock(&nd_log.sources[i].spinlock);
    }

    spinlock_unlock(&nd_log.std_output.spinlock);
    spinlock_unlock(&nd_log.std_error.spinlock);
}

void nd_log_limits_unlimited(void) {
    nd_log_limits_reset();
    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].limits.logs_per_period = 0;
    }
}

static bool nd_log_limit_reached(struct nd_log_source *source) {
    if(source->limits.throttle_period == 0 || source->limits.logs_per_period == 0)
        return false;

    usec_t now_ut = now_monotonic_usec();
    if(!source->limits.started_monotonic_ut)
        source->limits.started_monotonic_ut = now_ut;

    source->limits.counter++;

    if(now_ut - source->limits.started_monotonic_ut > (usec_t)source->limits.throttle_period) {
        if(source->limits.prevented) {
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                           "LOG FLOOD PROTECTION: resuming logging "
                           "(prevented %"PRIu32" logs in the last %"PRIu32" seconds).",
                           source->limits.prevented,
                           source->limits.throttle_period);

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

    if(source->limits.counter > source->limits.logs_per_period) {
        if(!source->limits.prevented) {
            BUFFER *wb = buffer_create(1024, NULL);
            buffer_sprintf(wb,
                    "LOG FLOOD PROTECTION: too many logs (%"PRIu32" logs in %"PRId64" seconds, threshold is set to %"PRIu32" logs "
                    "in %"PRIu32" seconds). Preventing more logs from process '%s' for %"PRId64" seconds.",
                    source->limits.counter,
                    (int64_t)((now_ut - source->limits.started_monotonic_ut) / USEC_PER_SEC),
                    source->limits.logs_per_period,
                    source->limits.throttle_period,
                    program_name,
                    (int64_t)(((source->limits.started_monotonic_ut + (source->limits.throttle_period * USEC_PER_SEC) - now_ut)) / USEC_PER_SEC)
            );

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
