// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

void appconfig_section_destroy_non_loaded(struct config *root, const char *section)
{
    struct section *co;
    struct config_option *cv, *cv_next;

    netdata_log_debug(D_CONFIG, "Destroying section '%s'.", section);

    co = appconfig_section_find(root, section);
    if(!co) {
        netdata_log_error("Could not destroy section '%s'. Not found.", section);
        return;
    }

    config_section_wrlock(co);
    for(cv = co->values; cv ; cv = cv->next) {
        if (cv->flags & CONFIG_VALUE_LOADED) {
            /* Do not destroy values that were loaded from the configuration files. */
            config_section_unlock(co);
            return;
        }
    }
    for(cv = co->values ; cv ; cv = cv_next) {
        cv_next = cv->next;
        if(unlikely(!appconfig_option_del(co, cv)))
            netdata_log_error("Cannot remove config option '%s' from section '%s'.",
                              string2str(cv->name), string2str(co->name));
        string_freez(cv->value);
        string_freez(cv->name);
        freez(cv);
    }
    co->values = NULL;
    config_section_unlock(co);

    if (unlikely(!appconfig_section_del(root, co))) {
        netdata_log_error("Cannot remove section '%s' from config.", section);
        return;
    }

    appconfig_wrlock(root);

    if (root->first_section == co) {
        root->first_section = co->next;

        if (root->last_section == co)
            root->last_section = root->first_section;
    } else {
        struct section *co_cur = root->first_section, *co_prev = NULL;

        while(co_cur && co_cur != co) {
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

    avl_destroy_lock(&co->values_index);
    string_freez(co->name);
    pthread_mutex_destroy(&co->mutex);
    freez(co);
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

    if (co->values == cv) {
        co->values = co->values->next;
    } else {
        struct config_option *cv_cur = co->values, *cv_prev = NULL;
        while (cv_cur && cv_cur != cv) {
            cv_prev = cv_cur;
            cv_cur = cv_cur->next;
        }
        if (cv_cur) {
            cv_prev->next = cv_cur->next;
        }
    }

    string_freez(cv->value);
    string_freez(cv->name);
    freez(cv);

    config_section_unlock(co);
}

