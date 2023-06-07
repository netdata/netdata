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
    char *name;
    struct submodule *submodules;
    size_t submodule_count;
    const char *schema;

    // caller is responsible to free the json_object provided
    json_object * (*get_current_config_cb)(void);
    int (*set_config_cb)(json_object *cfg);
};

//int has_module

json_object *get_list_of_modules_json();
struct configurable_module *get_module_by_name(const char *name);
json_object *get_config_of_module_json(struct configurable_module *module);
int set_module_config_json(struct configurable_module *module, json_object *cfg);

#endif // HTTPD_CONFIGURATION_H
