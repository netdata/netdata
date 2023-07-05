// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTPD_CONFIGURATION_H
#define HTTPD_CONFIGURATION_H

#include "../libnetdata.h"

enum module_type {
    ARRAY,
    SINGLE
};

struct module
{
    char *name;
    enum module_type type;
    const char *schema;
};

struct configurable_plugin {
    pthread_mutex_t lock;
    char *name;
    struct module *modules;
    size_t module_count;
    const char *schema;

    json_object *config;
    json_object *default_config;

    int (*set_config_cb)(json_object *cfg);
};

//int has_module

// API to be used by plugins
int register_plugin(struct configurable_plugin *plugin);


// API to be used by the web server(s)
json_object *get_list_of_plugins_json();
struct configurable_plugin *get_plugin_by_name(const char *name);
json_object *get_config_of_plugin_json(struct configurable_plugin *plugin);
const char *set_plugin_config_json(struct configurable_plugin *plugin, json_object *cfg);

// helper struct to make interface between internal webserver and h2o same
struct uni_http_response {
    int status;
    char *content;
    HTTP_CONTENT_TYPE content_type;
    void (*content_free)(void *);
};

struct uni_http_response dyn_conf_process_http_request(int method, const char *plugin, const char *module, const char *job_id);

// API to be used by main netdata process, initialization and destruction etc.
int dyn_conf_init(void);

#endif // HTTPD_CONFIGURATION_H
