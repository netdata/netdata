// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

int inicfg_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *opt_old, *opt_new;
    int ret = -1;

    netdata_log_debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct config_section *sect_old = inicfg_section_find(root, section_old);
    if(!sect_old) return ret;

    struct config_section *sect_new = inicfg_section_find(root, section_new);
    if(!sect_new) sect_new = inicfg_section_create(root, section_new);

    SECTION_LOCK(sect_old);
    if(sect_old != sect_new)
        SECTION_LOCK(sect_new);

    opt_old = inicfg_option_find(sect_old, name_old);
    if(!opt_old) goto cleanup;

    opt_new = inicfg_option_find(sect_new, name_new);
    if(opt_new) goto cleanup;

    if(unlikely(inicfg_option_del(sect_old, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.",
                          string2str(opt_old->name), string2str(sect_old->name));

    // remember the old position of the item
    struct config_option *opt_old_next = (sect_old == sect_new) ? opt_old->next : NULL;

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sect_old->values, opt_old, prev, next);

//    nd_log(NDLS_DAEMON, NDLP_WARNING,
//           "CONFIG: option '[%s].%s' has been migrated to '[%s].%s'.",
//           section_old, name_old,
//           section_new, name_new);

    if(!opt_old->migrated.name) {
        string_freez(opt_old->migrated.section);
        opt_old->migrated.section = string_dup(sect_old->name);
        opt_old->migrated.name = opt_old->name;
    }
    else
        string_freez(opt_old->name);

    opt_old->name = string_strdupz(name_new);
    opt_old->flags |= CONFIG_VALUE_MIGRATED;

    opt_new = opt_old;

    // put in the list, but try to keep the order
    if(opt_old_next && sect_old == sect_new)
        DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(sect_new->values, opt_old_next, opt_new, prev, next);
    else {
        // we don't have the old next item (probably a different section?)
        // find the last MIGRATED one
        struct config_option *t = sect_new->values ? sect_new->values->prev : NULL;
        for (; t && t != sect_new->values ; t = t->prev) {
            if (t->flags & CONFIG_VALUE_MIGRATED)
                break;
        }
        if (t == sect_new->values)
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(sect_new->values, opt_new, prev, next);
        else
            DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(sect_new->values, t, opt_new, prev, next);
    }

    if(unlikely(inicfg_option_add(sect_new, opt_old) != opt_old))
        netdata_log_error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.",
                          string2str(opt_old->name), string2str(sect_new->name));

    ret = 0;

cleanup:
    if(sect_old != sect_new)
        SECTION_UNLOCK(sect_new);
    SECTION_UNLOCK(sect_old);
    return ret;
}

int inicfg_move_everywhere(struct config *root, const char *name_old, const char *name_new) {
    int ret = -1;
    APPCONFIG_LOCK(root);
    struct config_section *sect;
    for(sect = root->sections; sect; sect = sect->next) {
        if(inicfg_move(root, string2str(sect->name), name_old, string2str(sect->name), name_new) == 0)
            ret = 0;
    }
    APPCONFIG_UNLOCK(root);
    return ret;
}

