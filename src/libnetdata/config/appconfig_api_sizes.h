// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_API_SIZES_H
#define NETDATA_APPCONFIG_API_SIZES_H

uint64_t appconfig_get_size_bytes(struct config *root, const char *section, const char *name, uint64_t default_value);
uint64_t appconfig_set_size_bytes(struct config *root, const char *section, const char *name, uint64_t value);
#define config_get_size_bytes(section, name, value) appconfig_get_size_bytes(&netdata_config, section, name, value)
#define config_set_size_bytes(section, name, value) appconfig_set_size_bytes(&netdata_config, section, name, value)

uint64_t appconfig_get_size_mb(struct config *root, const char *section, const char *name, uint64_t default_value);
uint64_t appconfig_set_size_mb(struct config *root, const char *section, const char *name, uint64_t value);
#define config_get_size_mb(section, name, value) appconfig_get_size_mb(&netdata_config, section, name, value)
#define config_set_size_mb(section, name, value) appconfig_set_size_mb(&netdata_config, section, name, value)

#endif //NETDATA_APPCONFIG_API_SIZES_H
