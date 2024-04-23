// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_GET_PID_MAX_H
#define NETDATA_GET_PID_MAX_H

#include <unistd.h>

extern pid_t pid_max;
pid_t os_get_system_pid_max(void);

#endif //NETDATA_GET_PID_MAX_H
