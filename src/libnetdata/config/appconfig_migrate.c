// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *opt_old, *opt_new;
    int ret = -1;

    netdata_log_debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct section *sect_old = appconfig_section_find(root, section_old);
    if(!sect_old) return ret;

    struct section *sect_new = appconfig_section_find(root, section_new);
    if(!sect_new) sect_new = appconfig_section_create(root, section_new);

    config_section_wrlock(sect_old);
    if(sect_old != sect_new)
        config_section_wrlock(sect_new);

    opt_old = appconfig_option_find(sect_old, name_old);
    if(!opt_old) goto cleanup;

    opt_new = appconfig_option_find(sect_new, name_new);
    if(opt_new) goto cleanup;

    if(unlikely(appconfig_option_del(sect_old, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.",
                          string2str(opt_old->name), string2str(sect_old->name));

    if(sect_old->values == opt_old) {
        sect_old->values = opt_old->next;
    }
    else {
        struct config_option *t;
        for(t = sect_old->values; t && t->next != opt_old;t = t->next) ;
        if(!t || t->next != opt_old)
            netdata_log_error("INTERNAL ERROR: cannot find variable '%s' in section '%s' of the config - but it should be there.",
                              string2str(opt_old->name), string2str(sect_old->name));
        else
            t->next = opt_old->next;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "CONFIG: option '[%s].%s' has been migrated to '[%s].%s'.",
           section_old, name_old,
           section_new, name_new);

    if(!opt_old->name_migrated)
        opt_old->name_migrated = opt_old->name;

    opt_old->name = string_strdupz(name_new);
    opt_old->flags |= CONFIG_VALUE_MIGRATED;

    opt_new = opt_old;

    // link it in front of the others in the new section
    opt_new->next = sect_new->values;
    sect_new->values = opt_new;

    if(unlikely(appconfig_option_add(sect_new, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.",
                          string2str(opt_old->name), string2str(sect_new->name));

    ret = 0;

cleanup:
    if(sect_old != sect_new)
        config_section_unlock(sect_new);
    config_section_unlock(sect_old);
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

