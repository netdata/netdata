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

// --------------------------------------------------------------------------------------------------------------------

void nd_log_initialize_for_external_plugins(const char *name) {
    // if we don't run under Netdata, log to stderr,
    // otherwise, use the logging method Netdata wants us to use.
    nd_setenv("NETDATA_LOG_METHOD", "stderr", 0);
    nd_setenv("NETDATA_LOG_FORMAT", "logfmt", 0);

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
}

void nd_log_reopen_log_files(bool log) {
    if(log)
        netdata_log_info("Reopening all log files.");

    nd_log.std_output.initialized = false;
    nd_log.std_error.initialized = false;
    nd_log_initialize();

    if(log)
        netdata_log_info("Log files re-opened.");
}

void nd_log_reopen_log_files_for_spawn_server(void) {
    if(nd_log.syslog.initialized) {
        closelog();
        nd_log.syslog.initialized = false;
        nd_log_syslog_init();
    }

    if(nd_log.journal_direct.initialized) {
        close(nd_log.journal_direct.fd);
        nd_log.journal_direct.fd = -1;
        nd_log.journal_direct.initialized = false;
        nd_log_journal_direct_init(NULL);
    }

    nd_log.sources[NDLS_UNSET].method = NDLM_DISABLED;
    nd_log.sources[NDLS_ACCESS].method = NDLM_DISABLED;
    nd_log.sources[NDLS_ACLK].method = NDLM_DISABLED;
    nd_log.sources[NDLS_DEBUG].method = NDLM_DISABLED;
    nd_log.sources[NDLS_HEALTH].method = NDLM_DISABLED;
    nd_log_reopen_log_files(false);
}
