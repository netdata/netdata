// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_text.h"

const char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = appconfig_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, NULL);
    if(!opt)
        return default_value;

    return string2str(opt->value);
}

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value) {
    struct config_option *opt = appconfig_set_raw_value(root, section, name, value, CONFIG_VALUE_TYPE_TEXT);
    return string2str(opt->value);
}
