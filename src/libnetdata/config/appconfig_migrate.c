// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *opt_old, *opt_new;
    int ret = -1;

    netdata_log_debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct config_section *sect_old = appconfig_section_find(root, section_old);
    if(!sect_old) return ret;

    struct config_section *sect_new = appconfig_section_find(root, section_new);
    if(!sect_new) sect_new = appconfig_section_create(root, section_new);

    SECTION_LOCK(sect_old);
    if(sect_old != sect_new)
        SECTION_LOCK(sect_new);

    opt_old = appconfig_option_find(sect_old, name_old);
    if(!opt_old) goto cleanup;

    opt_new = appconfig_option_find(sect_new, name_new);
    if(opt_new) goto cleanup;

    if(unlikely(appconfig_option_del(sect_old, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.",
                          string2str(opt_old->name), string2str(sect_old->name));

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sect_old->values, opt_old, prev, next);

    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "CONFIG: option '[%s].%s' has been migrated to '[%s].%s'.",
           section_old, name_old,
           section_new, name_new);

    if(!opt_old->name_migrated)
        opt_old->name_migrated = opt_old->name;

    opt_old->name = string_strdupz(name_new);
    opt_old->flags |= CONFIG_VALUE_MIGRATED;

    opt_new = opt_old;
    DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(sect_new->values, opt_new, prev, next);

    if(unlikely(appconfig_option_add(sect_new, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.",
                          string2str(opt_old->name), string2str(sect_new->name));

    ret = 0;

cleanup:
    if(sect_old != sect_new)
        SECTION_UNLOCK(sect_new);
    SECTION_UNLOCK(sect_old);
    return ret;
}

int appconfig_move_everywhere(struct config *root, const char *name_old, const char *name_new) {
    int ret = -1;
    APPCONFIG_LOCK(root);
    struct config_section *sect;
    for(sect = root->sections; sect; sect = sect->next) {
        if(appconfig_move(root, string2str(sect->name), name_old, string2str(sect->name), name_new) == 0)
            ret = 0;
    }
    APPCONFIG_UNLOCK(root);
    return ret;
}

