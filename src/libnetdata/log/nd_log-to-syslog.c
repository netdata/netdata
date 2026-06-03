// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if !defined(OS_WINDOWS)

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

#else // OS_WINDOWS

// Windows has no syslog daemon and no <syslog.h>. The Windows-native
// logging channel is nd_log-to-windows-events.c (Windows Event Log + ETW),
// selected by nd_log-init.c on OS_WINDOWS. nd_log-config.c rejects
// 'output = syslog' at config-parse time on Windows with a fatal error,
// so these stubs are truly unreachable; the internal_fatal() guards catch
// any code path that bypasses the config gate (dev/internal-checks builds
// abort loudly, production builds fall through safely).

void nd_log_init_syslog(void) {
    internal_fatal(true,
                   "syslog channel is not supported on Windows; "
                   "nd_log-config.c should have rejected NDLM_SYSLOG at parse time");
}

bool nd_logger_syslog(int priority __maybe_unused,
                      ND_LOG_FORMAT format __maybe_unused,
                      struct log_field *fields __maybe_unused,
                      size_t fields_max __maybe_unused) {
    internal_fatal(true,
                   "syslog channel is not supported on Windows; "
                   "nd_log-config.c should have rejected NDLM_SYSLOG at parse time");
    return false;
}

#endif // OS_WINDOWS
