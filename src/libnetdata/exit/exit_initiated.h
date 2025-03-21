// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXIT_INITIATED_H
#define NETDATA_EXIT_INITIATED_H

#include "../common.h"
#include "../template-enum.h"

typedef enum {
    EXIT_REASON_NONE                = 0,

    // signals - abnormal termination
    EXIT_REASON_SIGBUS              = (1 << 0),
    EXIT_REASON_SIGSEGV             = (1 << 1),
    EXIT_REASON_SIGFPE              = (1 << 2),
    EXIT_REASON_SIGILL              = (1 << 3),
    EXIT_REASON_SIGABRT             = (1 << 4),
    EXIT_REASON_SIGSYS              = (1 << 5),  // Bad system call
    EXIT_REASON_SIGXCPU             = (1 << 6),  // CPU time limit exceeded
    EXIT_REASON_SIGXFSZ             = (1 << 7),  // File size limit exceeded
    EXIT_REASON_OUT_OF_MEMORY       = (1 << 8),
    EXIT_REASON_ALREADY_RUNNING     = (1 << 9),

    // abnormal termination via a fatal message
    EXIT_REASON_FATAL               = (1 << 10),     // a fatal message

    // normal termination via APIs
    EXIT_REASON_API_QUIT            = (1 << 11),     // developer only
    EXIT_REASON_CMD_EXIT            = (1 << 12),     // netdatacli

    // signals - normal termination
    EXIT_REASON_SIGQUIT             = (1 << 13),     // rare, but graceful
    EXIT_REASON_SIGTERM             = (1 << 14),     // received on Linux, FreeBSD, MacOS
    EXIT_REASON_SIGINT              = (1 << 15),    // received on Windows on normal termination

    // windows specific, service stop
    EXIT_REASON_SERVICE_STOP        = (1 << 16),

    // automatically detect when exit_initiated_set() is called
    // supports Linux, FreeBSD, MacOS, Windows
    EXIT_REASON_SYSTEM_SHUTDOWN     = (1 << 17),

    // netdata update
    EXIT_REASON_UPDATE              = (1 << 18),

    // timeout while shutting down
    EXIT_REASON_SHUTDOWN_TIMEOUT    = (1 << 19),
} EXIT_REASON;

#define EXIT_REASON_NORMAL              \
    (                                   \
          EXIT_REASON_SIGINT            \
        | EXIT_REASON_SIGTERM           \
        | EXIT_REASON_SIGQUIT           \
        | EXIT_REASON_API_QUIT          \
        | EXIT_REASON_CMD_EXIT          \
        | EXIT_REASON_SERVICE_STOP      \
        | EXIT_REASON_SYSTEM_SHUTDOWN   \
        | EXIT_REASON_UPDATE            \
    )

#define EXIT_REASON_DEADLY_SIGNAL       \
    (                                   \
          EXIT_REASON_SIGBUS            \
        | EXIT_REASON_SIGSEGV           \
        | EXIT_REASON_SIGFPE            \
        | EXIT_REASON_SIGILL            \
        | EXIT_REASON_SIGSYS            \
        | EXIT_REASON_SIGXCPU           \
        | EXIT_REASON_SIGXFSZ           \
    )

#define EXIT_REASON_ABNORMAL            \
    (                                   \
          EXIT_REASON_DEADLY_SIGNAL     \
        | EXIT_REASON_SIGABRT           \
        | EXIT_REASON_FATAL             \
        | EXIT_REASON_ALREADY_RUNNING   \
        | EXIT_REASON_OUT_OF_MEMORY     \
        | EXIT_REASON_SHUTDOWN_TIMEOUT  \
    )

#define is_deadly_signal(reason) ((reason) & (EXIT_REASON_DEADLY_SIGNAL))
#define is_exit_reason_normal(reason) (((reason == EXIT_REASON_NONE) || ((reason) & EXIT_REASON_NORMAL)) && !((reason) & EXIT_REASON_ABNORMAL))

typedef struct web_buffer BUFFER;
BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(EXIT_REASON);

void exit_initiated_init(void);
EXIT_REASON exit_initiated_get(void);
void exit_initiated_set(EXIT_REASON reason);
void exit_initiated_add(EXIT_REASON reason);

#endif //NETDATA_EXIT_INITIATED_H
