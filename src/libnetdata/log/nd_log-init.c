// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

// --------------------------------------------------------------------------------------------------------------------

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
    nd_setenv("NETDATA_INVOCATION_ID", uuid, 1);
}

ND_UUID nd_log_get_invocation_id(void) {
    ND_UUID rc;
    uuid_copy(rc.uuid, nd_log.invocation_id);
    return rc;
}

// --------------------------------------------------------------------------------------------------------------------

void nd_log_initialize_for_external_plugins(const char *name) {
    // if we don't run under Netdata, log to stderr,
    // otherwise, use the logging method Netdata wants us to use.
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    nd_setenv("NETDATA_LOG_METHOD", ETW_NAME, 0);
    nd_setenv("NETDATA_LOG_FORMAT", ETW_NAME, 0);
#elif defined(HAVE_WEL)
    nd_setenv("NETDATA_LOG_METHOD", WEL_NAME, 0);
    nd_setenv("NETDATA_LOG_FORMAT", WEL_NAME, 0);
#else
    nd_setenv("NETDATA_LOG_METHOD", "stderr", 0);
    nd_setenv("NETDATA_LOG_FORMAT", "logfmt", 0);
#endif
#else
    nd_setenv("NETDATA_LOG_METHOD", "stderr", 0);
    nd_setenv("NETDATA_LOG_FORMAT", "logfmt", 0);
#endif

    nd_log.overwrite_process_source = NDLS_COLLECTORS;
    program_name = name;

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        nd_log.sources[i].method = NDLM_DEFAULT;
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
            if(!nd_log_journal_direct_init(getenv("NETDATA_SYSTEMD_JOURNAL_PATH")) &&
                !nd_log_journal_direct_init(NULL) && !nd_log_journal_systemd_init()) {
                nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to initialize journal. Using stderr.");
                method = NDLM_STDERR;
            }
            break;

#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
        case NDLM_ETW:
            if(!nd_log_init_etw()) {
                nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to initialize Events Tracing for Windows (ETW). Using stderr.");
                method = NDLM_STDERR;
            }
        break;
#endif
#if defined(HAVE_WEL)
            case NDLM_WEL:
            if(!nd_log_init_wel()) {
                nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to initialize Windows Event Log (WEL). Using stderr.");
                method = NDLM_STDERR;
            }
        break;
#endif
#endif

        case NDLM_SYSLOG:
            nd_log_init_syslog();
            break;

        default:
            method = NDLM_STDERR;
            break;
    }

    nd_log.sources[NDLS_COLLECTORS].method = method;
    nd_log.sources[NDLS_COLLECTORS].format = format;
    nd_log.sources[NDLS_COLLECTORS].fd = -1;
    nd_log.sources[NDLS_COLLECTORS].fp = NULL;

    //    nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "FINAL_LOG_METHOD: %s", nd_log_id2method(method));

#if defined(HAVE_LIBBACKTRACE)
    stacktrace_init();
#endif
}

// --------------------------------------------------------------------------------------------------------------------

void nd_log_open(struct nd_log_source *e, ND_LOG_SOURCES source) {
    if(e->method == NDLM_DEFAULT)
        nd_log_set_user_settings(source, e->filename);

    if((e->method == NDLM_FILE && !e->filename) ||
        (e->method == NDLM_DEVNULL && e->fd == -1))
        e->method = NDLM_DISABLED;

    if(e->fp)
        fflush(e->fp);

    switch(e->method) {
        case NDLM_SYSLOG:
            nd_log_init_syslog();
            break;

        case NDLM_JOURNAL:
            nd_log_journal_direct_init(NULL);
            nd_log_journal_systemd_init();
            break;

#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
        case NDLM_ETW:
            nd_log_init_etw();
            break;
#endif
#if defined(HAVE_WEL)
            case NDLM_WEL:
            nd_log_init_wel();
            break;
#endif
#endif

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

// --------------------------------------------------------------------------------------------------------------------

void nd_log_stdin_init(int fd, const char *filename) {
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

#if defined(HAVE_LIBBACKTRACE)
    stacktrace_init();
#endif
}

void nd_log_reopen_log_files(bool log) {
    if(log)
        netdata_log_info("Reopening all log files.");

    nd_log_initialize();

    if(log)
        netdata_log_info("Log files re-opened.");
}

int nd_log_systemd_journal_fd(void) {
    return nd_log.journal.fd;
}

void nd_log_reopen_log_files_for_spawn_server(const char *name) {
    nd_log.fatal_hook_cb = NULL;
    nd_log.fatal_final_cb = NULL;

    gettid_uncached();
#if defined(HAVE_LIBBACKTRACE)
    stacktrace_flush();
    stacktrace_forked();
#endif

    if(nd_log.syslog.initialized) {
        closelog();
        nd_log.syslog.initialized = false;
        nd_log_init_syslog();
    }

    if(nd_log.journal_direct.initialized) {
        close(nd_log.journal_direct.fd);
        nd_log.journal_direct.fd = -1;
        nd_log.journal_direct.initialized = false;
    }

    for(size_t i = 0; i < _NDLS_MAX ;i++) {
        spinlock_init(&nd_log.sources[i].spinlock);
        nd_log.sources[i].method = NDLM_DEFAULT;
        nd_log.sources[i].fd = -1;
        nd_log.sources[i].fp = NULL;
        nd_log.sources[i].pending_msg = NULL;
#if defined(OS_WINDOWS)
        nd_log.sources[i].hEventLog = NULL;
#endif
    }

    // initialize spinlocks
    spinlock_init(&nd_log.std_output.spinlock);
    spinlock_init(&nd_log.std_error.spinlock);

    nd_log.syslog.initialized = false;
    nd_log.eventlog.initialized = false;
    nd_log.std_output.initialized = false;
    nd_log.std_error.initialized = false;

    nd_log_initialize_for_external_plugins(name);
}
