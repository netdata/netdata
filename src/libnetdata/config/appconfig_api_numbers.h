// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_API_NUMBERS_H
#define NETDATA_APPCONFIG_API_NUMBERS_H

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value);
long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value);
#define config_get_number(section, name, value) appconfig_get_number(&netdata_config, section, name, value)
#define config_set_number(section, name, value) appconfig_set_number(&netdata_config, section, name, value)

NETDATA_DOUBLE appconfig_get_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value);
NETDATA_DOUBLE appconfig_set_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value);
#define config_get_double(section, name, value) appconfig_get_double(&netdata_config, section, name, value)
#define config_set_double(section, name, value) appconfig_set_float(&netdata_config, section, name, value)

#endif //NETDATA_APPCONFIG_API_NUMBERS_H
