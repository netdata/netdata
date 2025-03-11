// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_INICFG_INTERNALS_H
#define LIBNETDATA_INICFG_INTERNALS_H

#include "inicfg.h"

typedef enum __attribute__((packed)) {
    CONFIG_VALUE_TYPE_UNKNOWN = 0,
    CONFIG_VALUE_TYPE_TEXT,
    CONFIG_VALUE_TYPE_HOSTNAME,
    CONFIG_VALUE_TYPE_USERNAME,
    CONFIG_VALUE_TYPE_FILENAME,
    CONFIG_VALUE_TYPE_PATH,
    CONFIG_VALUE_TYPE_SIMPLE_PATTERN,
    CONFIG_VALUE_TYPE_URL,
    CONFIG_VALUE_TYPE_ENUM,
    CONFIG_VALUE_TYPE_BITMAP,
    CONFIG_VALUE_TYPE_INTEGER,
    CONFIG_VALUE_TYPE_DOUBLE,
    CONFIG_VALUE_TYPE_BOOLEAN,
    CONFIG_VALUE_TYPE_BOOLEAN_ONDEMAND,
    CONFIG_VALUE_TYPE_DURATION_IN_SECS,
    CONFIG_VALUE_TYPE_DURATION_IN_MS,
    CONFIG_VALUE_TYPE_DURATION_IN_DAYS_TO_SECONDS,
    CONFIG_VALUE_TYPE_SIZE_IN_BYTES,
    CONFIG_VALUE_TYPE_SIZE_IN_MB,
} CONFIG_VALUE_TYPES;

typedef enum __attribute__((packed)) {
    CONFIG_VALUE_LOADED         = (1 << 0), // has been loaded from the config
    CONFIG_VALUE_USED           = (1 << 1), // has been accessed from the program
    CONFIG_VALUE_CHANGED        = (1 << 2), // has been changed from the loaded value or the internal default value
    CONFIG_VALUE_CHECKED        = (1 << 3), // has been checked if the value is different from the default
    CONFIG_VALUE_MIGRATED       = (1 << 4), // has been migrated from an old config
    CONFIG_VALUE_REFORMATTED    = (1 << 5), // has been reformatted with the official formatting
    CONFIG_VALUE_DEFAULT_SET    = (1 << 6), // the default value has been set
} CONFIG_VALUE_FLAGS;

struct config_option {
    avl_t avl_node;         // the index entry of this entry - this has to be first!

    CONFIG_VALUE_TYPES type;
    CONFIG_VALUE_FLAGS flags;

    STRING *name;
    STRING *value;

    STRING *value_original;     // the original value of this option (the first value it got, independently on how it got it)
    STRING *value_default;      // the internal default value of this option (the first value it got, from inicfg_get_XXX())

    // when we move options around, this is where we keep the original
    // section and name (of the first migration)
    struct {
        STRING *section;
        STRING *name;
    } migrated;

    struct config_option *prev, *next; // config->mutex protects just this
};

struct config_section {
    avl_t avl_node;         // the index entry of this section - this has to be first!

    STRING *name;

    struct config_option *values;
    avl_tree_lock values_index;

    SPINLOCK spinlock;
    struct config_section *prev, *next;    // global config_mutex protects just this
};

// ----------------------------------------------------------------------------
// locking

#define APPCONFIG_LOCK(root) spinlock_lock(&((root)->spinlock))
#define APPCONFIG_UNLOCK(root) spinlock_unlock(&((root)->spinlock))
#define SECTION_LOCK(sect) spinlock_lock(&((sect)->spinlock))
#define SECTION_UNLOCK(sect) spinlock_unlock(&((sect)->spinlock));

// config sections
void inicfg_section_free(struct config_section *sect);
void inicfg_section_remove_and_delete(struct config *root, struct config_section *sect, bool have_root_lock, bool have_sect_lock);
#define inicfg_section_add(root, cfg) (struct config_section *)avl_insert_lock(&(root)->index, (avl_t *)(cfg))
#define inicfg_section_del(root, cfg) (struct config_section *)avl_remove_lock(&(root)->index, (avl_t *)(cfg))
struct config_section *inicfg_section_find(struct config *root, const char *name);
struct config_section *inicfg_section_create(struct config *root, const char *section);

// config options
void inicfg_option_cleanup(struct config_option *opt);
void inicfg_option_free(struct config_option *opt);
void inicfg_option_remove_and_delete(struct config_section *sect, struct config_option *opt, bool have_sect_lock);
void inicfg_option_remove_and_delete_all(struct config_section *sect, bool have_sect_lock);
int inicfg_option_compare(void *a, void *b);
#define inicfg_option_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl_t *)(cv))
#define inicfg_option_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl_t *)(cv))
struct config_option *inicfg_option_find(struct config_section *sect, const char *name);
struct config_option *inicfg_option_create(struct config_section *sect, const char *name, const char *value);

// lookup
int inicfg_get_boolean_by_section(struct config_section *sect, const char *name, int value);

typedef STRING *(*reformat_t)(STRING *value);
struct config_option *inicfg_get_raw_value_of_option_in_section(struct config_section *sect, const char *option, const char *default_value, CONFIG_VALUE_TYPES type, reformat_t cb);
struct config_option *inicfg_get_raw_value(struct config *root, const char *section, const char *option, const char *default_value, CONFIG_VALUE_TYPES type, reformat_t cb);

void inicfg_set_raw_value_of_option(struct config_option *opt, const char *value, CONFIG_VALUE_TYPES type);
struct config_option *inicfg_set_raw_value_of_option_in_section(struct config_section *sect, const char *option, const char *value, CONFIG_VALUE_TYPES type);
struct config_option *inicfg_set_raw_value(struct config *root, const char *section, const char *option, const char *value, CONFIG_VALUE_TYPES type);

// cleanup
void inicfg_section_destroy_non_loaded(struct config *root, const char *section);
void inicfg_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name);

// exporters
_CONNECTOR_INSTANCE *add_connector_instance(struct config_section *connector, struct config_section *instance);
int is_valid_connector(char *type, int check_reserved);

#endif /* LIBNETDATA_INICFG_INTERNALS_H */
