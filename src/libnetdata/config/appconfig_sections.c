// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

// ----------------------------------------------------------------------------
// config sections index

int appconfig_section_compare(void *a, void *b) {
    if(((struct config_section *)a)->name < ((struct config_section *)b)->name) return -1;
    else if(((struct config_section *)a)->name > ((struct config_section *)b)->name) return 1;
    else return string_cmp(((struct config_section *)a)->name, ((struct config_section *)b)->name);
}

struct config_section *appconfig_section_find(struct config *root, const char *name) {
    struct config_section sect_tmp = {
        .name = string_strdupz(name),
    };

    struct config_section *rc = (struct config_section *)avl_search_lock(&root->index, (avl_t *) &sect_tmp);
    string_freez(sect_tmp.name);
    return rc;
}

// ----------------------------------------------------------------------------
// config section methods

void appconfig_section_free(struct config_section *sect) {
    avl_destroy_lock(&sect->values_index);
    string_freez(sect->name);
    freez(sect);
}

void appconfig_section_remove_and_delete(struct config *root, struct config_section *sect, bool have_root_lock, bool have_sect_lock) {
    struct config_section *sect_found = appconfig_section_del(root, sect);
    if(sect_found != sect) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "INTERNAL ERROR: Cannot remove section '%s', it was not inserted before.",
               string2str(sect->name));
        return;
    }

    appconfig_option_remove_and_delete_all(sect, have_sect_lock);

    if(!have_root_lock)
        APPCONFIG_LOCK(root);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(root->sections, sect, prev, next);

    if(!have_root_lock)
        APPCONFIG_UNLOCK(root);

    // if the caller has the section lock, we will unlock it, to cleanup
    if(have_sect_lock)
        SECTION_UNLOCK(sect);

    appconfig_section_free(sect);
}

struct config_section *appconfig_section_create(struct config *root, const char *section) {
    struct config_section *sect = callocz(1, sizeof(struct config_section));
    sect->name = string_strdupz(section);
    spinlock_init(&sect->spinlock);

    avl_init_lock(&sect->values_index, appconfig_option_compare);

    struct config_section *sect_found = appconfig_section_add(root, sect);
    if(sect_found != sect) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CONFIG: section '%s', already exists, using existing.",
               string2str(sect->name));
        appconfig_section_free(sect);
        return sect_found;
    }

    APPCONFIG_LOCK(root);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(root->sections, sect, prev, next);
    APPCONFIG_UNLOCK(root);

    return sect;
}

// ----------------------------------------------------------------------------

const char *appconfig_get_value_of_option_in_section(struct config_section *co, const char *option, const char *default_value, reformat_t cb) {
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
