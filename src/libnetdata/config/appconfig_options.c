// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

// ----------------------------------------------------------------------------
// config options index

int appconfig_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->name < ((struct config_option *)b)->name) return -1;
    else if(((struct config_option *)a)->name > ((struct config_option *)b)->name) return 1;
    else return string_cmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

struct config_option *appconfig_option_find(struct section *co, const char *name) {
    struct config_option tmp;
    tmp.name = string_strdupz(name);

    struct config_option *rc = (struct config_option *)avl_search_lock(&(co->values_index), (avl_t *) &tmp);

    string_freez(tmp.name);
    return rc;
}

// ----------------------------------------------------------------------------
// config options methods

struct config_option *appconfig_option_create(struct section *co, const char *name, const char *value) {
    netdata_log_debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

    struct config_option *cv = callocz(1, sizeof(struct config_option));
    cv->name = string_strdupz(name);
    cv->value = string_strdupz(value);

    struct config_option *found = appconfig_option_add(co, cv);
    if(found != cv) {
        netdata_log_error("indexing of config '%s' in section '%s': already exists - using the existing one.",
                          string2str(cv->name), string2str(co->name));
        string_freez(cv->value);
        string_freez(cv->name);
        freez(cv);
        return found;
    }

    config_section_wrlock(co);
    struct config_option *cv2 = co->values;
    if(cv2) {
        while (cv2->next) cv2 = cv2->next;
        cv2->next = cv;
    }
    else co->values = cv;
    config_section_unlock(co);

    return cv;
}

const char *appconfig_get_value_and_reformat(struct config *root, const char *section, const char *option, const char *default_value, reformat_t cb) {
    struct section *co = appconfig_section_find(root, section);
    if (!co && !default_value)
        return NULL;
    if(!co) co = appconfig_section_create(root, section);

    return appconfig_get_value_of_option_in_section(co, option, default_value, cb);
}
