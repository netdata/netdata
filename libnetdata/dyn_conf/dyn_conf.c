// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyn_conf.h"

#define DYN_CONF_PATH_MAX (4096)
#define DYN_CONF_DIR VARLIB_DIR "/etc"

DICTIONARY *plugins_dict = NULL;

static int _get_list_of_plugins_json_cb(const DICTIONARY_ITEM *item, void *entry, void *data)
{
    UNUSED(item);
    json_object *obj = (json_object *)data;
    struct configurable_plugin *plugin = (struct configurable_plugin *)entry;

    json_object *plugin_name = json_object_new_string(plugin->name);
    json_object_array_add(obj, plugin_name);

    return 0;
}

json_object *get_list_of_plugins_json()
{
    json_object *obj = json_object_new_array();

    dictionary_walkthrough_read(plugins_dict, _get_list_of_plugins_json_cb, obj);

    return obj;
}

inline struct configurable_plugin *get_plugin_by_name(const char *name)
{
    return dictionary_get(plugins_dict, name);
}

json_object *get_config_of_plugin_json(struct configurable_plugin *plugin)
{
    pthread_mutex_lock(&plugin->lock);
    json_object *cfg = NULL;
    json_object_deep_copy(plugin->config, &cfg, NULL);
    pthread_mutex_unlock(&plugin->lock);
    return cfg;
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

json_object *load_config(const char *plugin_name, const char *module_name, const char *job_id)
{
    BUFFER *filename = buffer_create(DYN_CONF_PATH_MAX, NULL);
    buffer_sprintf(filename, DYN_CONF_DIR "/%s", plugin_name);
    if (module_name != NULL)
        buffer_sprintf(filename, "/%s", module_name);

    if (job_id != NULL)
        buffer_sprintf(filename, "/%s", job_id);

    buffer_strcat(filename, ".json");

    json_object *ret = json_object_from_file(buffer_tostring(filename));
    if (ret == NULL)
        error_report("DYNCFG load_config: failed to load config from %s, json_error: %s", buffer_tostring(filename), json_util_get_last_err());

    return ret;
}

const char *set_plugin_config_json(struct configurable_plugin *module, json_object *cfg)
{
    if (store_config(module->name, NULL, NULL, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", module->name);
        return "could not store config on disk";
    }

    if (module->set_config_cb == NULL) {
        error_report("DYNCFG module \"%s\" has no set_config_cb", module->name);
        return "module has no set_config_cb callback";
    }

    pthread_mutex_lock(&module->lock);
    if (module->config != module->default_config)
        json_object_put(module->config);
    module->config = cfg;
    pthread_mutex_unlock(&module->lock);

    json_object *cfg_copy = NULL;
    json_object_deep_copy(cfg, &cfg_copy, NULL);
    module->set_config_cb(cfg_copy);

    return NULL;
}

int register_plugin(struct configurable_plugin *plugin)
{
    if (get_plugin_by_name(plugin->name) != NULL) {
        error_report("DYNCFG plugin \"%s\" already registered", plugin->name);
        return 1;
    }

    if (plugin->set_config_cb == NULL) {
        error_report("DYNCFG plugin \"%s\" has no set_config_cb", plugin->name);
        return 1;
    }

    pthread_mutex_init(&plugin->lock, NULL);

    plugin->config = load_config(plugin->name, NULL, NULL);
    if (plugin->config == NULL) {
        plugin->config = plugin->default_config;
        if (plugin->config == NULL) {
            error_report("DYNCFG module \"%s\" has no default config", plugin->name);
            return 1;
        }
    }

    dictionary_set(plugins_dict, plugin->name, plugin, sizeof(plugin));

    return 0;
}

struct uni_http_response dyn_conf_process_http_request(int method, const char *plugin, const char *module, const char *job_id)
{
    struct uni_http_response resp = {
        .status = HTTP_RESP_INTERNAL_SERVER_ERROR,
        .content_type = CT_TEXT_PLAIN,
        .content = HTTP_RESP_INTERNAL_SERVER_ERROR_STR,
        .content_free = NULL
    };
    if (plugin == NULL) {
        if (method != HTTP_METHOD_GET) {
            resp.content = "method not allowed";
            resp.status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return resp;
        }
        json_object *obj = get_list_of_plugins_json();
        json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "configurable_plugins", obj);
        resp.content = strdup(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
        json_object_put(wrapper);
        resp.status = HTTP_RESP_OK;
        resp.content_type = CT_APPLICATION_JSON;
        resp.content_free = free;
        return resp;
    }
    struct configurable_plugin *plug = get_plugin_by_name(plugin);
    if (plug == NULL) {
        resp.content = "plugin not found";
        resp.status = HTTP_RESP_NOT_FOUND;
        return resp;
    }
    return resp;
}

int dyn_conf_init(void)
{
    if (mkdir(DYN_CONF_DIR, 0755) == -1) {
        if (errno != EEXIST) {
            error("failed to create directory for dynamic configuration");
            return 1;
        }
    }

    plugins_dict = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);

    return 0;
}