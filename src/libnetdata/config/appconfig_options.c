// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

// ----------------------------------------------------------------------------
// config options index

int appconfig_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->name < ((struct config_option *)b)->name) return -1;
    else if(((struct config_option *)a)->name > ((struct config_option *)b)->name) return 1;
    else return string_cmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

struct config_option *appconfig_option_find(struct section *co, const char *name) {
    struct config_option tmp = {
        .name = string_strdupz(name),
    };

    struct config_option *rc = (struct config_option *)avl_search_lock(&(co->values_index), (avl_t *) &tmp);

    appconfig_option_cleanup(&tmp);
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

struct config_option *appconfig_option_create(struct section *co, const char *name, const char *value) {
    netdata_log_debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

    struct config_option *opt = callocz(1, sizeof(struct config_option));
    opt->name = string_strdupz(name);
    opt->value = string_strdupz(value);

    struct config_option *opt_found = appconfig_option_add(co, opt);
    if(opt_found != opt) {
        netdata_log_error("indexing of config '%s' in section '%s': already exists - using the existing one.",
                          string2str(opt->name), string2str(co->name));
        appconfig_option_free(opt);
        return opt_found;
    }

    config_section_wrlock(co);
    struct config_option *opt2 = co->values;
    if(opt2) {
        while (opt2->next)
            opt2 = opt2->next;
        opt2->next = opt;
    }
    else co->values = opt;
    config_section_unlock(co);

    return opt;
}

const char *appconfig_get_value_and_reformat(struct config *root, const char *section, const char *option, const char *default_value, reformat_t cb) {
    struct section *sect = appconfig_section_find(root, section);

    if (!sect && !default_value)
        return NULL;

    if(!sect)
        sect = appconfig_section_create(root, section);

    return appconfig_get_value_of_option_in_section(sect, option, default_value, cb);
}
