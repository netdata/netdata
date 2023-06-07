// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyn_conf.h"

#define DYN_CONF_PATH_MAX (4096)
#define DYN_CONF_DIR VARLIB_DIR "/etc"

DICTIONARY *modules_dict = NULL;

static int _get_list_of_modules_json_cb(const DICTIONARY_ITEM *item, void *entry, void *data)
{
    json_object *obj = (json_object *)data;
    struct configurable_module *module = (struct configurable_module *)entry;

    json_object *module_name = json_object_new_string(module->name);
    json_object_array_add(obj, module_name);

    return 0;
}

json_object *get_list_of_modules_json()
{
    json_object *obj = json_object_new_array();

    dictionary_walkthrough_read(modules_dict, _get_list_of_modules_json_cb, obj);

    return obj;
}

struct configurable_module *get_module_by_name(const char *name)
{
    return dictionary_get(modules_dict, name);
}

json_object *get_config_of_module_json(struct configurable_module *module)
{
    if (module->get_current_config_cb == NULL) {
        return NULL;
    }

    return module->get_current_config_cb();
}

int store_config(const char *module_name, const char *submodule_name, const char *cfg_idx, json_object *cfg)
{
    BUFFER *filename = buffer_create(DYN_CONF_PATH_MAX, NULL);
    buffer_sprintf(filename, DYN_CONF_DIR "/%s", module_name);
    if (mkdir(buffer_tostring(filename), 0755) == -1) {
        if (errno != EEXIST) {
            error("DYNCFG store_config: failed to create module directory %s", buffer_tostring(filename));
            buffer_free(filename);
            return 1;
        }
    }

    if (submodule_name != NULL) {
        buffer_sprintf(filename, "/%s", submodule_name);
        if (mkdir(buffer_tostring(filename), 0755) == -1) {
            if (errno != EEXIST) {
                error("DYNCFG store_config: failed to create submodule directory %s", buffer_tostring(filename));
                buffer_free(filename);
                return 1;
            }
        }
    }

    if (cfg_idx != NULL)
        buffer_sprintf(filename, "/%s", cfg_idx);

    buffer_strcat(filename, ".json");


    error_report("DYNCFG store_config: %s", buffer_tostring(filename));

    // TODO check what permissions json_object_to_file_ext uses
    // if not satisfactory then use json_object_to_fd
    if (json_object_to_file_ext(buffer_tostring(filename), cfg, JSON_C_TO_STRING_PRETTY)) {
        error_report("DYNCFG store_config: failed to write config to %s, json_error: %s", buffer_tostring(filename), json_util_get_last_err());
        buffer_free(filename);
        return 1;
    }

    buffer_free(filename);
    return 0;
}

json_object *load_config(const char *module_name, const char *submodule_name, const char *cfg_idx)
{
    BUFFER *filename = buffer_create(DYN_CONF_PATH_MAX, NULL);
    buffer_sprintf(filename, DYN_CONF_DIR "/%s", module_name);
    if (submodule_name != NULL)
        buffer_sprintf(filename, "/%s", submodule_name);

    if (cfg_idx != NULL)
        buffer_sprintf(filename, "/%s", cfg_idx);

    buffer_strcat(filename, ".json");

    json_object *ret = json_object_from_file(buffer_tostring(filename));
    if (ret == NULL)
        error_report("DYNCFG load_config: failed to load config from %s, json_error: %s", buffer_tostring(filename), json_util_get_last_err());

    return ret;
}

const char *set_module_config_json(struct configurable_module *module, json_object *cfg)
{
    if (store_config(module->name, NULL, NULL, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", module->name);
        return "could not store config on disk";
    }

    if (module->set_config_cb == NULL) {
        error_report("DYNCFG module \"%s\" has no set_config_cb", module->name);
        return "module has no set_config_cb callback";
    }

    module->set_config_cb(cfg);

    return NULL;
}

int register_module(struct configurable_module *module)
{
    if (get_module_by_name(module->name) != NULL) {
        error_report("DYNCFG module \"%s\" already registered", module->name);
        return 1;
    }

    if (dictionary_set(modules_dict, module->name, module, sizeof(module))) {
        error_report("DYNCFG failed to register module \"%s\"", module->name);
        return 1;
    }

    return 0;
}

int dyn_conf_init(void)
{
    if (mkdir(DYN_CONF_DIR, 0755) == -1) {
        if (errno != EEXIST) {
            error("failed to create directory for dynamic configuration");
            return 1;
        }
    }

    modules_dict = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);

    return 0;
}