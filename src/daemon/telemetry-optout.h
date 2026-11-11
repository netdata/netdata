// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_TELEMETRY_OPTOUT_H
#define NETDATA_DAEMON_TELEMETRY_OPTOUT_H 1

#include <stdbool.h>

// False when the user opted out of telemetry: the .opt-out-from-anonymous-statistics file exists in the user
// config directory, or DISABLE_TELEMETRY is set and not empty. Evaluated once, on the first call, which
// must come after the config directories are set.
bool telemetry_enabled(void);

#endif //NETDATA_DAEMON_TELEMETRY_OPTOUT_H
