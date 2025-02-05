// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

// ----------------------------------------------------------------------------
// config options index

int inicfg_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->name < ((struct config_option *)b)->name) return -1;
    else if(((struct config_option *)a)->name > ((struct config_option *)b)->name) return 1;
    else return string_cmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

struct config_option *inicfg_option_find(struct config_section *sect, const char *name) {
    struct config_option opt_tmp = {
        .name = string_strdupz(name),
    };

    struct config_option *rc = (struct config_option *)avl_search_lock(&(sect->values_index), (avl_t *) &opt_tmp);

    inicfg_option_cleanup(&opt_tmp);
    return rc;
}

// ----------------------------------------------------------------------------
// config options methods

void inicfg_option_cleanup(struct config_option *opt) {
    string_freez(opt->value);
    string_freez(opt->name);
    string_freez(opt->migrated.section);
    string_freez(opt->migrated.name);
    string_freez(opt->value_original);
    string_freez(opt->value_default);

    opt->value = NULL;
    opt->name = NULL;
    opt->migrated.section = NULL;
    opt->migrated.name = NULL;
    opt->value_original = NULL;
    opt->value_default = NULL;
}

void inicfg_option_free(struct config_option *opt) {
    inicfg_option_cleanup(opt);
    freez(opt);
}

NEVERNULL
struct config_option *inicfg_option_create(struct config_section *sect, const char *name, const char *value) {
    struct config_option *opt = callocz(1, sizeof(struct config_option));
    opt->name = string_strdupz(name);
    opt->value = string_strdupz(value);
    opt->value_original = string_dup(opt->value);

    struct config_option *opt_found = inicfg_option_add(sect, opt);
    if(opt_found != opt) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "CONFIG: config '%s' in section '%s': already exists - using the existing one.",
               string2str(opt->name), string2str(sect->name));
        inicfg_option_free(opt);
        return opt_found;
    }

    SECTION_LOCK(sect);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sect->values, opt, prev, next);
    SECTION_UNLOCK(sect);

    return opt;
}

void inicfg_option_remove_and_delete(struct config_section *sect, struct config_option *opt, bool have_sect_lock) {
    struct config_option *opt_found = inicfg_option_del(sect, opt);
    if(opt_found != opt) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "INTERNAL ERROR: Cannot remove '%s' from  section '%s', it was not inserted before.",
               string2str(opt->name), string2str(sect->name));
        return;
    }

    if(!have_sect_lock)
        SECTION_LOCK(sect);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sect->values, opt, prev, next);

    if(!have_sect_lock)
        SECTION_UNLOCK(sect);

    inicfg_option_free(opt);
}

void inicfg_option_remove_and_delete_all(struct config_section *sect, bool have_sect_lock) {
    if(!have_sect_lock)
        SECTION_LOCK(sect);

    while(sect->values)
        inicfg_option_remove_and_delete(sect, sect->values, true);

    if(!have_sect_lock)
        SECTION_UNLOCK(sect);
}

void inicfg_get_raw_value_of_option(struct config_option *opt, const char *default_value, CONFIG_VALUE_TYPES type, reformat_t cb) {
    opt->flags |= CONFIG_VALUE_USED;

    if(type != CONFIG_VALUE_TYPE_UNKNOWN)
        opt->type = type;

    if((opt->flags & CONFIG_VALUE_LOADED) || (opt->flags & CONFIG_VALUE_CHANGED)) {
        // this is a loaded value from the config file
        // if it is different from the default, mark it
        if(!(opt->flags & CONFIG_VALUE_CHECKED)) {
            if(!(opt->flags & CONFIG_VALUE_REFORMATTED) && cb) {
                STRING *value_old = opt->value;
                opt->value = cb(opt->value);
                if(opt->value != value_old)
                    opt->flags |= CONFIG_VALUE_REFORMATTED;
            }

            if(default_value && string_strcmp(opt->value, default_value) != 0)
                opt->flags |= CONFIG_VALUE_CHANGED;

            opt->flags |= CONFIG_VALUE_CHECKED;
        }
    }

    if(!(opt->flags & CONFIG_VALUE_DEFAULT_SET)) {
        opt->flags |= CONFIG_VALUE_DEFAULT_SET;
        opt->value_default = string_strdupz(default_value);
    }
}

struct config_option *inicfg_get_raw_value_of_option_in_section(struct config_section *sect, const char *option, const char *default_value, CONFIG_VALUE_TYPES type, reformat_t cb) {
    // Only calls internal to this file check for a NULL result, and they do not supply a NULL arg.
    // External caller should treat NULL as an error case.
    struct config_option *opt = inicfg_option_find(sect, option);
    if (!opt) {
        if (!default_value) return NULL;
        opt = inicfg_option_create(sect, option, default_value);
    }

    inicfg_get_raw_value_of_option(opt, default_value, type, cb);
    return opt;
}

struct config_option *inicfg_get_raw_value(struct config *root, const char *section, const char *option, const char *default_value, CONFIG_VALUE_TYPES type, reformat_t cb) {
    struct config_section *sect = inicfg_section_find(root, section);
    if(!sect) {
        if(!default_value) return NULL;
        sect = inicfg_section_create(root, section);
    }

    return inicfg_get_raw_value_of_option_in_section(sect, option, default_value, type, cb);
}

void inicfg_set_raw_value_of_option(struct config_option *opt, const char *value, CONFIG_VALUE_TYPES type) {
    opt->flags |= CONFIG_VALUE_USED;

    if(opt->type == CONFIG_VALUE_TYPE_UNKNOWN)
        opt->type = type;

    if(string_strcmp(opt->value, value) != 0) {
        opt->flags |= CONFIG_VALUE_CHANGED;

        string_freez(opt->value);
        opt->value = string_strdupz(value);
    }
}

struct config_option *inicfg_set_raw_value_of_option_in_section(struct config_section *sect, const char *option, const char *value, CONFIG_VALUE_TYPES type) {
    struct config_option *opt = inicfg_option_find(sect, option);
    if(!opt)
        opt = inicfg_option_create(sect, option, value);

    inicfg_set_raw_value_of_option(opt, value, type);
    return opt;
}

struct config_option *inicfg_set_raw_value(struct config *root, const char *section, const char *option, const char *value, CONFIG_VALUE_TYPES type) {
    struct config_section *sect = inicfg_section_find(root, section);
    if(!sect)
        sect = inicfg_section_create(root, section);

    return inicfg_set_raw_value_of_option_in_section(sect, option, value, type);
}
