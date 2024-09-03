// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

// ----------------------------------------------------------------------------
// config options index

int appconfig_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->name < ((struct config_option *)b)->name) return -1;
    else if(((struct config_option *)a)->name > ((struct config_option *)b)->name) return 1;
    else return string_cmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

struct config_option *appconfig_option_find(struct section *sect, const char *name) {
    struct config_option opt_tmp = {
        .name = string_strdupz(name),
    };

    struct config_option *rc = (struct config_option *)avl_search_lock(&(sect->values_index), (avl_t *) &opt_tmp);

    appconfig_option_cleanup(&opt_tmp);
    return rc;
}

// ----------------------------------------------------------------------------
// config options methods

void appconfig_option_cleanup(struct config_option *opt) {
    string_freez(opt->value);
    string_freez(opt->name);
    string_freez(opt->name_migrated);
    string_freez(opt->value_reformatted);

    opt->value = NULL;
    opt->name = NULL;
    opt->name_migrated = NULL;
    opt->value_reformatted = NULL;
}

void appconfig_option_free(struct config_option *opt) {
    appconfig_option_cleanup(opt);
    freez(opt);
}

struct config_option *appconfig_option_create(struct section *sect, const char *name, const char *value) {
    struct config_option *opt = callocz(1, sizeof(struct config_option));
    opt->name = string_strdupz(name);
    opt->value = string_strdupz(value);

    struct config_option *opt_found = appconfig_option_add(sect, opt);
    if(opt_found != opt) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "CONFIG: config '%s' in section '%s': already exists - using the existing one.",
               string2str(opt->name), string2str(sect->name));
        appconfig_option_free(opt);
        return opt_found;
    }

    config_section_wrlock(sect);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sect->values, opt, prev, next);
    config_section_unlock(sect);

    return opt;
}

void appconfig_option_remove_and_delete(struct section *sect, struct config_option *opt, bool have_sect_lock) {
    struct config_option *opt_found = appconfig_option_del(sect, opt);
    if(opt_found != opt) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "INTERNAL ERROR: Cannot remove '%s' from  section '%s', it was not inserted before.",
               string2str(opt->name), string2str(sect->name));
        return;
    }

    if(!have_sect_lock)
        config_section_wrlock(sect);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sect->values, opt, prev, next);

    if(!have_sect_lock)
        config_section_unlock(sect);

    appconfig_option_free(opt);
}

const char *appconfig_get_value_and_reformat(struct config *root, const char *section, const char *option, const char *default_value, reformat_t cb) {
    struct section *sect = appconfig_section_find(root, section);

    if (!sect && !default_value)
        return NULL;

    if(!sect)
        sect = appconfig_section_create(root, section);

    return appconfig_get_value_of_option_in_section(sect, option, default_value, cb);
}
