// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

void appconfig_section_destroy_non_loaded(struct config *root, const char *section)
{
    struct section *sect;
    struct config_option *opt;

    netdata_log_debug(D_CONFIG, "Destroying section '%s'.", section);

    sect = appconfig_section_find(root, section);
    if(!sect) {
        netdata_log_error("Could not destroy section '%s'. Not found.", section);
        return;
    }

    config_section_wrlock(sect);

    // find if there is any loaded option
    for(opt = sect->values; opt; opt = opt->next) {
        if (opt->flags & CONFIG_VALUE_LOADED) {
            // do not destroy values that were loaded from the configuration files.
            config_section_unlock(sect);
            return;
        }
    }

    // no option is loaded, free them all
    while(sect->values)
        appconfig_option_remove_and_delete(sect, sect->values, true);

    config_section_unlock(sect);

    if (unlikely(!appconfig_section_del(root, sect))) {
        netdata_log_error("Cannot remove section '%s' from config.", section);
        return;
    }

    appconfig_wrlock(root);

    if (root->first_section == sect) {
        root->first_section = sect->next;

        if (root->last_section == sect)
            root->last_section = root->first_section;
    } else {
        struct section *co_cur = root->first_section, *co_prev = NULL;

        while(co_cur && co_cur != sect) {
            co_prev = co_cur;
            co_cur = co_cur->next;
        }

        if (co_cur) {
            co_prev->next = co_cur->next;

            if (root->last_section == co_cur)
                root->last_section = co_prev;
        }
    }

    appconfig_unlock(root);

    avl_destroy_lock(&sect->values_index);
    string_freez(sect->name);
    pthread_mutex_destroy(&sect->mutex);
    freez(sect);
}

void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name)
{
    netdata_log_debug(D_CONFIG, "Destroying section option '%s -> %s'.", section, name);

    struct section *co;
    co = appconfig_section_find(root, section);
    if (!co) {
        netdata_log_error("Could not destroy section option '%s -> %s'. The section not found.", section, name);
        return;
    }

    config_section_wrlock(co);

    struct config_option *cv;

    cv = appconfig_option_find(co, name);

    if (cv && cv->flags & CONFIG_VALUE_LOADED) {
        config_section_unlock(co);
        return;
    }

    if (unlikely(!(cv && appconfig_option_del(co, cv)))) {
        config_section_unlock(co);
        netdata_log_error("Could not destroy section option '%s -> %s'. The option not found.", section, name);
        return;
    }

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(co->values, cv, prev, next);

    appconfig_option_free(cv);
    config_section_unlock(co);
}

