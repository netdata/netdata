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

