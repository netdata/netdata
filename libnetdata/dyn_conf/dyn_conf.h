// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DYN_CONF_H
#define DYN_CONF_H

#include "../libnetdata.h"

#define FUNCTION_NAME_GET_PLUGIN_CONFIG "get_plugin_config"
#define FUNCTION_NAME_GET_PLUGIN_CONFIG_SCHEMA "get_plugin_config_schema"
#define FUNCTION_NAME_GET_MODULE_CONFIG "get_module_config"
#define FUNCTION_NAME_GET_MODULE_CONFIG_SCHEMA "get_module_config_schema"
#define FUNCTION_NAME_GET_JOB_CONFIG "get_job_config"
#define FUNCTION_NAME_GET_JOB_CONFIG_SCHEMA "get_job_config_schema"
#define FUNCTION_NAME_SET_PLUGIN_CONFIG "set_plugin_config"
#define FUNCTION_NAME_SET_MODULE_CONFIG "set_module_config"
#define FUNCTION_NAME_SET_JOB_CONFIG "set_job_config"
#define FUNCTION_NAME_DELETE_JOB "delete_job"

#define DYNCFG_MAX_WORDS 5

#define DYNCFG_VFNC_RET_CFG_ACCEPTED (1)

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

static inline const char *module_type2str(enum module_type type)
{
    switch (type) {
        case MOD_TYPE_ARRAY:
            return "job_array";
        case MOD_TYPE_SINGLE:
            return "single";
        default:
            return "unknown";
    }
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

static inline enum job_status str2job_state(const char *state_name) {
    if (strcmp(state_name, "stopped") == 0)
        return JOB_STATUS_STOPPED;
    else if (strcmp(state_name, "running") == 0)
        return JOB_STATUS_RUNNING;
    else if (strcmp(state_name, "error") == 0)
        return JOB_STATUS_ERROR;
    return JOB_STATUS_UNKNOWN;
}

const char *job_status2str(enum job_status status);

enum set_config_result {
    SET_CONFIG_ACCEPTED = 0,
    SET_CONFIG_REJECTED,
    SET_CONFIG_DEFFER
};

typedef uint32_t dyncfg_job_flg_t;
enum job_flags {
    JOB_FLG_PS_LOADED        = 1 << 0, // PS abbr. Persistent Storage
    JOB_FLG_PLUGIN_PUSHED    = 1 << 1, // got it from plugin (e.g. autodiscovered job)
    JOB_FLG_STREAMING_PUSHED = 1 << 2, // got it through streaming
    JOB_FLG_USER_CREATED     = 1 << 3, // user created this job during agent runtime
};

enum job_type {
    JOB_TYPE_UNKNOWN = 0,
    JOB_TYPE_STOCK = 1,
    JOB_TYPE_USER = 2,
    JOB_TYPE_AUTODISCOVERED = 3,
};

static inline const char* job_type2str(enum job_type type)
{
    switch (type) {
        case JOB_TYPE_STOCK:
            return "stock";
        case JOB_TYPE_USER:
            return "user";
        case JOB_TYPE_AUTODISCOVERED:
            return "autodiscovered";
        case JOB_TYPE_UNKNOWN:
        default:
            return "unknown";
    }
}

static inline enum job_type dyncfg_str2job_type(const char *type_name)
{
    if (strcmp(type_name, "stock") == 0)
        return JOB_TYPE_STOCK;
    else if (strcmp(type_name, "user") == 0)
        return JOB_TYPE_USER;
    else if (strcmp(type_name, "autodiscovered") == 0)
        return JOB_TYPE_AUTODISCOVERED;
    error_report("Unknown job type: %s", type_name);
    return JOB_TYPE_UNKNOWN;
}

struct job
{
    const char *name;
    enum job_type type;
    struct module *module;

    pthread_mutex_t lock;
    // lock protexts only fields below (which are modified during job existence)
    // others are static during lifetime of job

    int dirty; // this relates to rrdpush, true if parent has different data than us

    // state reported by plugin
    usec_t last_state_update;
    enum job_status status; // reported by plugin, enum as this has to be interpreted by UI
    int state; // code reported by plugin which can mean anything plugin wants
    char *reason; // reported by plugin, can be NULL (optional)

    dyncfg_job_flg_t flags;
};

struct module
{
    pthread_mutex_t lock;
    char *name;
    enum module_type type;

    struct configurable_plugin *plugin;

    // module config
    enum set_config_result (*set_config_cb)(void *usr_ctx, const char *plugin_name, const char *module_name, dyncfg_config_t *cfg);
    dyncfg_config_t (*get_config_cb)(void *usr_ctx, const char *plugin_name, const char *module_name);
    dyncfg_config_t (*get_config_schema_cb)(void *usr_ctx, const char *plugin_name, const char *module_name);
    void *config_cb_usr_ctx;

    DICTIONARY *jobs;

    // jobs config
    dyncfg_config_t (*get_job_config_cb)(void *usr_ctx, const char *plugin_name, const char *module_name, const char *job_name);
    dyncfg_config_t (*get_job_config_schema_cb)(void *usr_ctx, const char *plugin_name, const char *module_name);
    enum set_config_result (*set_job_config_cb)(void *usr_ctx, const char *plugin_name, const char *module_name, const char *job_name, dyncfg_config_t *cfg);
    enum set_config_result (*delete_job_cb)(void *usr_ctx, const char *plugin_name, const char *module_name, const char *job_name);
    void *job_config_cb_usr_ctx;
};

struct configurable_plugin {
    pthread_mutex_t lock;
    char *name;
    DICTIONARY *modules;
    const char *schema;

    dyncfg_config_t (*get_config_cb)(void *usr_ctx, const char *plugin_name);
    dyncfg_config_t (*get_config_schema_cb)(void *usr_ctx, const char *plugin_name);
    enum set_config_result (*set_config_cb)(void *usr_ctx, const char *plugin_name, dyncfg_config_t *cfg);
    void *cb_usr_ctx; // context for all callbacks (split if needed in future)
};

// API to be used by plugins
const DICTIONARY_ITEM *register_plugin(DICTIONARY *plugins_dict, struct configurable_plugin *plugin, bool localhost);
void unregister_plugin(DICTIONARY *plugins_dict, const DICTIONARY_ITEM *plugin);
int register_module(DICTIONARY *plugins_dict, struct configurable_plugin *plugin, struct module *module, bool localhost);
int register_job(DICTIONARY *plugins_dict, const char *plugin_name, const char *module_name, const char *job_name, enum job_type job_type, dyncfg_job_flg_t flags, int ignore_existing);

const DICTIONARY_ITEM *report_job_status_acq_lock(DICTIONARY *plugins_dict, const DICTIONARY_ITEM **plugin_acq_item, DICTIONARY **job_dict, const char *plugin_name, const char *module_name, const char *job_name, enum job_status status, int status_code, char *reason);

void dyn_conf_store_config(const char *function, const char *payload, struct configurable_plugin *plugin);
void unlink_job(const char *plugin_name, const char *module_name, const char *job_name);
void delete_job(struct configurable_plugin *plugin, const char *module_name, const char *job_name);
void delete_job_pname(DICTIONARY *plugins_dict, const char *plugin_name, const char *module_name, const char *job_name);

// API to be used by the web server(s)
json_object *get_list_of_plugins_json(DICTIONARY *plugins_dict);
struct configurable_plugin *get_plugin_by_name(DICTIONARY *plugins_dict, const char *name);

json_object *get_list_of_modules_json(struct configurable_plugin *plugin);
struct module *get_module_by_name(struct configurable_plugin *plugin, const char *module_name);

json_object *job2json(struct job *job);

// helper struct to make interface between internal webserver and h2o same
struct uni_http_response {
    int status;
    char *content;
    size_t content_length;
    HTTP_CONTENT_TYPE content_type;
    void (*content_free)(void *);
};

struct uni_http_response dyn_conf_process_http_request(DICTIONARY *plugins_dict, int method, const char *plugin, const char *module, const char *job_id, void *payload, size_t payload_size);

// API to be used by main netdata process, initialization and destruction etc.
int dyn_conf_init(void);
void freez_dyncfg(void *ptr);

#define dyncfg_dictionary_create() dictionary_create(DICT_OPTION_VALUE_LINK_DONT_CLONE)

void plugin_del_cb(const DICTIONARY_ITEM *item, void *value, void *data);

void *dyncfg_main(void *in);

#define DYNCFG_FUNCTION_TYPE_REGULAR (1 << 0)
#define DYNCFG_FUNCTION_TYPE_PAYLOAD (1 << 1)
#define DYNCFG_FUNCTION_TYPE_GET     (1 << 2)
#define DYNCFG_FUNCTION_TYPE_SET     (1 << 3)
#define DYNCFG_FUNCTION_TYPE_DELETE  (1 << 4)
#define DYNCFG_FUNCTION_TYPE_ALL \
    (DYNCFG_FUNCTION_TYPE_REGULAR | DYNCFG_FUNCTION_TYPE_PAYLOAD | DYNCFG_FUNCTION_TYPE_GET | DYNCFG_FUNCTION_TYPE_SET | DYNCFG_FUNCTION_TYPE_DELETE)

bool is_dyncfg_function(const char *function_name, uint8_t type);

#endif //DYN_CONF_H
