// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

size_t appconfig_foreach_value_in_section(struct config *root, const char *section, appconfig_foreach_value_cb_t cb, void *data) {
    size_t used = 0;
    struct config_section *co = appconfig_section_find(root, section);
    if(co) {
        SECTION_LOCK(co);
        struct config_option *cv;
        for(cv = co->values; cv ; cv = cv->next) {
            if(cb(data, string2str(cv->name), string2str(cv->value))) {
                cv->flags |= CONFIG_VALUE_USED;
                used++;
            }
        }
        SECTION_UNLOCK(co);
    }

    return used;
}

bool stream_conf_needs_dbengine(struct config *root) {
    struct config_section *sect;
    bool ret = false;

    APPCONFIG_LOCK(root);
    for(sect = root->sections; sect; sect = sect->next) {
        if(string_strcmp(sect->name, "stream") == 0)
            continue; // the first section is not relevant

        const char *s;

        s = appconfig_get_value_of_option_in_section(sect, "enabled", NULL, NULL, CONFIG_VALUE_TYPE_UNKNOWN);
        if(!s || !appconfig_test_boolean_value(s))
            continue;

        s = appconfig_get_value_of_option_in_section(sect, "db", NULL, NULL, CONFIG_VALUE_TYPE_UNKNOWN);
        if(s && strcmp(s, "dbengine") == 0) {
            ret = true;
            break;
        }
    }
    APPCONFIG_UNLOCK(root);

    return ret;
}

bool stream_conf_has_uuid_section(struct config *root) {
    struct config_section *sect = NULL;
    bool is_parent = false;

    APPCONFIG_LOCK(root);
    for (sect = root->sections; sect; sect = sect->next) {
        nd_uuid_t uuid;

        if (uuid_parse(string2str(sect->name), uuid) != -1 &&
            appconfig_get_boolean_by_section(sect, "enabled", 0)) {
            is_parent = true;
            break;
        }
    }
    APPCONFIG_UNLOCK(root);

    return is_parent;
}
