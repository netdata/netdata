// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DAEMON_H
#define NETDATA_PULSE_DAEMON_H

#include "daemon/common.h"

#if defined(PULSE_INTERNALS)
void pulse_daemon_do(bool extended);
#endif

#endif //NETDATA_PULSE_DAEMON_H
