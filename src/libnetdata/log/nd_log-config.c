// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

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
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
            else if(strcmp(name, ETW_NAME) == 0)
                ls->format = NDLF_ETW;
#endif
#if defined(HAVE_WEL)
                else if(strcmp(name, WEL_NAME) == 0)
                ls->format = NDLF_WEL;
#endif
#endif
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

                        int period;
                        if(!duration_parse_seconds(slash, &period)) {
                            nd_log(NDLS_DAEMON, NDLP_ERR, "Error while parsing period '%s'", slash);
                            period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
                        }

                        ls->limits.throttle_period = period;
                    }
                    else {
                        ls->limits.logs_per_period = ls->limits.logs_per_period_backup = str2u(value);
                        ls->limits.throttle_period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
                    }
                }
            }
            else
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "Error while parsing configuration of log source '%s'. "
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
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    else if(strcmp(output, ETW_NAME) == 0) {
        ls->method = NDLM_ETW;
        ls->filename = NULL;
    }
#endif
#if defined(HAVE_WEL)
        else if(strcmp(output, WEL_NAME) == 0) {
        ls->method = NDLM_WEL;
        ls->filename = NULL;
    }
#endif
#endif
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

        ND_LOG_METHOD method = NDLM_STDERR;
        ND_LOG_FORMAT format = NDLF_LOGFMT;
        ND_LOG_FIELD_PRIORITY priority = ls->min_priority;

        if(IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(ls->method)) {
            method = ls->method;
            format = ls->format;
        }

        nd_setenv("NETDATA_LOG_METHOD", nd_log_id2method(method), 1);
        nd_setenv("NETDATA_LOG_FORMAT", nd_log_id2format(format), 1);
        nd_setenv("NETDATA_LOG_LEVEL", nd_log_id2priority(priority), 1);
    }
}

void nd_log_set_priority_level(const char *setting) {
    if(!setting || !*setting)
        setting = "info";

    ND_LOG_FIELD_PRIORITY priority = nd_log_priority2id(setting);

#if defined(NETDATA_INTERNAL_CHECKS) || defined(NETDATA_DEV_MODE)
    priority = NDLP_DEBUG;
#endif

    for (size_t i = 0; i < _NDLS_MAX; i++) {
        if (i != NDLS_DEBUG)
            nd_log.sources[i].min_priority = priority;
    }

    // the right one
    nd_setenv("NETDATA_LOG_LEVEL", nd_log_id2priority(priority), 1);
}

void nd_log_set_facility(const char *facility) {
    if(!facility || !*facility)
        facility = "daemon";

    nd_log.syslog.facility = nd_log_facility2id(facility);
    nd_setenv("NETDATA_SYSLOG_FACILITY", nd_log_id2facility(nd_log.syslog.facility), 1);
}

void nd_log_set_flood_protection(size_t logs, time_t period) {
    // daemon logs
    nd_log.sources[NDLS_DAEMON].limits.logs_per_period = logs;
    nd_log.sources[NDLS_DAEMON].limits.logs_per_period_backup = logs;
    nd_log.sources[NDLS_DAEMON].limits.throttle_period = period;

    // collectors logs
    nd_log.sources[NDLS_COLLECTORS].limits.logs_per_period = logs;
    nd_log.sources[NDLS_COLLECTORS].limits.logs_per_period_backup = logs;
    nd_log.sources[NDLS_COLLECTORS].limits.throttle_period = period;

    char buf[100];
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t)period);
    nd_setenv("NETDATA_ERRORS_THROTTLE_PERIOD", buf, 1);
    snprintfz(buf, sizeof(buf), "%" PRIu64, (uint64_t)logs);
    nd_setenv("NETDATA_ERRORS_PER_PERIOD", buf, 1);
}
