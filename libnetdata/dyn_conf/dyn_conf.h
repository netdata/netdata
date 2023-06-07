// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_CONFIGURATION_H
#define HTTPD_CONFIGURATION_H

#include "../libnetdata.h"

enum submodule_type {
    ARRAY,
    SINGLE
};

struct submodule
{
    char *name;
    enum submodule_type type;
    const char *schema;
};

struct configurable_module {
    pthread_mutex_t lock;
    char *name;
    struct submodule *submodules;
    size_t submodule_count;
    const char *schema;

    json_object *config;
    json_object *default_config;

    int (*set_config_cb)(json_object *cfg);
};

//int has_module

// API to be used by modules
int register_module(struct configurable_module *module);


// API to be used by the web server
json_object *get_list_of_modules_json();
struct configurable_module *get_module_by_name(const char *name);
json_object *get_config_of_module_json(struct configurable_module *module);
const char *set_module_config_json(struct configurable_module *module, json_object *cfg);

// API to be used by main netdata process, initialization and destruction etc.
int dyn_conf_init(void);

#endif // HTTPD_CONFIGURATION_H
