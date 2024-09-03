// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_text.h"

const char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value) {
    return appconfig_get_raw_value(root, section, name, default_value, NULL, CONFIG_VALUE_TYPE_TEXT);
}

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect)
        sect = appconfig_section_create(root, section);

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt)
        opt = appconfig_option_create(sect, name, value);

    opt->flags |= CONFIG_VALUE_USED;

    if(string_strcmp(opt->value, value) != 0) {
        opt->flags |= CONFIG_VALUE_CHANGED;

        string_freez(opt->value);
        opt->value = string_strdupz(value);
    }

    return value;
}

