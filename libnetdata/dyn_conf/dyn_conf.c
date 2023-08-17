// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyn_conf.h"

#define DYN_CONF_PATH_MAX (4096)
#define DYN_CONF_DIR VARLIB_DIR "/etc"

#define DYN_CONF_JOB_SCHEMA "job_schema"
#define DYN_CONF_SCHEMA "schema"
#define DYN_CONF_MODULE_LIST "modules"
#define DYN_CONF_JOB_LIST "jobs"
#define DYN_CONF_CFG_EXT ".cfg"

DICTIONARY *plugins_dict = NULL;

struct deferred_cfg_send {
    char *plugin_name;
    char *module_name;
    char *job_name;
    struct deferred_cfg_send *next;
};

bool dyncfg_shutdown = false;
struct deferred_cfg_send *deferred_configs = NULL;
pthread_mutex_t deferred_configs_lock = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t deferred_configs_cond = PTHREAD_COND_INITIALIZER;

static void deferred_config_free(struct deferred_cfg_send *dcs)
{
    freez(dcs->plugin_name);
    freez(dcs->module_name);
    freez(dcs->job_name);
    freez(dcs);
}

static void deferred_config_push_back(const char *plugin_name, const char *module_name, const char *job_name)
{
    struct deferred_cfg_send *deferred = callocz(1, sizeof(struct deferred_cfg_send));
    deferred->plugin_name = strdupz(plugin_name);
    if (module_name != NULL) {
        deferred->module_name = strdupz(module_name);
        if (job_name != NULL)
            deferred->job_name = strdupz(job_name);
    }
    pthread_mutex_lock(&deferred_configs_lock);
    if (dyncfg_shutdown) {
        pthread_mutex_unlock(&deferred_configs_lock);
        deferred_config_free(deferred);
        return;
    }
    struct deferred_cfg_send *last = deferred_configs;
    if (last == NULL)
        deferred_configs = deferred;
    else {
        while (last->next != NULL)
            last = last->next;
        last->next = deferred;
    }
    pthread_cond_signal(&deferred_configs_cond);
    pthread_mutex_unlock(&deferred_configs_lock);
}

static void deferred_configs_unlock()
{
    dyncfg_shutdown = true;
    // if we get cancelled in pthread_cond_wait
    // we will arrive at cancelled cleanup handler
    // with mutex locked we need to unlock it
    pthread_mutex_unlock(&deferred_configs_lock);
}

static struct deferred_cfg_send *deferred_config_pop(void *ptr)
{
    pthread_mutex_lock(&deferred_configs_lock);
    while (deferred_configs == NULL) {
        netdata_thread_cleanup_push(deferred_configs_unlock, ptr);
        pthread_cond_wait(&deferred_configs_cond, &deferred_configs_lock);
        netdata_thread_cleanup_pop(0);
    }
    struct deferred_cfg_send *deferred = deferred_configs;
    deferred_configs = deferred_configs->next;
    pthread_mutex_unlock(&deferred_configs_lock);
    return deferred;
}

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
        case MOD_TYPE_SINGLE:
            module_type = "single";
            break;
        case MOD_TYPE_ARRAY:
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

const char *job_status2str(enum job_status status)
{
    switch (status) {
        case JOB_STATUS_UNKNOWN:
            return "unknown";
        case JOB_STATUS_STOPPED:
            return "stopped";
        case JOB_STATUS_RUNNING:
            return "running";
        case JOB_STATUS_ERROR:
            return "error";
        default:
            return "unknown";
    }
}

static int _get_list_of_jobs_json_cb(const DICTIONARY_ITEM *item, void *entry, void *data)
{
    UNUSED(item);
    json_object *obj = (json_object *)data;
    struct job *job = (struct job *)entry;

    json_object *json_job = json_object_new_object();
    json_object *json_item = json_object_new_string(job->name);
    json_object_object_add(json_job, "name", json_item);
    json_item = json_object_new_string(job_status2str(job->status));
    json_object_object_add(json_job, "state", json_item);
    int64_t last_state_update_s  = job->last_state_update / USEC_PER_SEC;
    int64_t last_state_update_us = job->last_state_update % USEC_PER_SEC;

    json_item = json_object_new_int64(last_state_update_s);
    json_object_object_add(json_job, "last_state_update_s", json_item);

    json_item = json_object_new_int64(last_state_update_us);
    json_object_object_add(json_job, "last_state_update_us", json_item);

    json_object_array_add(obj, json_job);

    return 0;
}

json_object *get_list_of_jobs_json(struct module *module)
{
    json_object *obj = json_object_new_array();

    pthread_mutex_lock(&module->lock);

    dictionary_walkthrough_read(module->jobs, _get_list_of_jobs_json_cb, obj);

    pthread_mutex_unlock(&module->lock);

    return obj;
}

struct job *get_job_by_name(struct module *module, const char *job_name)
{
    return dictionary_get(module->jobs, job_name);
}

int remove_job(struct module *module, struct job *job)
{
    // as we are going to do unlink here we better make sure we have all to build proper path
    if (unlikely(job->name == NULL || module == NULL || module->name == NULL || module->plugin == NULL || module->plugin->name == NULL))
        return 0;

    enum set_config_result rc = module->delete_job_cb(module->job_config_cb_usr_ctx, module->name, job->name);

    if (rc != SET_CONFIG_ACCEPTED) {
        error_report("DYNCFG module \"%s\" rejected delete job for \"%s\"", module->name, job->name);
        return 0;
    }

    BUFFER *buffer = buffer_create(DYN_CONF_PATH_MAX, NULL);
    buffer_sprintf(buffer, DYN_CONF_DIR "/%s/%s/%s" DYN_CONF_CFG_EXT, module->plugin->name, module->name, job->name);
    unlink(buffer_tostring(buffer));
    buffer_free(buffer);
    return dictionary_del(module->jobs, job->name);
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
            netdata_log_error("DYNCFG store_config: failed to create module directory %s", buffer_tostring(filename));
            buffer_free(filename);
            return 1;
        }
    }

    if (submodule_name != NULL) {
        buffer_sprintf(filename, "/%s", submodule_name);
        if (mkdir(buffer_tostring(filename), 0755) == -1) {
            if (errno != EEXIST) {
                netdata_log_error("DYNCFG store_config: failed to create submodule directory %s", buffer_tostring(filename));
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
    config.data = read_by_filename(buffer_tostring(filename), &bytes);

    if (config.data == NULL)
        error_report("DYNCFG load_config: failed to load config from %s", buffer_tostring(filename));

    config.data_size = bytes;

    buffer_free(filename);

    return config;
}

char *set_plugin_config(struct configurable_plugin *plugin, dyncfg_config_t cfg)
{
    enum set_config_result rc = plugin->set_config_cb(plugin->cb_usr_ctx, &cfg);
    if (rc != SET_CONFIG_ACCEPTED) {
        error_report("DYNCFG plugin \"%s\" rejected config", plugin->name);
        return "plugin rejected config";
    }

    if (store_config(plugin->name, NULL, NULL, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", plugin->name);
        return "could not store config on disk";
    }
    return NULL;
}

static char *set_module_config(struct module *mod, dyncfg_config_t cfg)
{
    struct configurable_plugin *plugin = mod->plugin;

    enum set_config_result rc = mod->set_config_cb(mod->config_cb_usr_ctx, mod->name, &cfg);
    if (rc != SET_CONFIG_ACCEPTED) {
        error_report("DYNCFG module \"%s\" rejected config", plugin->name);
        return "module rejected config";
    }

    if (store_config(plugin->name, mod->name, NULL, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", mod->name);
        return "could not store config on disk";
    }

    return NULL;
}

struct job *job_new()
{
    struct job *job = callocz(1, sizeof(struct job));
    job->state = JOB_STATUS_UNKNOWN;
    job->last_state_update = now_realtime_usec();
    return job;
}

static int set_job_config(struct job *job, dyncfg_config_t cfg)
{
    struct module *mod = job->module;
    enum set_config_result rt = mod->set_job_config_cb(mod->job_config_cb_usr_ctx, mod->name, job->name, &cfg);

    if (rt != SET_CONFIG_ACCEPTED) {
        error_report("DYNCFG module \"%s\" rejected config for job \"%s\"", mod->name, job->name);
        return 1;
    }

    if (store_config(mod->plugin->name, mod->name, job->name, cfg)) {
        error_report("DYNCFG could not store config for module \"%s\"", mod->name);
        return 1;
    }

    return 0;
}

struct job *add_job(struct module *mod, const char *job_id, dyncfg_config_t cfg)
{
    struct job *job = job_new();
    job->name = strdupz(job_id);
    job->module = mod;

    if (set_job_config(job, cfg)) {
        freez(job->name);
        freez(job);
        return NULL;
    }

    dictionary_set(mod->jobs, job->name, job, sizeof(job));

    return job;

}

void module_del_cb(const DICTIONARY_ITEM *item, void *value, void *data)
{
    UNUSED(item);
    UNUSED(data);
    struct module *mod = (struct module *)value;
    dictionary_destroy(mod->jobs);
    freez(mod->name);
    freez(mod);
}


const DICTIONARY_ITEM *register_plugin(struct configurable_plugin *plugin)
{
    if (get_plugin_by_name(plugin->name) != NULL) {
        error_report("DYNCFG plugin \"%s\" already registered", plugin->name);
        return NULL;
    }

    if (plugin->set_config_cb == NULL) {
        error_report("DYNCFG plugin \"%s\" has no set_config_cb", plugin->name);
        return NULL;
    }

    pthread_mutex_init(&plugin->lock, NULL);

    plugin->modules = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);
    dictionary_register_delete_callback(plugin->modules, module_del_cb, NULL);

    deferred_config_push_back(plugin->name, NULL, NULL);

    dictionary_set(plugins_dict, plugin->name, plugin, sizeof(plugin));

    // the plugin keeps the pointer to the dictionary item, so we need to acquire it
    return dictionary_get_and_acquire_item(plugins_dict, plugin->name);
}

void unregister_plugin(const DICTIONARY_ITEM *plugin)
{
    struct configurable_plugin *plug = dictionary_acquired_item_value(plugin);
    dictionary_acquired_item_release(plugins_dict, plugin);
    dictionary_del(plugins_dict, plug->name);
}

void job_del_cb(const DICTIONARY_ITEM *item, void *value, void *data)
{
    UNUSED(item);
    UNUSED(data);
    struct job *job = (struct job *)value;
    freez(job->reason);
    freez(job->name);
    freez(job);
}

int register_module(struct configurable_plugin *plugin, struct module *module)
{
    if (get_module_by_name(plugin, module->name) != NULL) {
        error_report("DYNCFG module \"%s\" already registered", module->name);
        return 1;
    }

    pthread_mutex_init(&module->lock, NULL);

    deferred_config_push_back(plugin->name, module->name, NULL);

    module->plugin = plugin;

    if (module->type == MOD_TYPE_ARRAY) {
        module->jobs = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);
        dictionary_register_delete_callback(module->jobs, job_del_cb, NULL);

        // load all jobs from disk
        BUFFER *path = buffer_create(DYN_CONF_PATH_MAX, NULL);
        buffer_sprintf(path, "%s/%s/%s", DYN_CONF_DIR, plugin->name, module->name);
        DIR *dir = opendir(buffer_tostring(path));
        if (dir != NULL) {
            struct dirent *ent;
            while ((ent = readdir(dir)) != NULL) {
                if (ent->d_name[0] == '.')
                    continue;
                if (ent->d_type != DT_REG)
                    continue;
                size_t len = strnlen(ent->d_name, NAME_MAX);
                if (len <= strlen(DYN_CONF_CFG_EXT))
                    continue;
                if (strcmp(ent->d_name + len - strlen(DYN_CONF_CFG_EXT), DYN_CONF_CFG_EXT) != 0)
                    continue;
                ent->d_name[len - strlen(DYN_CONF_CFG_EXT)] = '\0';

                struct job *job = job_new();
                job->name = strdupz(ent->d_name);
                job->module = module;
                dictionary_set(module->jobs, job->name, job, sizeof(job));

                deferred_config_push_back(plugin->name, module->name, job->name);
            }
            closedir(dir);
        }
        buffer_free(path);
    }

    dictionary_set(plugin->modules, module->name, module, sizeof(module));

    return 0;
}

void freez_dyncfg(void *ptr) {
    freez(ptr);
}

static void handle_dyncfg_root(struct uni_http_response *resp, int method)
{
    if (method != HTTP_METHOD_GET) {
        resp->content = "method not allowed";
        resp->content_length = strlen(resp->content);
        resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
        return;
    }
    json_object *obj = get_list_of_plugins_json();
    json_object *wrapper = json_object_new_object();
    json_object_object_add(wrapper, "configurable_plugins", obj);
    resp->content = strdupz(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
    json_object_put(wrapper);
    resp->status = HTTP_RESP_OK;
    resp->content_type = CT_APPLICATION_JSON;
    resp->content_free = freez_dyncfg;
    resp->content_length = strlen(resp->content);
}

static void handle_plugin_root(struct uni_http_response *resp, int method, struct configurable_plugin *plugin, void *post_payload, size_t post_payload_size)
{
    switch(method) {
        case HTTP_METHOD_GET:
        {
            dyncfg_config_t cfg = plugin->get_config_cb(plugin->cb_usr_ctx);
            resp->content = mallocz(cfg.data_size);
            memcpy(resp->content, cfg.data, cfg.data_size);
            resp->status = HTTP_RESP_OK;
            resp->content_free = freez_dyncfg;
            resp->content_length = cfg.data_size;
            return;
        }
        case HTTP_METHOD_PUT:
        {
            char *response;
            if (post_payload == NULL) {
                resp->content = "no payload";
                resp->content_length = strlen(resp->content);
                resp->status = HTTP_RESP_BAD_REQUEST;
                return;
            }
            dyncfg_config_t cont = {
                .data = post_payload,
                .data_size = post_payload_size
            };
            response = set_plugin_config(plugin, cont);
            if (response == NULL) {
                resp->status = HTTP_RESP_OK;
                resp->content = "OK";
                resp->content_length = strlen(resp->content);
            } else {
                resp->status = HTTP_RESP_BAD_REQUEST;
                resp->content = response;
                resp->content_length = strlen(resp->content);
            }
            return;
        }
        default:
            resp->content = "method not allowed";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return;
    }
}

void handle_module_root(struct uni_http_response *resp, int method, struct configurable_plugin *plugin, const char *module, void *post_payload, size_t post_payload_size)
{
    if (strncmp(module, DYN_CONF_SCHEMA, strlen(DYN_CONF_SCHEMA)) == 0) {
        dyncfg_config_t cfg = plugin->get_config_schema_cb(plugin->cb_usr_ctx);
        resp->content = mallocz(cfg.data_size);
        memcpy(resp->content, cfg.data, cfg.data_size);
        resp->status = HTTP_RESP_OK;
        resp->content_free = freez_dyncfg;
        resp->content_length = cfg.data_size;
        return;
    }
    if (strncmp(module, DYN_CONF_MODULE_LIST, strlen(DYN_CONF_MODULE_LIST)) == 0) {
        if (method != HTTP_METHOD_GET) {
            resp->content = "method not allowed (only GET)";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return;
        }
        json_object *obj = get_list_of_modules_json(plugin);
        json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "modules", obj);
        resp->content = strdupz(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
        json_object_put(wrapper);
        resp->status = HTTP_RESP_OK;
        resp->content_type = CT_APPLICATION_JSON;
        resp->content_free = freez_dyncfg;
        resp->content_length = strlen(resp->content);
        return;
    }
    struct module *mod = get_module_by_name(plugin, module);
    if (mod == NULL) {
        resp->content = "module not found";
        resp->content_length = strlen(resp->content);
        resp->status = HTTP_RESP_NOT_FOUND;
        return;
    }
    if (method == HTTP_METHOD_GET) {
        dyncfg_config_t cfg = mod->get_config_cb(mod->config_cb_usr_ctx, mod->name);
        resp->content = mallocz(cfg.data_size);
        memcpy(resp->content, cfg.data, cfg.data_size);
        resp->status = HTTP_RESP_OK;
        resp->content_free = freez_dyncfg;
        resp->content_length = cfg.data_size;
        return;
    } else if (method == HTTP_METHOD_PUT) {
        char *response;
        if (post_payload == NULL) {
            resp->content = "no payload";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_BAD_REQUEST;
            return;
        }
        dyncfg_config_t cont = {
            .data = post_payload,
            .data_size = post_payload_size
        };
        response = set_module_config(mod, cont);
        if (response == NULL) {
            resp->status = HTTP_RESP_OK;
            resp->content = "OK";
            resp->content_length = strlen(resp->content);
        } else {
            resp->status = HTTP_RESP_BAD_REQUEST;
            resp->content = response;
            resp->content_length = strlen(resp->content);
        }
        return;
    }
    resp->content = "method not allowed";
    resp->content_length = strlen(resp->content);
    resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
}

static inline void _handle_job_root(struct uni_http_response *resp, int method, struct module *mod, const char *job_id, void *post_payload, size_t post_payload_size, struct job *job)
{
    if (method == HTTP_METHOD_POST) {
        if (job != NULL) {
            resp->content = "can't POST, job already exists (use PUT to update?)";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_BAD_REQUEST;
            return;
        }
        if (post_payload == NULL) {
            resp->content = "no payload";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_BAD_REQUEST;
            return;
        }
        dyncfg_config_t cont = {
            .data = post_payload,
            .data_size = post_payload_size
        };
        job = add_job(mod, job_id, cont);
        if (job == NULL) {
            resp->content = "failed to add job";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_INTERNAL_SERVER_ERROR;
            return;
        }
        resp->status = HTTP_RESP_OK;
        resp->content = "OK";
        resp->content_length = strlen(resp->content);
        return;
    }
    if (job == NULL) {
        resp->content = "job not found";
        resp->content_length = strlen(resp->content);
        resp->status = HTTP_RESP_NOT_FOUND;
        return;
    }
    switch (method) {
        case HTTP_METHOD_GET:
        {
            dyncfg_config_t cfg = mod->get_job_config_cb(mod->job_config_cb_usr_ctx, mod->name, job->name);
            resp->content = mallocz(cfg.data_size);
            memcpy(resp->content, cfg.data, cfg.data_size);
            resp->status = HTTP_RESP_OK;
            resp->content_free = freez_dyncfg;
            resp->content_length = cfg.data_size;
            return;
        }
        case HTTP_METHOD_PUT:
        {
            if (post_payload == NULL) {
                resp->content = "missing payload";
                resp->content_length = strlen(resp->content);
                resp->status = HTTP_RESP_BAD_REQUEST;
                return;
            }
            dyncfg_config_t cont = {
                .data = post_payload,
                .data_size = post_payload_size
            };
            if(set_job_config(job, cont)) {
                resp->status = HTTP_RESP_BAD_REQUEST;
                resp->content = "failed to set job config";
                resp->content_length = strlen(resp->content);
                return;
            }
            resp->status = HTTP_RESP_OK;
            resp->content = "OK";
            resp->content_length = strlen(resp->content);
            return;
        }
        case HTTP_METHOD_DELETE:
        {
            if (!remove_job(mod, job)) {
                resp->content = "failed to remove job";
                resp->content_length = strlen(resp->content);
                resp->status = HTTP_RESP_INTERNAL_SERVER_ERROR;
                return;
            }
            resp->status = HTTP_RESP_OK;
            resp->content = "OK";
            resp->content_length = strlen(resp->content);
            return;
        }
        default:
            resp->content = "method not allowed (only GET, PUT, DELETE)";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return;
    }
}

void handle_job_root(struct uni_http_response *resp, int method, struct module *mod, const char *job_id, void *post_payload, size_t post_payload_size)
{
    if (strncmp(job_id, DYN_CONF_SCHEMA, strlen(DYN_CONF_SCHEMA)) == 0) {
        dyncfg_config_t cfg = mod->get_config_schema_cb(mod->config_cb_usr_ctx, mod->name);
        resp->content = mallocz(cfg.data_size);
        memcpy(resp->content, cfg.data, cfg.data_size);
        resp->status = HTTP_RESP_OK;
        resp->content_free = freez_dyncfg;
        resp->content_length = cfg.data_size;
        return;
    }
    if (strncmp(job_id, DYN_CONF_JOB_SCHEMA, strlen(DYN_CONF_JOB_SCHEMA)) == 0) {
        dyncfg_config_t cfg = mod->get_job_config_schema_cb(mod->job_config_cb_usr_ctx, mod->name);
        resp->content = mallocz(cfg.data_size);
        memcpy(resp->content, cfg.data, cfg.data_size);
        resp->status = HTTP_RESP_OK;
        resp->content_free = freez_dyncfg;
        resp->content_length = cfg.data_size;
        return;
    }
    if (strncmp(job_id, DYN_CONF_JOB_LIST, strlen(DYN_CONF_JOB_LIST)) == 0) {
        if (mod->type != MOD_TYPE_ARRAY) {
            resp->content = "module type is not job_array (can't get the list of jobs)";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_NOT_FOUND;
            return;
        }
        if (method != HTTP_METHOD_GET) {
            resp->content = "method not allowed (only GET)";
            resp->content_length = strlen(resp->content);
            resp->status = HTTP_RESP_METHOD_NOT_ALLOWED;
            return;
        }
        json_object *obj = get_list_of_jobs_json(mod);
        json_object *wrapper = json_object_new_object();
        json_object_object_add(wrapper, "jobs", obj);
        resp->content = strdupz(json_object_to_json_string_ext(wrapper, JSON_C_TO_STRING_PRETTY));
        json_object_put(wrapper);
        resp->status = HTTP_RESP_OK;
        resp->content_type = CT_APPLICATION_JSON;
        resp->content_free = freez_dyncfg;
        resp->content_length = strlen(resp->content);
        return;
    }
    const DICTIONARY_ITEM *job_item = dictionary_get_and_acquire_item(mod->jobs, job_id);
    struct job *job = dictionary_acquired_item_value(job_item);

    _handle_job_root(resp, method, mod, job_id, post_payload, post_payload_size, job);

    dictionary_acquired_item_release(mod->jobs, job_item);
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
        handle_dyncfg_root(&resp, method);
        return resp;
    }
    const DICTIONARY_ITEM *plugin_item = dictionary_get_and_acquire_item(plugins_dict, plugin);
    if (plugin_item == NULL) {
        resp.content = "plugin not found";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_NOT_FOUND;
        return resp;
    }
    struct configurable_plugin *plug = dictionary_acquired_item_value(plugin_item);
    if (module == NULL) {
        handle_plugin_root(&resp, method, plug, post_payload, post_payload_size);
        goto EXIT_PLUGIN;
    }
    if (job_id == NULL) {
        handle_module_root(&resp, method, plug, module, post_payload, post_payload_size);
        goto EXIT_PLUGIN;
    }
    // for modules we do not do get_and_acquire as modules are never removed (only together with the plugin)
    struct module *mod = get_module_by_name(plug, module);
    if (mod == NULL) {
        resp.content = "module not found";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_NOT_FOUND;
        goto EXIT_PLUGIN;
    }
    if (mod->type != MOD_TYPE_ARRAY) {
        resp.content = "module is not array";
        resp.content_length = strlen(resp.content);
        resp.status = HTTP_RESP_NOT_FOUND;
        goto EXIT_PLUGIN;
    }
    handle_job_root(&resp, method, mod, job_id, post_payload, post_payload_size);

EXIT_PLUGIN:
    dictionary_acquired_item_release(plugins_dict, plugin_item);
    return resp;
}

void plugin_del_cb(const DICTIONARY_ITEM *item, void *value, void *data)
{
    UNUSED(item);
    UNUSED(data);
    struct configurable_plugin *plugin = (struct configurable_plugin *)value;
    dictionary_destroy(plugin->modules);
    freez(plugin->name);
    freez(plugin);
}

void report_job_status(struct configurable_plugin *plugin, const char *module_name, const char *job_name, enum job_status status, int status_code, char *reason)
{
    struct job *job = NULL;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(plugins_dict, plugin->name);
    if (item == NULL) {
        netdata_log_error("plugin %s not found", plugin->name);
        freez(reason);
        return;
    }
    struct configurable_plugin *plug = dictionary_acquired_item_value(item);
    struct module *mod = get_module_by_name(plug, module_name);
    if (mod == NULL) {
        netdata_log_error("module %s not found", module_name);
        goto EXIT_PLUGIN;
    }
    if (mod->type != MOD_TYPE_ARRAY) {
        netdata_log_error("module %s is not array", module_name);
        goto EXIT_PLUGIN;
    }
    const DICTIONARY_ITEM *job_item = dictionary_get_and_acquire_item(mod->jobs, job_name);
    if (job_item == NULL) {
        netdata_log_error("job %s not found", job_name);
        goto EXIT_PLUGIN;
    }
    job = dictionary_acquired_item_value(job_item);
    job->status = status;
    job->state = status_code;
    if (job->reason != NULL) {
        freez(job->reason);
    }
    job->reason = reason;
    job->last_state_update = now_realtime_usec();

    dictionary_acquired_item_release(mod->jobs, job_item);

EXIT_PLUGIN:
    if (!job)
        freez(reason);
    dictionary_acquired_item_release(plugins_dict, item);
}

int dyn_conf_init(void)
{
    if (mkdir(DYN_CONF_DIR, 0755) == -1) {
        if (errno != EEXIST) {
            netdata_log_error("failed to create directory for dynamic configuration");
            return 1;
        }
    }

    plugins_dict = dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE);
    dictionary_register_delete_callback(plugins_dict, plugin_del_cb, NULL);

    return 0;
}

static void dyncfg_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *) ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("cleaning up...");

    pthread_mutex_lock(&deferred_configs_lock);
    dyncfg_shutdown = true;
    while (deferred_configs != NULL) {
        struct deferred_cfg_send *dcs = deferred_configs;
        deferred_configs = dcs->next;
        deferred_config_free(dcs);
    }
    pthread_mutex_unlock(&deferred_configs_lock);

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *dyncfg_main(void *ptr)
{
    netdata_thread_cleanup_push(dyncfg_cleanup, ptr);

    while (!netdata_exit) {
        struct deferred_cfg_send *dcs = deferred_config_pop(ptr);
        const DICTIONARY_ITEM *plugin_item = dictionary_get_and_acquire_item(plugins_dict, dcs->plugin_name);
        if (plugin_item == NULL) {
            error_report("DYNCFG, plugin %s not found", dcs->plugin_name);
            deferred_config_free(dcs);
            continue;
        }
        struct configurable_plugin *plugin = dictionary_acquired_item_value(plugin_item);
        if (dcs->module_name == NULL) {
            dyncfg_config_t cfg = load_config(dcs->plugin_name, NULL, NULL);
            if (cfg.data != NULL) {
                plugin->set_config_cb(plugin->cb_usr_ctx, &cfg);
                freez(cfg.data);
            }
        } else if (dcs->job_name == NULL) {
            dyncfg_config_t cfg = load_config(dcs->plugin_name, dcs->module_name, NULL);
            if (cfg.data != NULL) {
                struct module *mod = get_module_by_name(plugin, dcs->module_name);
                mod->set_config_cb(mod->config_cb_usr_ctx, mod->name, &cfg);
                freez(cfg.data);
            }
        } else {
            dyncfg_config_t cfg = load_config(dcs->plugin_name, dcs->module_name, dcs->job_name);
            if (cfg.data != NULL) {
                struct module *mod = get_module_by_name(plugin, dcs->module_name);
                mod->set_job_config_cb(mod->job_config_cb_usr_ctx, mod->name, dcs->job_name, &cfg);
                freez(cfg.data);
            }
        }
        deferred_config_free(dcs);
        dictionary_acquired_item_release(plugins_dict, plugin_item);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
