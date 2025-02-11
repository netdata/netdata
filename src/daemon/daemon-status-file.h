// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_STATUS_FILE_H
#define NETDATA_DAEMON_STATUS_FILE_H

#include "libnetdata/libnetdata.h"

typedef enum {
    DAEMON_STATUS_NONE,
    DAEMON_STATUS_INITIALIZING,
    DAEMON_STATUS_RUNNING,
    DAEMON_STATUS_EXITING,
    DAEMON_STATUS_EXITED,
} DAEMON_STATUS;

ENUM_STR_DEFINE_FUNCTIONS_EXTERN(DAEMON_STATUS);

typedef struct daemon_status_file {
    char version[32];       // the netdata version
    time_t boottime;        // system boottime
    time_t uptime;          // netdata uptime
    time_t timestamp;       // the timestamp of the status file
    ND_UUID invocation;     // the netdata invocation id generated the file
    EXIT_REASON reason;     // the exit reason (maybe empty)
    DAEMON_STATUS status;   // the daemon status

    struct {
        long line;
        const char *filename;
        const char *function;
        const char *stack_trace;
    } fatal;
} DAEMON_STATUS_FILE;

// returns the current status
DAEMON_STATUS_FILE daemon_status_file_get(DAEMON_STATUS status);

// loads the last status saved
DAEMON_STATUS_FILE daemon_status_file_load(void);

// saves the current status
void daemon_status_file_save(DAEMON_STATUS status);

// check for a crash
void daemon_status_file_check_crash(void);

bool daemon_status_file_has_last_crashed(void);
bool daemon_status_file_was_incomplete_shutdown(void);

#endif //NETDATA_DAEMON_STATUS_FILE_H
