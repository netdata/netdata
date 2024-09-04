// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_numbers.h"

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    struct config_option *opt = appconfig_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    return strtoll(s, NULL, 0);
}

NETDATA_DOUBLE appconfig_get_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    struct config_option *opt = appconfig_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_DOUBLE, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    return str2ndd(s, NULL);
}

long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    appconfig_set_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER);
    return value;
}

NETDATA_DOUBLE appconfig_set_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    appconfig_set_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_DOUBLE);
    return value;
}

