// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyn_conf.h"

// TODO this is hardcoded for now, as virtual module during development
// as demo, in future this callback should exist within the module/plugin itself
json_object *http_check_config = NULL;

json_object *get_current_config_http_check()
{
    if (http_check_config == NULL) {
        http_check_config = json_object_new_object();
        json_object *sub = json_object_new_string("I'am http_check and this is my current configuration");
        json_object_object_add(http_check_config, "info", sub);
        sub = json_object_new_int(5);
        json_object_object_add(http_check_config, "update_every", sub);
    }
    json_object *copy = NULL;
    json_object_deep_copy(http_check_config, &copy, NULL);
    return copy;
}

int set_current_config_http_check(json_object *cfg)
{
    json_object_put(http_check_config);
    http_check_config = cfg;
    return 0;
}

// TODO this has to be created dynamically in future
// e.g. plugins registering their modules

struct configurable_module modules[] = {
    {
        .name = "http_check",
        .submodules = NULL,
        .submodule_count = 0,
        .schema = NULL,
        .get_current_config_cb = get_current_config_http_check,
        .set_config_cb = set_current_config_http_check,
    },
    {
        .name = NULL,
        .submodules = NULL,
        .submodule_count = 0,
        .schema = NULL,
        .get_current_config_cb = NULL,
        .set_config_cb = NULL
    }
};

json_object *get_list_of_modules_json()
{
    json_object *obj = json_object_new_array();

    for (int i = 0; modules[i].name != NULL; i++) {
        json_object *module = json_object_new_string(modules[i].name);
        json_object_array_add(obj, module);
    }

    return obj;
}

struct configurable_module *get_module_by_name(const char *name)
{
    for (int i = 0; modules[i].name != NULL; i++) {
        if (strcmp(modules[i].name, name) == 0) {
            return &modules[i];
        }
    }

    return NULL;
}

json_object *get_config_of_module_json(struct configurable_module *module)
{
    if (module->get_current_config_cb == NULL) {
        return NULL;
    }

    return module->get_current_config_cb();
}

int set_module_config_json(struct configurable_module *module, json_object *cfg)
{
    if (module->set_config_cb == NULL) {
        return -1;
    }

    return module->set_config_cb(cfg);
}