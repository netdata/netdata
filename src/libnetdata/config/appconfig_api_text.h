// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_API_TEXT_H
#define NETDATA_APPCONFIG_API_TEXT_H

const char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value);
const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value);
#define config_get(section, name, default_value) appconfig_get(&netdata_config, section, name, default_value)
#define config_set(section, name, default_value) appconfig_set(&netdata_config, section, name, default_value)


#endif //NETDATA_APPCONFIG_API_TEXT_H
