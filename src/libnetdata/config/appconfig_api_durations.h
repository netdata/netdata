// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_API_DURATIONS_H
#define NETDATA_APPCONFIG_API_DURATIONS_H

msec_t appconfig_get_duration_ms(struct config *root, const char *section, const char *name, msec_t default_value);
msec_t appconfig_set_duration_ms(struct config *root, const char *section, const char *name, msec_t value);
#define config_get_duration_ms(section, name, value) appconfig_get_duration_ms(&netdata_config, section, name, value)
#define config_set_duration_ms(section, name, value) appconfig_set_duration_ms(&netdata_config, section, name, value)

time_t appconfig_get_duration_seconds(struct config *root, const char *section, const char *name, time_t default_value);
time_t appconfig_set_duration_seconds(struct config *root, const char *section, const char *name, time_t value);
#define config_get_duration_seconds(section, name, value) appconfig_get_duration_seconds(&netdata_config, section, name, value)
#define config_set_duration_seconds(section, name, value) appconfig_set_duration_seconds(&netdata_config, section, name, value)

unsigned appconfig_get_duration_days(struct config *root, const char *section, const char *name, unsigned default_value);
unsigned appconfig_set_duration_days(struct config *root, const char *section, const char *name, unsigned value);
#define config_get_duration_days(section, name, value) appconfig_get_duration_days(&netdata_config, section, name, value)
#define config_set_duration_days(section, name, value) appconfig_set_duration_days(&netdata_config, section, name, value)

#endif //NETDATA_APPCONFIG_API_DURATIONS_H
