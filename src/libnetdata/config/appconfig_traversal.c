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
