// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

size_t appconfig_foreach_value_in_section(struct config *root, const char *section, appconfig_foreach_value_cb_t cb, void *data) {
    size_t used = 0;
    struct section *co = appconfig_section_find(root, section);
    if(co) {
        config_section_wrlock(co);
        struct config_option *cv;
        for(cv = co->values; cv ; cv = cv->next) {
            if(cb(data, string2str(cv->name), string2str(cv->value))) {
                cv->flags |= CONFIG_VALUE_USED;
                used++;
            }
        }
        config_section_unlock(co);
    }

    return used;
}

bool stream_conf_needs_dbengine(struct config *root) {
    struct section *co;
    bool ret = false;

    appconfig_wrlock(root);
    for(co = root->first_section; co; co = co->next) {
        if(string_strcmp(co->name, "stream") == 0)
            continue; // the first section is not relevant

        const char *s;

        s = appconfig_get_value_of_option_in_section(co, "enabled", NULL, NULL);
        if(!s || !appconfig_test_boolean_value(s))
            continue;

        s = appconfig_get_value_of_option_in_section(co, "db", NULL, NULL);
        if(s && strcmp(s, "dbengine") == 0) {
            ret = true;
            break;
        }
    }
    appconfig_unlock(root);

    return ret;
}

bool stream_conf_has_uuid_section(struct config *root) {
    struct section *section = NULL;
    bool is_parent = false;

    appconfig_wrlock(root);
    for (section = root->first_section; section; section = section->next) {
        nd_uuid_t uuid;

        if (uuid_parse(string2str(section->name), uuid) != -1 &&
            appconfig_get_boolean_by_section(section, "enabled", 0)) {
            is_parent = true;
            break;
        }
    }
    appconfig_unlock(root);

    return is_parent;
}
