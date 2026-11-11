// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_TIMEZONE_H
#define NETDATA_DAEMON_TIMEZONE_H 1

#include <stdbool.h>
#include <stddef.h>

void get_system_timezone(void);
bool system_timezone_is_user_configured(void);
bool system_timezone_is_tzdb_name(void);
const char *detect_system_timezone_name(char *buffer, size_t buffer_size);
int timezone_windows_mapping_unittest(void);
bool timezone_name_is_safe_tzdb_path(const char *timezone);
void refresh_system_timezone(const char *timezone, bool is_tzdb_name);

#endif //NETDATA_DAEMON_TIMEZONE_H
