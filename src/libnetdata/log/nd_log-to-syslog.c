// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

void nd_log_init_syslog(void) {
    if(nd_log.syslog.initialized)
        return;

    openlog(program_name, LOG_PID, nd_log.syslog.facility);
    nd_log.syslog.initialized = true;
}

bool nd_logger_syslog(int priority, ND_LOG_FORMAT format __maybe_unused, struct log_field *fields, size_t fields_max) {
    CLEAN_BUFFER *wb = buffer_create(1024, NULL);

    nd_logger_logfmt(wb, fields, fields_max);
    syslog(priority, "%s", buffer_tostring(wb));

    return true;
}
