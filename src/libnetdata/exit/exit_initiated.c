// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

volatile EXIT_REASON exit_initiated = EXIT_REASON_NONE;

ENUM_STR_MAP_DEFINE(EXIT_REASON) = {
    { EXIT_REASON_SIGBUS, "signal-bus-error"},
    { EXIT_REASON_SIGSEGV, "signal-segmentation-fault"},
    { EXIT_REASON_SIGFPE, "signal-floating-point-exception"},
    { EXIT_REASON_SIGILL, "signal-illegal-instruction"},
    { EXIT_REASON_OUT_OF_MEMORY, "out-of-memory"},
    { EXIT_REASON_ALREADY_RUNNING, "already-running"},

    { EXIT_REASON_FATAL, "fatal"},

    { EXIT_REASON_API_QUIT, "api-quit"},
    { EXIT_REASON_CMD_EXIT, "cmd-exit"},

    { EXIT_REASON_SIGQUIT, "signal-quit"},
    { EXIT_REASON_SIGTERM, "signal-terminate"},
    { EXIT_REASON_SIGINT, "signal-interrupt"},

    { EXIT_REASON_SERVICE_STOP, "service-stop"},

    { EXIT_REASON_SYSTEM_SHUTDOWN, "system-shutdown"},

    { EXIT_REASON_UPDATE, "update"},

    // terminator
    {0, NULL},
};

BITMAP_STR_DEFINE_FUNCTIONS(EXIT_REASON, EXIT_REASON_NONE, "none");

#if defined(OS_LINUX)
static bool is_system_shutdown_sysv(void) {
    const char *shutdown_files[] = {
        "/etc/nologin",          // Created during shutdown
        "/etc/halt",             // SysV shutdown indicator
        "/run/nologin",          // Modern systems shutdown indicator
        NULL
    };

    for (const char **file = shutdown_files; *file != NULL; file++) {
        if (access(*file, F_OK) == 0)
            return true;
    }

    return false;
}

static bool is_system_shutdown(void) {
    return is_system_shutdown_sysv();
}
#endif

#if defined(OS_FREEBSD)
#include <sys/sysctl.h>
static bool is_system_shutdown(void) {
    int state = 0;
    size_t state_len = sizeof(state);

    if (sysctlbyname("kern.shutdown", &state, &state_len, NULL, 0) == 0)
        return state != 0;

    return false;
}
#endif

#if defined(OS_MACOS)
#include <sys/sysctl.h>
static bool is_system_shutdown(void) {
    char buf[1024];
    size_t len = sizeof(buf);

    if (sysctlbyname("kern.shutdownstate", buf, &len, NULL, 0) == 0)
        return true;

    if (access("/var/db/.SystemShutdown", F_OK) == 0)
        return true;

    return false;
}
#endif

#if defined(OS_WINDOWS)
#include <windows.h>
static bool is_system_shutdown(void) {
    return GetSystemMetrics(SM_SHUTTINGDOWN) != 0;
}
#endif

static const char *self_path = NULL;
static OS_FILE_METADATA self = { 0 };

void exit_initiated_reset(void) {
    exit_initiated = EXIT_REASON_NONE;

    freez((char *)self_path);
    self_path = os_get_process_path();
    if(self_path)
        self = os_get_file_metadata(self_path);
}

void exit_initiated_add(EXIT_REASON reason) {
    exit_initiated |= reason;
}

void exit_initiated_set(EXIT_REASON reason) {
    if(exit_initiated == EXIT_REASON_NONE && !(reason & EXIT_REASON_SYSTEM_SHUTDOWN) && is_system_shutdown())
        reason |= EXIT_REASON_SYSTEM_SHUTDOWN;

    if(exit_initiated == EXIT_REASON_NONE && self_path && OS_FILE_METADATA_OK(self)) {
        OS_FILE_METADATA self_now = os_get_file_metadata(self_path);
        if(OS_FILE_METADATA_OK(self_now) && (self_now.modified_time != self.modified_time || self_now.size_bytes != self.size_bytes))
            reason |= EXIT_REASON_UPDATE;
    }

    // we combine all of them together
    // so that if this is called multiple times,
    // we will have all of them
    exit_initiated_add(reason);
}

