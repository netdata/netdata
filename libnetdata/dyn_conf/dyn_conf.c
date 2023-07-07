// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyn_conf.h"

#define DYN_CONF_PATH_MAX (4096)
#define DYN_CONF_DIR VARLIB_DIR "/etc"

#define DYN_CONF_SCHEMA "schema"
#define DYN_CONF_MODULE_LIST "modules"
#define DYN_CONF_CFG_EXT ".cfg"

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

static int _get_list_of_modules_json_cb(const DICTIONARY_ITEM *item, void *entry, void *data)
{
    UNUSED(item);
    json_object *obj = (json_object *)data;
    struct module *module = (struct module *)entry;

    json_object *json_module = json_object_new_object();

    json_object *json_item = json_object_new_string(module->name);
    json_object_object_add(json_module, "name", json_item);
    const char *module_type;
    switch (module->type) {
        case SINGLE:
            module_type = "single";
            break;
        case ARRAY:
            module_type = "job_array";
            break;
        default:
            module_type = "unknown";
            break;
    }
    json_item = json_object_new_string(module_type);
    json_object_object_add(json_module, "type", json_item);

    json_object_array_add(obj, json_module);

    return 0;
}

json_object *get_list_of_modules_json(struct configurable_plugin *plugin)
{
    json_object *obj = json_object_new_array();

    pthread_mutex_lock(&plugin->lock);

    dictionary_walkthrough_read(plugin->modules, _get_list_of_modules_json_cb, obj);

    pthread_mutex_unlock(&plugin->lock);

    return obj;
}

struct module *get_module_by_name(struct configurable_plugin *plugin, const char *module_name)
{
    return dictionary_get(plugin->modules, module_name);
}

inline struct configurable_plugin *get_plugin_by_name(const char *name)
{
    return dictionary_get(plugins_dict, name);
}

static int store_config(const char *module_name, const char *submodule_name, const char *cfg_idx, dyncfg_config_t cfg)
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

    buffer_strcat(filename, DYN_CONF_CFG_EXT);


    error_report("DYNCFG store_config: %s", buffer_tostring(filename));

    //write to file
    FILE *f = fopen(buffer_tostring(filename), "w");
    if (f == NULL) {
        error_report("DYNCFG store_config: failed to open %s for writing", buffer_tostring(filename));
        buffer_free(filename);
        return 1;
    }

    fwrite(cfg.data, cfg.data_size, 1, f);
    fclose(f);

    buffer_free(filename);
    return 0;
}

dyncfg_config_t load_config(const char *plugin_name, const char *module_name, const char *job_id)
{
    BUFFER *filename = buffer_create(DYN_CONF_PATH_MAX, NULL);
    buffer_sprintf(filename, DYN_CONF_DIR "/%s", plugin_name);
    if (module_name != NULL)
        buffer_sprintf(filename, "/%s", module_name);

    if (job_id != NULL)
        buffer_sprintf(filename, "/%s", job_id);

    buffer_strcat(filename, DYN_CONF_CFG_EXT);

    dyncfg_config_t config;
    long bytes;
    config.data = read_by_filename(filename, &bytes);

    if (config.data == NULL)
        error_report("DYNCFG load_config: failed to load config from %s", buffer_tostring(filename));

    config.data_size = bytes;

    return config;
}

static const char *set_plugin_config(struct configurable_plugin *plugin, dyncfg_config_t cfg)
{
    if (store_config(plugin->name, NULL, NULL, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", plugin->name);
        return "could not store config on disk";
    }

    if (plugin->set_config_cb == NULL) {
        error_report("DYNCFG module \"%s\" has no set_config_cb", plugin->name);
        return "module has no set_config_cb callback";
    }

    pthread_mutex_lock(&plugin->lock);
    plugin->config = cfg;
    pthread_mutex_unlock(&plugin->lock);

//    module->set_config_cb(cfg_copy);

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
    if (plugin->config.data == NULL) {
        plugin->config = plugin->default_config;
        if (plugin->config.data == NULL) {
            error_report("DYNCFG module \"%s\" has no default config", plugin->name);
            return 1;
        }
    }

    plugin->modules = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);

    dictionary_set(plugins_dict, plugin->name, plugin, sizeof(plugin));

    return 0;
}

int register_module(struct configurable_plugin *plugin, struct module *module)
{
    if (get_module_by_name(plugin, module->name) != NULL) {
        error_report("DYNCFG module \"%s\" already registered", module->name);
        return 1;
    }

    pthread_mutex_init(&module->lock, NULL);

    module->config = load_config(plugin->name, module->name, NULL);
    if (module->config.data == NULL) {
        module->config = module->default_config;
        if (module->config.data == NULL) {
            error_report("DYNCFG module \"%s\" has no default config", module->name);
            return 1;
        }
    }

    dictionary_set(plugin->modules, module->name, module, sizeof(module));

    return 0;
}

struct uni_http_response dyn_conf_process_http_request(int method, const char *plugin, const char *module, const char *job_id, void *post_payload, size_t post_payload_size)
{
    struct uni_http_response resp = {
        .status = HTTP_RESP_INTERNAL_SERVER_ERROR,
        .content_type = CT_TEXT_PLAIN,
        .content = HTTP_RESP_INTERNAL_SERVER_ERROR_STR,
        .content_free = NULL,
        .content_length = 0
    };
    if (plugin == NULL) {
        if (method != HTTP_METHOD_GET) {
            resp.content = "method not allowed";
            resp.content_length = strlen(resp.content);
            resp.status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return resp;
        }
        json_object *obj = get_list_of_plugins_json();
        json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "configurable_plugins", obj);
        resp.content = strdupz(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
        json_object_put(wrapper);
        resp.status = HTTP_RESP_OK;
        resp.content_type = CT_APPLICATION_JSON;
        resp.content_free = freez;
        resp.content_length = strlen(resp.content);
        return resp;
    }
    struct configurable_plugin *plug = get_plugin_by_name(plugin);
    if (plug == NULL) {
        resp.content = "plugin not found";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_NOT_FOUND;
        return resp;
    }
    if (module == NULL) {
        if (method == HTTP_METHOD_GET) {
            resp.content = mallocz(plug->config.data_size);
            memcpy(resp.content, plug->config.data, plug->config.data_size);
            resp.status = HTTP_RESP_OK;
//          resp.content_type = CT_APPLICATION_JSON;
            resp.content_free = free;
            resp.content_length = plug->config.data_size;
            return resp;
        } else if (method == HTTP_METHOD_PUT) {
            dyncfg_config_t cont = {
                .data = post_payload,
                .data_size = post_payload_size
            };
            set_plugin_config(plug, cont);
            resp.status = HTTP_RESP_OK;
            return resp;
        }
        resp.content = "method not allowed";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_METHOD_NOT_ALLOWED;
        return resp;
    }
    if (strncmp(module, DYN_CONF_SCHEMA, strlen(DYN_CONF_SCHEMA)) == 0) {
        resp.content = "not implemented yet";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_NOT_FOUND;
        return resp;
    }
    if (strncmp(module, DYN_CONF_MODULE_LIST, strlen(DYN_CONF_MODULE_LIST)) == 0) {
        if (method != HTTP_METHOD_GET) {
            resp.content = "method not allowed";
            resp.status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return resp;
        }
        json_object *obj = get_list_of_modules_json(plug);
        json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "modules", obj);
        resp.content = strdupz(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
        json_object_put(wrapper);
        resp.status = HTTP_RESP_OK;
        resp.content_type = CT_APPLICATION_JSON;
        resp.content_free = freez;
        resp.content_length = strlen(resp.content);
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