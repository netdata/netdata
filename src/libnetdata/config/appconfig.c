// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

typedef enum __attribute__((packed)) {
    CONFIG_VALUE_LOADED = (1 << 0),         // has been loaded from the config
    CONFIG_VALUE_USED = (1 << 1),           // has been accessed from the program
    CONFIG_VALUE_CHANGED = (1 << 2),        // has been changed from the loaded value or the internal default value
    CONFIG_VALUE_CHECKED = (1 << 3),        // has been checked if the value is different from the default
    CONFIG_VALUE_MIGRATED = (1 << 4),       // has been migrated from an old config
    CONFIG_VALUE_REFORMATTED = (1 << 5),    // has been reformatted with the official formatting
} CONFIG_VALUE_FLAGS;

struct config_option {
    avl_t avl_node;         // the index entry of this entry - this has to be first!

    CONFIG_VALUE_FLAGS flags;

    STRING *name;
    STRING *value;

    struct config_option *next; // config->mutex protects just this
};

struct section {
    avl_t avl_node;         // the index entry of this section - this has to be first!

    STRING *name;

    struct section *next;    // global config_mutex protects just this

    struct config_option *values;
    avl_tree_lock values_index;

    netdata_mutex_t mutex;  // this locks only the writers, to ensure atomic updates
                           // readers are protected using the rwlock in avl_tree_lock
};

static void appconfig_wrlock(struct config *root);
static void appconfig_unlock(struct config *root);
static void config_section_wrlock(struct section *co);
static void config_section_unlock(struct section *co);

typedef STRING *(*reformat_t)(STRING *value);
static const char *appconfig_get_by_section(struct section *co, const char *name, const char *default_value, reformat_t cb);
static int appconfig_get_boolean_by_section(struct section *co, const char *name, int value);

size_t appconfig_foreach_value_in_section(struct config *root, const char *section, appconfig_foreach_value_cb_t cb, void *data) {
    size_t used = 0;
    struct section *co = appconfig_get_section(root, section);
    if(co) {
        config_section_wrlock(co);
        struct config_option *cv;
        for(cv = co->values; cv ; cv = cv->next) {
            if(cb(data, string2str(cv->name), string2str(cv->value))) {
                cv->flags |= CONFIG_VALUE_USED;
                used++;
            }
        }
        config_section_unlock(co);
    }

    return used;
}

/*
 * @Input:
 *      Connector / instance to add to an internal structure
 * @Return
 *      The current head of the linked list of connector_instance
 *
 */

_CONNECTOR_INSTANCE *add_connector_instance(struct section *connector, struct section *instance)
{
    static struct _connector_instance *global_connector_instance = NULL;
    struct _connector_instance *local_ci, *local_ci_tmp;

    if (unlikely(!connector)) {
        if (unlikely(!instance))
            return global_connector_instance;

        local_ci = global_connector_instance;
        while (local_ci) {
            local_ci_tmp = local_ci->next;
            freez(local_ci);
            local_ci = local_ci_tmp;
        }
        global_connector_instance = NULL;
        return NULL;
    }

    local_ci = callocz(1, sizeof(struct _connector_instance));
    local_ci->instance = instance;
    local_ci->connector = connector;
    strncpyz(local_ci->instance_name, string2str(instance->name), CONFIG_MAX_NAME);
    strncpyz(local_ci->connector_name, string2str(connector->name), CONFIG_MAX_NAME);
    local_ci->next = global_connector_instance;
    global_connector_instance = local_ci;

    return global_connector_instance;
}

int is_valid_connector(char *type, int check_reserved)
{
    int rc = 1;

    if (unlikely(!type))
        return 0;

    if (!check_reserved) {
        if (unlikely(is_valid_connector(type,1))) {
            return 0;
        }
        //if (unlikely(*type == ':')
        //    return 0;
        char *separator = strrchr(type, ':');
        if (likely(separator)) {
            *separator = '\0';
            rc = (int)(separator - type);
        } else
            return 0;
    }
//    else {
//        if (unlikely(is_valid_connector(type,1))) {
//            netdata_log_error("Section %s invalid -- reserved name", type);
//            return 0;
//        }
//    }

    if (!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {
        return rc;
    } else if (!strcmp(type, "graphite:http") || !strcmp(type, "graphite:https")) {
        return rc;
    } else if (!strcmp(type, "json") || !strcmp(type, "json:plaintext")) {
        return rc;
    } else if (!strcmp(type, "json:http") || !strcmp(type, "json:https")) {
        return rc;
    } else if (!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {
        return rc;
    } else if (!strcmp(type, "opentsdb:http") || !strcmp(type, "opentsdb:https")) {
        return rc;
    } else if (!strcmp(type, "prometheus_remote_write")) {
        return rc;
    } else if (!strcmp(type, "prometheus_remote_write:http") || !strcmp(type, "prometheus_remote_write:https")) {
        return rc;
    } else if (!strcmp(type, "kinesis") || !strcmp(type, "kinesis:plaintext")) {
        return rc;
    } else if (!strcmp(type, "pubsub") || !strcmp(type, "pubsub:plaintext")) {
        return rc;
    } else if (!strcmp(type, "mongodb") || !strcmp(type, "mongodb:plaintext")) {
        return rc;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// locking

static inline void appconfig_wrlock(struct config *root) {
    netdata_mutex_lock(&root->mutex);
}

static inline void appconfig_unlock(struct config *root) {
    netdata_mutex_unlock(&root->mutex);
}

static inline void config_section_wrlock(struct section *co) {
    netdata_mutex_lock(&co->mutex);
}

static inline void config_section_unlock(struct section *co) {
    netdata_mutex_unlock(&co->mutex);
}


// ----------------------------------------------------------------------------
// config name-value index

static int appconfig_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->name < ((struct config_option *)b)->name) return -1;
    else if(((struct config_option *)a)->name > ((struct config_option *)b)->name) return 1;
    else return string_cmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

#define appconfig_option_index_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl_t *)(cv))
#define appconfig_option_index_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl_t *)(cv))

static struct config_option *appconfig_option_index_find(struct section *co, const char *name) {
    struct config_option tmp;
    tmp.name = string_strdupz(name);

    struct config_option *rc = (struct config_option *)avl_search_lock(&(co->values_index), (avl_t *) &tmp);

    string_freez(tmp.name);
    return rc;
}


// ----------------------------------------------------------------------------
// config sections index

int appconfig_section_compare(void *a, void *b) {
    if(((struct section *)a)->name < ((struct section *)b)->name) return -1;
    else if(((struct section *)a)->name > ((struct section *)b)->name) return 1;
    else return string_cmp(((struct section *)a)->name, ((struct section *)b)->name);
}

#define appconfig_index_add(root, cfg) (struct section *)avl_insert_lock(&(root)->index, (avl_t *)(cfg))
#define appconfig_index_del(root, cfg) (struct section *)avl_remove_lock(&(root)->index, (avl_t *)(cfg))

static struct section *appconfig_index_find(struct config *root, const char *name) {
    struct section tmp;
    tmp.name = string_strdupz(name);

    struct section *rc = (struct section *)avl_search_lock(&root->index, (avl_t *) &tmp);
    string_freez(tmp.name);
    return rc;
}


// ----------------------------------------------------------------------------
// config section methods

static inline struct section *appconfig_section_find(struct config *root, const char *section) {
    return appconfig_index_find(root, section);
}

static inline struct section *appconfig_section_create(struct config *root, const char *section) {
    netdata_log_debug(D_CONFIG, "Creating section '%s'.", section);

    struct section *co = callocz(1, sizeof(struct section));
    co->name = string_strdupz(section);
    netdata_mutex_init(&co->mutex);

    avl_init_lock(&co->values_index, appconfig_option_compare);

    if(unlikely(appconfig_index_add(root, co) != co))
        netdata_log_error("INTERNAL ERROR: indexing of section '%s', already exists.",
                          string2str(co->name));

    appconfig_wrlock(root);
    struct section *co2 = root->last_section;
    if(co2) {
        co2->next = co;
    } else {
        root->first_section = co;
    }
    root->last_section = co;
    appconfig_unlock(root);

    return co;
}

void appconfig_section_destroy_non_loaded(struct config *root, const char *section)
{
    struct section *co;
    struct config_option *cv, *cv_next;

    netdata_log_debug(D_CONFIG, "Destroying section '%s'.", section);

    co = appconfig_section_find(root, section);
    if(!co) {
        netdata_log_error("Could not destroy section '%s'. Not found.", section);
        return;
    }

    config_section_wrlock(co);
    for(cv = co->values; cv ; cv = cv->next) {
        if (cv->flags & CONFIG_VALUE_LOADED) {
            /* Do not destroy values that were loaded from the configuration files. */
            config_section_unlock(co);
            return;
        }
    }
    for(cv = co->values ; cv ; cv = cv_next) {
        cv_next = cv->next;
        if(unlikely(!appconfig_option_index_del(co, cv)))
            netdata_log_error("Cannot remove config option '%s' from section '%s'.",
                              string2str(cv->name), string2str(co->name));
        string_freez(cv->value);
        string_freez(cv->name);
        freez(cv);
    }
    co->values = NULL;
    config_section_unlock(co);

    if (unlikely(!appconfig_index_del(root, co))) {
        netdata_log_error("Cannot remove section '%s' from config.", section);
        return;
    }
    
    appconfig_wrlock(root);

    if (root->first_section == co) {
        root->first_section = co->next;

        if (root->last_section == co)
            root->last_section = root->first_section;
    } else {
        struct section *co_cur = root->first_section, *co_prev = NULL;

        while(co_cur && co_cur != co) {
            co_prev = co_cur;
            co_cur = co_cur->next;
        }

        if (co_cur) {
            co_prev->next = co_cur->next;

            if (root->last_section == co_cur)
                root->last_section = co_prev;
        }
    }

    appconfig_unlock(root);

    avl_destroy_lock(&co->values_index);
    string_freez(co->name);
    pthread_mutex_destroy(&co->mutex);
    freez(co);
}

void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name)
{
    netdata_log_debug(D_CONFIG, "Destroying section option '%s -> %s'.", section, name);

    struct section *co;
    co = appconfig_section_find(root, section);
    if (!co) {
        netdata_log_error("Could not destroy section option '%s -> %s'. The section not found.", section, name);
        return;
    }

    config_section_wrlock(co);

    struct config_option *cv;

    cv = appconfig_option_index_find(co, name);

    if (cv && cv->flags & CONFIG_VALUE_LOADED) {
        config_section_unlock(co);
        return;
    }

    if (unlikely(!(cv && appconfig_option_index_del(co, cv)))) {
        config_section_unlock(co);
        netdata_log_error("Could not destroy section option '%s -> %s'. The option not found.", section, name);
        return;
    }

    if (co->values == cv) {
        co->values = co->values->next;
    } else {
        struct config_option *cv_cur = co->values, *cv_prev = NULL;
        while (cv_cur && cv_cur != cv) {
            cv_prev = cv_cur;
            cv_cur = cv_cur->next;
        }
        if (cv_cur) {
            cv_prev->next = cv_cur->next;
        }
    }

    string_freez(cv->value);
    string_freez(cv->name);
    freez(cv);

    config_section_unlock(co);
}

// ----------------------------------------------------------------------------
// config name-value methods

static inline struct config_option *appconfig_value_create(struct section *co, const char *name, const char *value) {
    netdata_log_debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

    struct config_option *cv = callocz(1, sizeof(struct config_option));
    cv->name = string_strdupz(name);
    cv->value = string_strdupz(value);

    struct config_option *found = appconfig_option_index_add(co, cv);
    if(found != cv) {
        netdata_log_error("indexing of config '%s' in section '%s': already exists - using the existing one.",
                          string2str(cv->name), string2str(co->name));
        string_freez(cv->value);
        string_freez(cv->name);
        freez(cv);
        return found;
    }

    config_section_wrlock(co);
    struct config_option *cv2 = co->values;
    if(cv2) {
        while (cv2->next) cv2 = cv2->next;
        cv2->next = cv;
    }
    else co->values = cv;
    config_section_unlock(co);

    return cv;
}

int appconfig_exists(struct config *root, const char *section, const char *name) {
    struct config_option *cv;

    netdata_log_debug(D_CONFIG, "request to get config in section '%s', name '%s'", section, name);

    struct section *co = appconfig_section_find(root, section);
    if(!co) return 0;

    cv = appconfig_option_index_find(co, name);
    if(!cv) return 0;

    return 1;
}

int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *cv_old, *cv_new;
    int ret = -1;

    netdata_log_debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct section *co_old = appconfig_section_find(root, section_old);
    if(!co_old) return ret;

    struct section *co_new = appconfig_section_find(root, section_new);
    if(!co_new) co_new = appconfig_section_create(root, section_new);

    config_section_wrlock(co_old);
    if(co_old != co_new)
        config_section_wrlock(co_new);

    cv_old = appconfig_option_index_find(co_old, name_old);
    if(!cv_old) goto cleanup;

    cv_new = appconfig_option_index_find(co_new, name_new);
    if(cv_new) goto cleanup;

    if(unlikely(appconfig_option_index_del(co_old, cv_old) != cv_old))
        netdata_log_error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.",
                          string2str(cv_old->name), string2str(co_old->name));

    if(co_old->values == cv_old) {
        co_old->values = cv_old->next;
    }
    else {
        struct config_option *t;
        for(t = co_old->values; t && t->next != cv_old ;t = t->next) ;
        if(!t || t->next != cv_old)
            netdata_log_error("INTERNAL ERROR: cannot find variable '%s' in section '%s' of the config - but it should be there.",
                              string2str(cv_old->name), string2str(co_old->name));
        else
            t->next = cv_old->next;
    }

    nd_log(NDLS_DAEMON, NDLP_WARNING,
           "CONFIG: option '[%s].%s' has been migrated to '[%s].%s'.",
           section_old, name_old,
           section_new, name_new);

    string_freez(cv_old->name);
    cv_old->name = string_strdupz(name_new);
    cv_old->flags |= CONFIG_VALUE_MIGRATED;

    cv_new = cv_old;
    cv_new->next = co_new->values;
    co_new->values = cv_new;

    if(unlikely(appconfig_option_index_add(co_new, cv_old) != cv_old))
        netdata_log_error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.",
                          string2str(cv_old->name), string2str(co_new->name));

    ret = 0;

cleanup:
    if(co_old != co_new)
        config_section_unlock(co_new);
    config_section_unlock(co_old);
    return ret;
}

int appconfig_move_everywhere(struct config *root, const char *name_old, const char *name_new) {
    int ret = -1;
    appconfig_wrlock(root);
    struct section *co;
    for(co = root->first_section; co ; co = co->next) {
        appconfig_unlock(root);
        if(appconfig_move(root, string2str(co->name), name_old, string2str(co->name), name_new) == 0)
            ret = 0;
        appconfig_wrlock(root);
    }
    appconfig_unlock(root);
    return ret;
}

static const char *appconfig_get_by_section(struct section *co, const char *name, const char *default_value, reformat_t cb) {
    struct config_option *cv;

    // Only calls internal to this file check for a NULL result, and they do not supply a NULL arg.
    // External caller should treat NULL as an error case.
    cv = appconfig_option_index_find(co, name);
    if (!cv) {
        if (!default_value) return NULL;
        cv = appconfig_value_create(co, name, default_value);
        if (!cv) return NULL;
    }
    cv->flags |= CONFIG_VALUE_USED;

    if((cv->flags & CONFIG_VALUE_LOADED) || (cv->flags & CONFIG_VALUE_CHANGED)) {
        // this is a loaded value from the config file
        // if it is different from the default, mark it
        if(!(cv->flags & CONFIG_VALUE_CHECKED)) {
            if(default_value && string_strcmp(cv->value, default_value) != 0)
                cv->flags |= CONFIG_VALUE_CHANGED;

            cv->flags |= CONFIG_VALUE_CHECKED;
        }
    }

    if((cv->flags & CONFIG_VALUE_MIGRATED) && !(cv->flags & CONFIG_VALUE_REFORMATTED) && cb) {
        cv->value = cb(cv->value);
        cv->flags |= CONFIG_VALUE_REFORMATTED;
    }

    return string2str(cv->value);
}

static const char *appconfig_get_and_reformat(struct config *root, const char *section, const char *name, const char *default_value, reformat_t cb) {
    if (default_value == NULL)
        netdata_log_debug(D_CONFIG, "request to get config in section '%s', name '%s' or fail", section, name);
    else
        netdata_log_debug(D_CONFIG, "request to get config in section '%s', name '%s', default_value '%s'", section, name, default_value);

    struct section *co = appconfig_section_find(root, section);
    if (!co && !default_value)
        return NULL;
    if(!co) co = appconfig_section_create(root, section);

    return appconfig_get_by_section(co, name, default_value, cb);
}

const char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value) {
    return appconfig_get_and_reformat(root, section, name, default_value, NULL);
}

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value)
{
    char buffer[100];
    const char *s;
    sprintf(buffer, "%lld", value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return strtoll(s, NULL, 0);
}

NETDATA_DOUBLE appconfig_get_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value)
{
    char buffer[100];
    const char *s;
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return str2ndd(s, NULL);
}

inline int appconfig_test_boolean_value(const char *s) {
    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on")
       || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand"))
        return 1;

    return 0;
}

static int appconfig_get_boolean_by_section(struct section *co, const char *name, int value) {
    const char *s = appconfig_get_by_section(co, name, (!value)?"no":"yes", NULL);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value)
{
    const char *s;
    if(value) s = "yes";
    else s = "no";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value)
{
    const char *s;

    if(value == CONFIG_BOOLEAN_AUTO)
        s = "auto";

    else if(value == CONFIG_BOOLEAN_NO)
        s = "no";

    else
        s = "yes";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    if(!strcmp(s, "yes") || !strcmp(s, "true") || !strcmp(s, "on"))
        return CONFIG_BOOLEAN_YES;
    else if(!strcmp(s, "no") || !strcmp(s, "false") || !strcmp(s, "off"))
        return CONFIG_BOOLEAN_NO;
    else if(!strcmp(s, "auto") || !strcmp(s, "on demand"))
        return CONFIG_BOOLEAN_AUTO;

    return value;
}

const char *appconfig_set_default(struct config *root, const char *section, const char *name, const char *value)
{
    struct config_option *cv;

    netdata_log_debug(D_CONFIG, "request to set default config in section '%s', name '%s', value '%s'", section, name, value);

    struct section *co = appconfig_section_find(root, section);
    if(!co) return appconfig_set(root, section, name, value);

    cv = appconfig_option_index_find(co, name);
    if(!cv) return appconfig_set(root, section, name, value);

    cv->flags |= CONFIG_VALUE_USED;

    if(cv->flags & CONFIG_VALUE_LOADED)
        return string2str(cv->value);

    if(string_strcmp(cv->value, value) != 0) {
        cv->flags |= CONFIG_VALUE_CHANGED;

        string_freez(cv->value);
        cv->value = string_strdupz(value);
    }

    return string2str(cv->value);
}

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value)
{
    struct config_option *cv;

    netdata_log_debug(D_CONFIG, "request to set config in section '%s', name '%s', value '%s'", section, name, value);

    struct section *co = appconfig_section_find(root, section);
    if(!co) co = appconfig_section_create(root, section);

    cv = appconfig_option_index_find(co, name);
    if(!cv) cv = appconfig_value_create(co, name, value);
    cv->flags |= CONFIG_VALUE_USED;

    if(string_strcmp(cv->value, value) != 0) {
        cv->flags |= CONFIG_VALUE_CHANGED;

        string_freez(cv->value);
        cv->value = string_strdupz(value);
    }

    return value;
}

long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value)
{
    char buffer[100];
    sprintf(buffer, "%lld", value);

    appconfig_set(root, section, name, buffer);

    return value;
}

NETDATA_DOUBLE appconfig_set_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value)
{
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    appconfig_set(root, section, name, buffer);

    return value;
}

int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value)
{
    char *s;
    if(value) s = "yes";
    else s = "no";

    appconfig_set(root, section, name, s);

    return value;
}

static STRING *reformat_duration_seconds(STRING *value) {
    int result = 0;
    if(!duration_parse_seconds(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_time_t(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

time_t appconfig_get_duration_seconds(struct config *root, const char *section, const char *name, time_t default_value) {

    char default_str[128];
    duration_snprintf_time_t(default_str, sizeof(default_str), default_value);

    const char *s = appconfig_get_and_reformat(root, section, name, default_str, reformat_duration_seconds);
    if(!s)
        return default_value;

    int result = 0;
    if(!duration_parse_seconds(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

time_t appconfig_set_duration_seconds(struct config *root, const char *section, const char *name, time_t value) {
    char str[128];
    duration_snprintf_time_t(str, sizeof(str), value);

    appconfig_set(root, section, name, str);

    return value;
}

static STRING *reformat_duration_ms(STRING *value) {
    int64_t result = 0;
    if(!duration_parse_msec_t(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_msec_t(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

msec_t appconfig_get_duration_ms(struct config *root, const char *section, const char *name, msec_t default_value) {

    char default_str[128];
    duration_snprintf_msec_t(default_str, sizeof(default_str), default_value);

    const char *s = appconfig_get_and_reformat(root, section, name, default_str, reformat_duration_ms);
    if(!s)
        return default_value;

    smsec_t result = 0;
    if(!duration_parse_msec_t(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

msec_t appconfig_set_duration_ms(struct config *root, const char *section, const char *name, msec_t value) {
    char str[128];
    duration_snprintf_msec_t(str, sizeof(str), (smsec_t)value);

    appconfig_set(root, section, name, str);

    return value;
}

static STRING *reformat_duration_days(STRING *value) {
    int64_t result = 0;
    if(!duration_parse_days(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_days(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_duration_days(struct config *root, const char *section, const char *name, unsigned default_value) {

    char default_str[128];
    duration_snprintf_days(default_str, sizeof(default_str), (int)default_value);

    const char *s = appconfig_get_and_reformat(root, section, name, default_str, reformat_duration_days);
    if(!s)
        return default_value;

    int64_t result = 0;
    if(!duration_parse_days(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return (unsigned)ABS(result);
}

unsigned appconfig_set_duration_days(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    duration_snprintf_days(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}

static STRING *reformat_size_bytes(STRING *value) {
    uint64_t result = 0;
    if(!size_parse_bytes(string2str(value), &result))
        return value;

    char buf[128];
    if(size_snprintf_bytes(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_size_bytes(struct config *root, const char *section, const char *name, unsigned default_value) {

    char default_str[128];
    size_snprintf_bytes(default_str, sizeof(default_str), (int)default_value);

    const char *s = appconfig_get_and_reformat(root, section, name, default_str, reformat_size_bytes);
    if(!s)
        return default_value;

    uint64_t result = 0;
    if(!size_parse_bytes(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

unsigned appconfig_set_size_bytes(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    size_snprintf_bytes(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}

static STRING *reformat_size_mb(STRING *value) {
    uint64_t result = 0;
    if(!size_parse_mb(string2str(value), &result))
        return value;

    char buf[128];
    if(size_snprintf_mb(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_size_mb(struct config *root, const char *section, const char *name, unsigned default_value) {

    char default_str[128];
    size_snprintf_mb(default_str, sizeof(default_str), (int)default_value);

    const char *s = appconfig_get_and_reformat(root, section, name, default_str, reformat_size_mb);
    if(!s)
        return default_value;

    uint64_t result = 0;
    if(!size_parse_mb(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

unsigned appconfig_set_size_mb(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    size_snprintf_mb(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}

// ----------------------------------------------------------------------------
// config load/save

int appconfig_load(struct config *root, char *filename, int overwrite_used, const char *section_name) {
    int line = 0;
    struct section *co = NULL;
    int is_exporter_config = 0;
    int _connectors = 0;              // number of exporting connector sections we have
    char working_instance[CONFIG_MAX_NAME + 1];
    char working_connector[CONFIG_MAX_NAME + 1];
    struct section *working_connector_section = NULL;
    int global_exporting_section = 0;

    char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

    if(!filename) filename = CONFIG_DIR "/" CONFIG_FILENAME;

    netdata_log_debug(D_CONFIG, "CONFIG: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        if(errno != ENOENT)
            netdata_log_info("CONFIG: cannot open file '%s'. Using internal defaults.", filename);

        return 0;
    }

    CLEAN_STRING *section_string = string_strdupz(section_name);
    is_exporter_config = (strstr(filename, EXPORTING_CONF) != NULL);

    while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if(!s || *s == '#') {
            netdata_log_debug(D_CONFIG, "CONFIG: ignoring line %d of file '%s', it is empty.", line, filename);
            continue;
        }

        int len = (int) strlen(s);
        if(*s == '[' && s[len - 1] == ']') {
            // new section
            s[len - 1] = '\0';
            s++;

            if (is_exporter_config) {
                global_exporting_section = !(strcmp(s, CONFIG_SECTION_EXPORTING)) || !(strcmp(s, CONFIG_SECTION_PROMETHEUS));

                if (unlikely(!global_exporting_section)) {
                    int rc;
                    rc = is_valid_connector(s, 0);
                    if (likely(rc)) {
                        strncpyz(working_connector, s, CONFIG_MAX_NAME);
                        s = s + rc + 1;
                        if (unlikely(!(*s))) {
                            _connectors++;
                            sprintf(buffer, "instance_%d", _connectors);
                            s = buffer;
                        }
                        strncpyz(working_instance, s, CONFIG_MAX_NAME);
                        working_connector_section = NULL;
                        if (unlikely(appconfig_section_find(root, working_instance))) {
                            netdata_log_error("Instance (%s) already exists", working_instance);
                            co = NULL;
                            continue;
                        }
                    }
                    else {
                        co = NULL;
                        netdata_log_error("Section (%s) does not specify a valid connector", s);
                        continue;
                    }
                }
            }

            co = appconfig_section_find(root, s);
            if(!co) co = appconfig_section_create(root, s);

            if(co && section_string && overwrite_used && section_string == co->name) {
                config_section_wrlock(co);
                struct config_option *cv2 = co->values;
                while (cv2) {
                    struct config_option *save = cv2->next;
                    struct config_option *found = appconfig_option_index_del(co, cv2);
                    if(found != cv2)
                        netdata_log_error("INTERNAL ERROR: Cannot remove '%s' from  section '%s', it was not inserted before.",
                                          string2str(cv2->name), string2str(co->name));

                    string_freez(cv2->name);
                    string_freez(cv2->value);
                    freez(cv2);
                    cv2 = save;
                }
                co->values = NULL;
                config_section_unlock(co);
            }

            continue;
        }

        if(!co) {
            // line outside a section
            netdata_log_error("CONFIG: ignoring line %d ('%s') of file '%s', it is outside all sections.", line, s, filename);
            continue;
        }

        if(section_string && overwrite_used && section_string != co->name)
            continue;

        char *name = s;
        char *value = strchr(s, '=');
        if(!value) {
            netdata_log_error("CONFIG: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(!name || *name == '#') {
            netdata_log_error("CONFIG: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value) value = "";

        struct config_option *cv = appconfig_option_index_find(co, name);

        if (!cv) {
            cv = appconfig_value_create(co, name, value);
            if (likely(is_exporter_config) && unlikely(!global_exporting_section)) {
                if (unlikely(!working_connector_section)) {
                    working_connector_section = appconfig_section_find(root, working_connector);
                    if (!working_connector_section)
                        working_connector_section = appconfig_section_create(root, working_connector);
                    if (likely(working_connector_section)) {
                        add_connector_instance(working_connector_section, co);
                    }
                }
            }
        }
        else {
            if (((cv->flags & CONFIG_VALUE_USED) && overwrite_used) || !(cv->flags & CONFIG_VALUE_USED)) {
                netdata_log_debug(
                    D_CONFIG, "CONFIG: line %d of file '%s', overwriting '%s/%s'.", line, filename, co->name, cv->name);
                string_freez(cv->value);
                cv->value = string_strdupz(value);
            } else
                netdata_log_debug(
                    D_CONFIG,
                    "CONFIG: ignoring line %d of file '%s', '%s/%s' is already present and used.",
                    line,
                    filename,
                    co->name,
                    cv->name);
        }
        cv->flags |= CONFIG_VALUE_LOADED;
    }

    fclose(fp);

    return 1;
}

void appconfig_generate(struct config *root, BUFFER *wb, int only_changed, bool netdata_conf)
{
    int i, pri;
    struct section *co;
    struct config_option *cv;

    {
        int found_host_labels = 0;
        for (co = root->first_section; co; co = co->next)
            if(!string_strcmp(co->name, CONFIG_SECTION_HOST_LABEL))
                found_host_labels = 1;

        if(netdata_conf && !found_host_labels) {
            appconfig_section_create(root, CONFIG_SECTION_HOST_LABEL);
            appconfig_get(root, CONFIG_SECTION_HOST_LABEL, "name", "value");
        }
    }

    if(netdata_conf) {
    buffer_strcat(wb,
                  "# netdata configuration\n"
                  "#\n"
                  "# You can download the latest version of this file, using:\n"
                  "#\n"
                  "#  wget -O /etc/netdata/netdata.conf http://localhost:19999/netdata.conf\n"
                  "# or\n"
                  "#  curl -o /etc/netdata/netdata.conf http://localhost:19999/netdata.conf\n"
                  "#\n"
                  "# You can uncomment and change any of the options below.\n"
                  "# The value shown in the commented settings, is the default value.\n"
                  "#\n"
                  "\n# global netdata configuration\n");
    }

    for(i = 0; i <= 17 ;i++) {
        appconfig_wrlock(root);
        for(co = root->first_section; co ; co = co->next) {
            if(!string_strcmp(co->name, CONFIG_SECTION_GLOBAL))                 pri = 0;
            else if(!string_strcmp(co->name, CONFIG_SECTION_DB))                pri = 1;
            else if(!string_strcmp(co->name, CONFIG_SECTION_DIRECTORIES))       pri = 2;
            else if(!string_strcmp(co->name, CONFIG_SECTION_LOGS))              pri = 3;
            else if(!string_strcmp(co->name, CONFIG_SECTION_ENV_VARS))          pri = 4;
            else if(!string_strcmp(co->name, CONFIG_SECTION_HOST_LABEL))        pri = 5;
            else if(!string_strcmp(co->name, CONFIG_SECTION_SQLITE))            pri = 6;
            else if(!string_strcmp(co->name, CONFIG_SECTION_CLOUD))             pri = 7;
            else if(!string_strcmp(co->name, CONFIG_SECTION_ML))                pri = 8;
            else if(!string_strcmp(co->name, CONFIG_SECTION_HEALTH))            pri = 9;
            else if(!string_strcmp(co->name, CONFIG_SECTION_WEB))               pri = 10;
            else if(!string_strcmp(co->name, CONFIG_SECTION_WEBRTC))            pri = 11;
            // by default, new sections will get pri = 12 (set at the end, below)
            else if(!string_strcmp(co->name, CONFIG_SECTION_REGISTRY))          pri = 13;
            else if(!string_strcmp(co->name, CONFIG_SECTION_GLOBAL_STATISTICS)) pri = 14;
            else if(!string_strcmp(co->name, CONFIG_SECTION_PLUGINS))           pri = 15;
            else if(!string_strcmp(co->name, CONFIG_SECTION_STATSD))            pri = 16;
            else if(!string_strncmp(co->name, "plugin:", 7))                    pri = 17; // << change the loop too if you change this
            else pri = 12; // this is used for any new (currently unknown) sections

            if(i == pri) {
                int loaded = 0;
                int used = 0;
                int changed = 0;
                int count = 0;

                config_section_wrlock(co);
                for(cv = co->values; cv ; cv = cv->next) {
                    used += (cv->flags & CONFIG_VALUE_USED)?1:0;
                    loaded += (cv->flags & CONFIG_VALUE_LOADED)?1:0;
                    changed += (cv->flags & CONFIG_VALUE_CHANGED)?1:0;
                    count++;
                }
                config_section_unlock(co);

                if(!count) continue;
                if(only_changed && !changed && !loaded) continue;

                if(!used)
                    buffer_sprintf(wb, "\n# section '%s' is not used.", string2str(co->name));

                buffer_sprintf(wb, "\n[%s]\n", string2str(co->name));

                size_t options_added = 0;
                config_section_wrlock(co);
                for(cv = co->values; cv ; cv = cv->next) {
                    bool unused = used && !(cv->flags & CONFIG_VALUE_USED);
                    bool migrated = used && (cv->flags & CONFIG_VALUE_MIGRATED);
                    bool reformatted = used && (cv->flags & CONFIG_VALUE_REFORMATTED);

                    if((unused || migrated || reformatted) && options_added)
                        buffer_strcat(wb, "\n");

                    if(unused)
                        buffer_sprintf(wb, "\t# option '%s' is not used.\n", string2str(cv->name));

                    if(migrated && reformatted)
                        buffer_sprintf(wb, "\t# option '%s' has been migrated and reformatted.\n", string2str(cv->name));
                    else {
                        if (migrated)
                            buffer_sprintf(wb, "\t# option '%s' has been migrated.\n", string2str(cv->name));

                        if (reformatted)
                            buffer_sprintf(wb, "\t# option '%s' has been reformatted.\n", string2str(cv->name));
                    }

                    buffer_sprintf(wb, "\t%s%s = %s\n",
                                   (
                                       !(cv->flags & CONFIG_VALUE_LOADED) &&
                                       !(cv->flags & CONFIG_VALUE_CHANGED) &&
                                       (cv->flags & CONFIG_VALUE_USED)
                                   ) ? "# " : "",
                                   string2str(cv->name),
                                   string2str(cv->value));

                    options_added++;
                }
                config_section_unlock(co);
            }
        }
        appconfig_unlock(root);
    }
}

struct section *appconfig_get_section(struct config *root, const char *name)
{
    return appconfig_section_find(root, name);
}

bool stream_conf_needs_dbengine(struct config *root) {
    struct section *co;
    bool ret = false;

    appconfig_wrlock(root);
    for(co = root->first_section; co; co = co->next) {
        if(string_strcmp(co->name, "stream") == 0)
            continue; // the first section is not relevant

        const char *s;

        s = appconfig_get_by_section(co, "enabled", NULL, NULL);
        if(!s || !appconfig_test_boolean_value(s))
            continue;

        s = appconfig_get_by_section(co, "db", NULL, NULL);
        if(s && strcmp(s, "dbengine") == 0) {
            ret = true;
            break;
        }
    }
    appconfig_unlock(root);

    return ret;
}

bool stream_conf_has_uuid_section(struct config *root) {
    struct section *section = NULL;
    bool is_parent = false;

    appconfig_wrlock(root);
    for (section = root->first_section; section; section = section->next) {
        nd_uuid_t uuid;

        if (uuid_parse(string2str(section->name), uuid) != -1 &&
            appconfig_get_boolean_by_section(section, "enabled", 0)) {
            is_parent = true;
            break;
        }
    }
    appconfig_unlock(root);

    return is_parent;
}
