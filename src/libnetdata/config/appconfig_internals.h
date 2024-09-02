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

    struct config_option *next; // config->mutex protects just this
};

struct section {
    avl_t avl_node;         // the index entry of this section - this has to be first!

    STRING *name;

    struct section *next;    // global config_mutex protects just this

    struct config_option *values;
    avl_tree_lock values_index;

    netdata_mutex_t mutex;  // this locks only the writers, to ensure atomic updates
                           // readers are protected using the rwlock in avl_tree_lock
};

// ----------------------------------------------------------------------------
// locking

static inline void appconfig_wrlock(struct config *root) {
    netdata_mutex_lock(&root->mutex);
}

static inline void appconfig_unlock(struct config *root) {
    netdata_mutex_unlock(&root->mutex);
}

static inline void config_section_wrlock(struct section *co) {
    netdata_mutex_lock(&co->mutex);
}

static inline void config_section_unlock(struct section *co) {
    netdata_mutex_unlock(&co->mutex);
}

// config sections
#define appconfig_section_add(root, cfg) (struct section *)avl_insert_lock(&(root)->index, (avl_t *)(cfg))
#define appconfig_section_del(root, cfg) (struct section *)avl_remove_lock(&(root)->index, (avl_t *)(cfg))
struct section *appconfig_section_find(struct config *root, const char *name);
struct section *appconfig_section_create(struct config *root, const char *section);

// config options
int appconfig_option_compare(void *a, void *b);
#define appconfig_option_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl_t *)(cv))
#define appconfig_option_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl_t *)(cv))
struct config_option *appconfig_option_find(struct section *co, const char *name);
struct config_option *appconfig_option_create(struct section *co, const char *name, const char *value);

// lookup
int appconfig_get_boolean_by_section(struct section *co, const char *name, int value);

typedef STRING *(*reformat_t)(STRING *value);
const char *appconfig_get_value_of_option_in_section(struct section *co, const char *option, const char *default_value, reformat_t cb);
const char *appconfig_get_value_and_reformat(struct config *root, const char *section, const char *option, const char *default_value, reformat_t cb);

// cleanup
void appconfig_section_destroy_non_loaded(struct config *root, const char *section);
void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name);

// exporters
_CONNECTOR_INSTANCE *add_connector_instance(struct section *connector, struct section *instance);
int is_valid_connector(char *type, int check_reserved);

#endif //NETDATA_APPCONFIG_INTERNALS_H
