// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

void inicfg_section_destroy_non_loaded(struct config *root, const char *section)
{
    struct config_section *sect;
    struct config_option *opt;

    netdata_log_debug(D_CONFIG, "Destroying section '%s'.", section);

    sect = inicfg_section_find(root, section);
    if(!sect) {
        netdata_log_error("Could not destroy section '%s'. Not found.", section);
        return;
    }

    SECTION_LOCK(sect);

    // find if there is any loaded option
    for(opt = sect->values; opt; opt = opt->next) {
        if (opt->flags & CONFIG_VALUE_LOADED) {
            // do not destroy values that were loaded from the configuration files.
            SECTION_UNLOCK(sect);
            return;
        }
    }

    // no option is loaded, free them all
    inicfg_section_remove_and_delete(root, sect, false, true);
}

void inicfg_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name) {
    struct config_section *sect;
    sect = inicfg_section_find(root, section);
    if (!sect) {
        netdata_log_error("Could not destroy section option '%s -> %s'. The section not found.", section, name);
        return;
    }

    SECTION_LOCK(sect);

    struct config_option *opt = inicfg_option_find(sect, name);
    if (opt && opt->flags & CONFIG_VALUE_LOADED) {
        SECTION_UNLOCK(sect);
        return;
    }

    if (unlikely(!(opt && inicfg_option_del(sect, opt)))) {
        SECTION_UNLOCK(sect);
        netdata_log_error("Could not destroy section option '%s -> %s'. The option not found.", section, name);
        return;
    }

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sect->values, opt, prev, next);

    inicfg_option_free(opt);
    SECTION_UNLOCK(sect);
}

/**
 * Free all config memory
 * 
 * This function frees all memory associated with a config structure,
 * including all sections and options.
 * 
 * @param root The config structure to free
 */
void inicfg_free(struct config *root) {
    if (!root)
        return;

    nd_log(NDLS_DAEMON, NDLP_DEBUG, "Freeing config memory");
    
    // First let's free the linked list (this will properly free all sections and options)
    APPCONFIG_LOCK(root);
    struct config_section *sect = root->sections;
    
    while (sect) {
        struct config_section *next_sect = sect->next;
        
        // Remove and free all options in this section
        SECTION_LOCK(sect);
        struct config_option *opt = sect->values;
        while (opt) {
            struct config_option *next_opt = opt->next;
            
            // Remove from index
            if(inicfg_option_del(sect, opt)) { ; }
            
            // Free the option
            inicfg_option_free(opt);
            
            opt = next_opt;
        }
        SECTION_UNLOCK(sect);
        
        // Remove from index
        if(inicfg_section_del(root, sect)) { ; }
        
        // Free the section
        inicfg_section_free(sect);
        
        sect = next_sect;
    }
    
    // Reset the sections pointer
    root->sections = NULL;
    APPCONFIG_UNLOCK(root);
    
    // Destroy the tree
    avl_destroy_lock(&root->index);
}

