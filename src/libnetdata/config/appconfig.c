// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

int appconfig_exists(struct config *root, const char *section, const char *name) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect) return 0;

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt) return 0;

    return 1;
}

void appconfig_set_default_raw_value(struct config *root, const char *section, const char *name, const char *value) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect) {
        appconfig_set_raw_value(root, section, name, value, CONFIG_VALUE_TYPE_UNKNOWN);
        return;
    }

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt) {
        appconfig_set_raw_value(root, section, name, value, CONFIG_VALUE_TYPE_UNKNOWN);
        return;
    }

    opt->flags |= CONFIG_VALUE_USED;

    if(opt->flags & CONFIG_VALUE_LOADED)
        return;

    if(string_strcmp(opt->value, value) != 0) {
        opt->flags |= CONFIG_VALUE_CHANGED;

        string_freez(opt->value);
        opt->value = string_strdupz(value);
    }
}

bool stream_conf_needs_dbengine(struct config *root) {
    struct config_section *sect;
    bool ret = false;

    APPCONFIG_LOCK(root);
    for(sect = root->sections; sect; sect = sect->next) {
        if(string_strcmp(sect->name, "stream") == 0)
            continue; // the first section is not relevant

        struct config_option *opt = appconfig_get_raw_value_of_option_in_section(sect, "enabled", NULL, CONFIG_VALUE_TYPE_UNKNOWN, NULL);
        if(!opt || !appconfig_test_boolean_value(string2str(opt->value)))
            continue;

        opt = appconfig_get_raw_value_of_option_in_section(sect, "db", NULL, CONFIG_VALUE_TYPE_UNKNOWN, NULL);
        if(opt && string_strcmp(opt->value, "dbengine") == 0) {
            ret = true;
            break;
        }
    }
    APPCONFIG_UNLOCK(root);

    return ret;
}

bool stream_conf_has_api_enabled(struct config *root) {
    struct config_section *sect = NULL;
    struct config_option *opt;
    bool is_parent = false;

    APPCONFIG_LOCK(root);
    for (sect = root->sections; sect; sect = sect->next) {
        nd_uuid_t uuid;

        if (uuid_parse(string2str(sect->name), uuid) != 0)
            continue;

        opt = appconfig_option_find(sect, "type");
        // when the 'type' is missing, we assume it is 'api'
        if(opt && string_strcmp(opt->value, "api") != 0)
            continue;

        opt = appconfig_option_find(sect, "enabled");
        // when the 'enabled' is missing, we assume it is 'false'
        if(!opt || !appconfig_test_boolean_value(string2str(opt->value)))
            continue;

        is_parent = true;
        break;
    }
    APPCONFIG_UNLOCK(root);

    return is_parent;
}

void appconfig_foreach_section(struct config *root, void (*cb)(struct config *root, const char *name, void *data), void *data) {
    struct config_section *sect = NULL;

    APPCONFIG_LOCK(root);
    for (sect = root->sections; sect; sect = sect->next) {
        cb(root, string2str(sect->name), data);
    }
    APPCONFIG_UNLOCK(root);
}
