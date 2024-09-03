// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPCONFIG_INTERNALS_H
#define NETDATA_APPCONFIG_INTERNALS_H

#include "appconfig.h"

typedef enum __attribute__((packed)) {
    CONFIG_VALUE_LOADED = (1 << 0),         // has been loaded from the config
    CONFIG_VALUE_USED = (1 << 1),           // has been accessed from the program
    CONFIG_VALUE_CHANGED = (1 << 2),        // has been changed from the loaded value or the internal default value
    CONFIG_VALUE_CHECKED = (1 << 3),        // has been checked if the value is different from the default
    CONFIG_VALUE_MIGRATED = (1 << 4),       // has been migrated from an old config
    CONFIG_VALUE_REFORMATTED = (1 << 5),    // has been reformatted with the official formatting
} CONFIG_VALUE_FLAGS;

struct config_option {
    avl_t avl_node;         // the index entry of this entry - this has to be first!

    CONFIG_VALUE_FLAGS flags;

    STRING *name;
    STRING *value;

    STRING *name_migrated;
    STRING *value_reformatted;

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
void appconfig_section_free(struct config_section *sect);
void appconfig_section_remove_and_delete(struct config *root, struct config_section *sect, bool have_root_lock, bool have_sect_lock);
#define appconfig_section_add(root, cfg) (struct config_section *)avl_insert_lock(&(root)->index, (avl_t *)(cfg))
#define appconfig_section_del(root, cfg) (struct config_section *)avl_remove_lock(&(root)->index, (avl_t *)(cfg))
struct config_section *appconfig_section_find(struct config *root, const char *name);
struct config_section *appconfig_section_create(struct config *root, const char *section);

// config options
void appconfig_option_cleanup(struct config_option *opt);
void appconfig_option_free(struct config_option *opt);
void appconfig_option_remove_and_delete(struct config_section *sect, struct config_option *opt, bool have_sect_lock);
void appconfig_option_remove_and_delete_all(struct config_section *sect, bool have_sect_lock);
int appconfig_option_compare(void *a, void *b);
#define appconfig_option_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl_t *)(cv))
#define appconfig_option_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl_t *)(cv))
struct config_option *appconfig_option_find(struct config_section *sect, const char *name);
struct config_option *appconfig_option_create(struct config_section *sect, const char *name, const char *value);

// lookup
int appconfig_get_boolean_by_section(struct config_section *co, const char *name, int value);

typedef STRING *(*reformat_t)(STRING *value);
const char *appconfig_get_value_of_option_in_section(struct config_section *co, const char *option, const char *default_value, reformat_t cb);
const char *appconfig_get_value_and_reformat(struct config *root, const char *section, const char *option, const char *default_value, reformat_t cb);

// cleanup
void appconfig_section_destroy_non_loaded(struct config *root, const char *section);
void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name);

// exporters
_CONNECTOR_INSTANCE *add_connector_instance(struct config_section *connector, struct config_section *instance);
int is_valid_connector(char *type, int check_reserved);

#endif //NETDATA_APPCONFIG_INTERNALS_H
