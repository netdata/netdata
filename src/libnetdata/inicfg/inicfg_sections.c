// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

// ----------------------------------------------------------------------------
// config sections index

int inicfg_section_compare(void *a, void *b) {
    if(((struct config_section *)a)->name < ((struct config_section *)b)->name) return -1;
    else if(((struct config_section *)a)->name > ((struct config_section *)b)->name) return 1;
    else return string_cmp(((struct config_section *)a)->name, ((struct config_section *)b)->name);
}

struct config_section *inicfg_section_find(struct config *root, const char *name) {
    struct config_section sect_tmp = {
        .name = string_strdupz(name),
    };

    struct config_section *rc = (struct config_section *)avl_search_lock(&root->index, (avl_t *) &sect_tmp);
    string_freez(sect_tmp.name);
    return rc;
}

// ----------------------------------------------------------------------------
// config section methods

void inicfg_section_free(struct config_section *sect) {
    avl_destroy_lock(&sect->values_index);
    string_freez(sect->name);
    freez(sect);
}

void inicfg_section_remove_and_delete(struct config *root, struct config_section *sect, bool have_root_lock, bool have_sect_lock) {
    struct config_section *sect_found = inicfg_section_del(root, sect);
    if(sect_found != sect) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "INTERNAL ERROR: Cannot remove section '%s', it was not inserted before.",
               string2str(sect->name));
        return;
    }

    inicfg_option_remove_and_delete_all(sect, have_sect_lock);

    if(!have_root_lock)
        APPCONFIG_LOCK(root);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(root->sections, sect, prev, next);

    if(!have_root_lock)
        APPCONFIG_UNLOCK(root);

    // if the caller has the section lock, we will unlock it, to cleanup
    if(have_sect_lock)
        SECTION_UNLOCK(sect);

    inicfg_section_free(sect);
}

struct config_section *inicfg_section_create(struct config *root, const char *section) {
    struct config_section *sect = callocz(1, sizeof(struct config_section));
    sect->name = string_strdupz(section);
    spinlock_init(&sect->spinlock);

    avl_init_lock(&sect->values_index, inicfg_option_compare);

    struct config_section *sect_found = inicfg_section_add(root, sect);
    if(sect_found != sect) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CONFIG: section '%s', already exists, using existing.",
               string2str(sect->name));
        inicfg_section_free(sect);
        return sect_found;
    }

    APPCONFIG_LOCK(root);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(root->sections, sect, prev, next);
    APPCONFIG_UNLOCK(root);

    return sect;
}


