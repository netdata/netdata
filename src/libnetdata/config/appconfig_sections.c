// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

// ----------------------------------------------------------------------------
// config sections index

int appconfig_section_compare(void *a, void *b) {
    if(((struct section *)a)->name < ((struct section *)b)->name) return -1;
    else if(((struct section *)a)->name > ((struct section *)b)->name) return 1;
    else return string_cmp(((struct section *)a)->name, ((struct section *)b)->name);
}

struct section *appconfig_section_find(struct config *root, const char *name) {
    struct section tmp;
    tmp.name = string_strdupz(name);

    struct section *rc = (struct section *)avl_search_lock(&root->index, (avl_t *) &tmp);
    string_freez(tmp.name);
    return rc;
}

// ----------------------------------------------------------------------------
// config section methods

struct section *appconfig_section_create(struct config *root, const char *section) {
    netdata_log_debug(D_CONFIG, "Creating section '%s'.", section);

    struct section *co = callocz(1, sizeof(struct section));
    co->name = string_strdupz(section);
    netdata_mutex_init(&co->mutex);

    avl_init_lock(&co->values_index, appconfig_option_compare);

    if(unlikely(appconfig_section_add(root, co) != co))
        netdata_log_error("INTERNAL ERROR: indexing of section '%s', already exists.",
                          string2str(co->name));

    appconfig_wrlock(root);
    struct section *co2 = root->last_section;
    if(co2) {
        co2->next = co;
    } else {
        root->first_section = co;
    }
    root->last_section = co;
    appconfig_unlock(root);

    return co;
}

// ----------------------------------------------------------------------------

const char *appconfig_get_value_of_option_in_section(struct section *co, const char *option, const char *default_value, reformat_t cb) {
    struct config_option *cv;

    // Only calls internal to this file check for a NULL result, and they do not supply a NULL arg.
    // External caller should treat NULL as an error case.
    cv = appconfig_option_find(co, option);
    if (!cv) {
        if (!default_value) return NULL;
        cv = appconfig_option_create(co, option, default_value);
        if (!cv) return NULL;
    }
    cv->flags |= CONFIG_VALUE_USED;

    if((cv->flags & CONFIG_VALUE_LOADED) || (cv->flags & CONFIG_VALUE_CHANGED)) {
        // this is a loaded value from the config file
        // if it is different from the default, mark it
        if(!(cv->flags & CONFIG_VALUE_CHECKED)) {
            if(default_value && string_strcmp(cv->value, default_value) != 0)
                cv->flags |= CONFIG_VALUE_CHANGED;

            cv->flags |= CONFIG_VALUE_CHECKED;
        }
    }

    if((cv->flags & CONFIG_VALUE_MIGRATED) && !(cv->flags & CONFIG_VALUE_REFORMATTED) && cb) {
        if(!cv->value_reformatted)
            cv->value_reformatted = string_dup(cv->value);

        cv->value = cb(cv->value);
        cv->flags |= CONFIG_VALUE_REFORMATTED;
    }

    return string2str(cv->value);
}
