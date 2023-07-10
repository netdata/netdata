// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_CONFIGURATION_H
#define HTTPD_CONFIGURATION_H

#include "../libnetdata.h"

enum module_type {
    ARRAY,
    SINGLE
};

struct dyncfg_config {
    void *data;
    size_t data_size;
};

typedef struct dyncfg_config dyncfg_config_t;

struct configurable_plugin;
struct module;

enum job_state {
    JOB_STATE_UNKNOWN = 0, // State used until plugin reports first status
    JOB_STATE_STOPPED,
    JOB_STATE_RUNNING,
    JOB_STATE_ERROR
};

enum set_config_result {
    SET_CONFIG_ACCEPTED = 0,
    SET_CONFIG_REJECTED,
    SET_CONFIG_DEFFER
};

typedef enum set_config_result (*set_config_cb_t)(void *usr_ctx, dyncfg_config_t *cfg);

struct job
{
    char *name;
    dyncfg_config_t config;

    enum job_state state;
    usec_t last_state_update;

    struct module *module;

    set_config_cb_t set_config_cb;
    void *set_config_cb_usr_ctx;
};

struct module
{
    pthread_mutex_t lock;
    char *name;
    enum module_type type;
    const char *schema;

    dyncfg_config_t config;
    dyncfg_config_t default_config;

    struct configurable_plugin *plugin;

    DICTIONARY *jobs;

    set_config_cb_t set_config_cb;
    void *set_config_cb_usr_ctx;
};

struct configurable_plugin {
    pthread_mutex_t lock;
    char *name;
    DICTIONARY *modules;
    const char *schema;

    dyncfg_config_t config;
    dyncfg_config_t default_config;

    set_config_cb_t set_config_cb;
    void *set_config_cb_usr_ctx;
};

//int has_module

// API to be used by plugins
int register_plugin(struct configurable_plugin *plugin);
int register_module(struct configurable_plugin *plugin, struct module *module);


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

#endif // HTTPD_CONFIGURATION_H
