// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DYN_CONF_H
#define DYN_CONF_H

#include "../libnetdata.h"

enum module_type {
    MOD_TYPE_UNKNOWN = 0,
    MOD_TYPE_ARRAY,
    MOD_TYPE_SINGLE
};

static inline enum module_type str2_module_type(const char *type_name)
{
    if (strcmp(type_name, "job_array") == 0)
        return MOD_TYPE_ARRAY;
    else if (strcmp(type_name, "single") == 0)
        return MOD_TYPE_SINGLE;
    return MOD_TYPE_UNKNOWN;
}

struct dyncfg_config {
    void *data;
    size_t data_size;
};

typedef struct dyncfg_config dyncfg_config_t;

struct configurable_plugin;
struct module;

enum job_status {
    JOB_STATUS_UNKNOWN = 0, // State used until plugin reports first status
    JOB_STATUS_STOPPED,
    JOB_STATUS_RUNNING,
    JOB_STATUS_ERROR
};

inline enum job_status str2job_state(const char *state_name) {
    if (strcmp(state_name, "stopped") == 0)
        return JOB_STATUS_STOPPED;
    else if (strcmp(state_name, "running") == 0)
        return JOB_STATUS_RUNNING;
    else if (strcmp(state_name, "error") == 0)
        return JOB_STATUS_ERROR;
    return JOB_STATUS_UNKNOWN;
}

enum set_config_result {
    SET_CONFIG_ACCEPTED = 0,
    SET_CONFIG_REJECTED,
    SET_CONFIG_DEFFER
};

struct job
{
    char *name;

    //state reported by config
    enum job_status status; // reported by plugin, enum as this has to be interpreted by UI
    int state; // code reported by plugin which can mean anything plugin wants
    char *reason; // reported by plugin, can be NULL (optional)

    usec_t last_state_update;

    struct module *module;
};

struct module
{
    pthread_mutex_t lock;
    char *name;
    enum module_type type;

    struct configurable_plugin *plugin;

    // module config
    enum set_config_result (*set_config_cb)(void *usr_ctx, const char *module_name, dyncfg_config_t *cfg);
    dyncfg_config_t (*get_config_cb)(void *usr_ctx, const char *name);
    dyncfg_config_t (*get_config_schema_cb)(void *usr_ctx, const char *name);
    void *config_cb_usr_ctx;

    DICTIONARY *jobs;

    // jobs config
    dyncfg_config_t (*get_job_config_cb)(void *usr_ctx, const char *module_name, const char *job_name);
    dyncfg_config_t (*get_job_config_schema_cb)(void *usr_ctx, const char *module_name);
    enum set_config_result (*set_job_config_cb)(void *usr_ctx, const char *module_name, const char *job_name, dyncfg_config_t *cfg);
    enum set_config_result (*delete_job_cb)(void *usr_ctx, const char *module_name, const char *job_name);
    void *job_config_cb_usr_ctx;
};

struct configurable_plugin {
    pthread_mutex_t lock;
    char *name;
    DICTIONARY *modules;
    const char *schema;

    dyncfg_config_t (*get_config_cb)(void *usr_ctx);
    dyncfg_config_t (*get_config_schema_cb)(void *usr_ctx);
    enum set_config_result (*set_config_cb)(void *usr_ctx, dyncfg_config_t *cfg);
    void *cb_usr_ctx; // context for all callbacks (split if needed in future)
};

// API to be used by plugins
const DICTIONARY_ITEM *register_plugin(struct configurable_plugin *plugin);
void unregister_plugin(const DICTIONARY_ITEM *plugin);
int register_module(struct configurable_plugin *plugin, struct module *module);

void report_job_status(struct configurable_plugin *plugin, const char *module_name, const char *job_name, enum job_status status, int status_code, char *reason);

// API to be used by the web server(s)
json_object *get_list_of_plugins_json();
struct configurable_plugin *get_plugin_by_name(const char *name);

json_object *get_list_of_modules_json(struct configurable_plugin *plugin);
struct module *get_module_by_name(struct configurable_plugin *plugin, const char *module_name);

// helper struct to make interface between internal webserver and h2o same
struct uni_http_response {
    int status;
    char *content;
    size_t content_length;
    HTTP_CONTENT_TYPE content_type;
    void (*content_free)(void *);
};

struct uni_http_response dyn_conf_process_http_request(int method, const char *plugin, const char *module, const char *job_id, void *payload, size_t payload_size);

// API to be used by main netdata process, initialization and destruction etc.
int dyn_conf_init(void);

void *dyncfg_main(void *in);

#endif //DYN_CONF_H
