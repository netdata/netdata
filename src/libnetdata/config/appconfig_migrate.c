// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *cv_old, *cv_new;
    int ret = -1;

    netdata_log_debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct section *co_old = appconfig_section_find(root, section_old);
    if(!co_old) return ret;

    struct section *co_new = appconfig_section_find(root, section_new);
    if(!co_new) co_new = appconfig_section_create(root, section_new);

    config_section_wrlock(co_old);
    if(co_old != co_new)
        config_section_wrlock(co_new);

    cv_old = appconfig_option_find(co_old, name_old);
    if(!cv_old) goto cleanup;

    cv_new = appconfig_option_find(co_new, name_new);
    if(cv_new) goto cleanup;

    if(unlikely(appconfig_option_del(co_old, cv_old) != cv_old))
        netdata_log_error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.",
                          string2str(cv_old->name), string2str(co_old->name));

    if(co_old->values == cv_old) {
        co_old->values = cv_old->next;
    }
    else {
        struct config_option *t;
        for(t = co_old->values; t && t->next != cv_old ;t = t->next) ;
        if(!t || t->next != cv_old)
            netdata_log_error("INTERNAL ERROR: cannot find variable '%s' in section '%s' of the config - but it should be there.",
                              string2str(cv_old->name), string2str(co_old->name));
        else
            t->next = cv_old->next;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "CONFIG: option '[%s].%s' has been migrated to '[%s].%s'.",
           section_old, name_old,
           section_new, name_new);

    string_freez(cv_old->name);
    cv_old->name = string_strdupz(name_new);
    cv_old->flags |= CONFIG_VALUE_MIGRATED;

    cv_new = cv_old;
    cv_new->next = co_new->values;
    co_new->values = cv_new;

    if(unlikely(appconfig_option_add(co_new, cv_old) != cv_old))
        netdata_log_error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.",
                          string2str(cv_old->name), string2str(co_new->name));

    ret = 0;

cleanup:
    if(co_old != co_new)
        config_section_unlock(co_new);
    config_section_unlock(co_old);
    return ret;
}

int appconfig_move_everywhere(struct config *root, const char *name_old, const char *name_new) {
    int ret = -1;
    appconfig_wrlock(root);
    struct section *co;
    for(co = root->first_section; co ; co = co->next) {
        appconfig_unlock(root);
        if(appconfig_move(root, string2str(co->name), name_old, string2str(co->name), name_new) == 0)
            ret = 0;
        appconfig_wrlock(root);
    }
    appconfig_unlock(root);
    return ret;
}

