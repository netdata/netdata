// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_API_BOOLEAN_H
#define NETDATA_APPCONFIG_API_BOOLEAN_H

#define CONFIG_BOOLEAN_INVALID 100  // an invalid value to check for validity (used as default initialization when needed)

#define CONFIG_BOOLEAN_NO   0       // disabled
#define CONFIG_BOOLEAN_YES  1       // enabled

#ifndef CONFIG_BOOLEAN_AUTO
#define CONFIG_BOOLEAN_AUTO 2       // enabled if it has useful info when enabled
#endif

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value);
#define config_get_boolean(section, name, value) appconfig_get_boolean(&netdata_config, section, name, value)

int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value);
#define config_get_boolean_ondemand(section, name, value) appconfig_get_boolean_ondemand(&netdata_config, section, name, value)

int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value);
#define config_set_boolean(section, name, value) appconfig_set_boolean(&netdata_config, section, name, value)

#endif //NETDATA_APPCONFIG_API_BOOLEAN_H
