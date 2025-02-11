// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXIT_INITIATED_H
#define NETDATA_EXIT_INITIATED_H

#include "../common.h"
#include "../template-enum.h"

typedef enum {
    EXIT_REASON_NONE                = 0,
    EXIT_REASON_SYSTEM_SHUTDOWN     = (1 << 0),
    EXIT_REASON_SIGINT              = (1 << 1),
    EXIT_REASON_SIGQUIT             = (1 << 2),
    EXIT_REASON_SIGTERM             = (1 << 3),
    EXIT_REASON_SIGBUS              = (1 << 4),
    EXIT_REASON_SIGSEGV             = (1 << 5),
    EXIT_REASON_SIGFPE              = (1 << 6),
    EXIT_REASON_SIGILL              = (1 << 7),
    EXIT_REASON_API_QUIT            = (1 << 7),
    EXIT_REASON_CMD_EXIT            = (1 << 8),
    EXIT_REASON_FATAL               = (1 << 9),
    EXIT_REASON_SERVICE_STOP        = (1 << 10),
} EXIT_REASON;

typedef struct web_buffer BUFFER;
BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(EXIT_REASON);

extern volatile EXIT_REASON exit_initiated;

void exit_initiated_set(EXIT_REASON reason);

#endif //NETDATA_EXIT_INITIATED_H
