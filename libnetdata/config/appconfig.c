// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

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
    strncpyz(local_ci->instance_name, instance->name, CONFIG_MAX_NAME);
    strncpyz(local_ci->connector_name, connector->name, CONFIG_MAX_NAME);
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
            rc = separator - type;
        } else
            return 0;
    }
//    else {
//        if (unlikely(is_valid_connector(type,1))) {
//            error("Section %s invalid -- reserved name", type);
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

inline void appconfig_wrlock(struct config *root) {
    netdata_mutex_lock(&root->mutex);
}

inline void appconfig_unlock(struct config *root) {
    netdata_mutex_unlock(&root->mutex);
}

inline void config_section_wrlock(struct section *co) {
    netdata_mutex_lock(&co->mutex);
}

inline void config_section_unlock(struct section *co) {
    netdata_mutex_unlock(&co->mutex);
}


// ----------------------------------------------------------------------------
// config name-value index

static int appconfig_option_compare(void *a, void *b) {
    if(((struct config_option *)a)->hash < ((struct config_option *)b)->hash) return -1;
    else if(((struct config_option *)a)->hash > ((struct config_option *)b)->hash) return 1;
    else return strcmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

#define appconfig_option_index_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl_t *)(cv))
#define appconfig_option_index_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl_t *)(cv))

static struct config_option *appconfig_option_index_find(struct section *co, const char *name, uint32_t hash) {
    struct config_option tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = (char *)name;

    return (struct config_option *)avl_search_lock(&(co->values_index), (avl_t *) &tmp);
}


// ----------------------------------------------------------------------------
// config sections index

int appconfig_section_compare(void *a, void *b) {
    if(((struct section *)a)->hash < ((struct section *)b)->hash) return -1;
    else if(((struct section *)a)->hash > ((struct section *)b)->hash) return 1;
    else return strcmp(((struct section *)a)->name, ((struct section *)b)->name);
}

#define appconfig_index_add(root, cfg) (struct section *)avl_insert_lock(&(root)->index, (avl_t *)(cfg))
#define appconfig_index_del(root, cfg) (struct section *)avl_remove_lock(&(root)->index, (avl_t *)(cfg))

static struct section *appconfig_index_find(struct config *root, const char *name, uint32_t hash) {
    struct section tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = (char *)name;

    return (struct section *)avl_search_lock(&root->index, (avl_t *) &tmp);
}


// ----------------------------------------------------------------------------
// config section methods

static inline struct section *appconfig_section_find(struct config *root, const char *section) {
    return appconfig_index_find(root, section, 0);
}

static inline struct section *appconfig_section_create(struct config *root, const char *section) {
    debug(D_CONFIG, "Creating section '%s'.", section);

    struct section *co = callocz(1, sizeof(struct section));
    co->name = strdupz(section);
    co->hash = simple_hash(co->name);
    netdata_mutex_init(&co->mutex);

    avl_init_lock(&co->values_index, appconfig_option_compare);

    if(unlikely(appconfig_index_add(root, co) != co))
        error("INTERNAL ERROR: indexing of section '%s', already exists.", co->name);

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

    debug(D_CONFIG, "Destroying section '%s'.", section);

    co = appconfig_section_find(root, section);
    if(!co) {
        error("Could not destroy section '%s'. Not found.", section);
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
            error("Cannot remove config option '%s' from section '%s'.", cv->name, co->name);
        freez(cv->value);
        freez(cv->name);
        freez(cv);
    }
    co->values = NULL;
    config_section_unlock(co);

    if (unlikely(!appconfig_index_del(root, co))) {
        error("Cannot remove section '%s' from config.", section);
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
    freez(co->name);
    pthread_mutex_destroy(&co->mutex);
    freez(co);
}

void appconfig_section_option_destroy_non_loaded(struct config *root, const char *section, const char *name)
{
    debug(D_CONFIG, "Destroying section option '%s -> %s'.", section, name);

    struct section *co;
    co = appconfig_section_find(root, section);
    if (!co) {
        error("Could not destroy section option '%s -> %s'. The section not found.", section, name);
        return;
    }

    config_section_wrlock(co);

    struct config_option *cv;

    cv = appconfig_option_index_find(co, name, simple_hash(name));

    if (cv && cv->flags & CONFIG_VALUE_LOADED) {
        config_section_unlock(co);
        return;
    }

    if (unlikely(!(cv && appconfig_option_index_del(co, cv)))) {
        config_section_unlock(co);
        error("Could not destroy section option '%s -> %s'. The option not found.", section, name);
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

    freez(cv->value);
    freez(cv->name);
    freez(cv);

    config_section_unlock(co);
    return;
}

// ----------------------------------------------------------------------------
// config name-value methods

static inline struct config_option *appconfig_value_create(struct section *co, const char *name, const char *value) {
    debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

    struct config_option *cv = callocz(1, sizeof(struct config_option));
    cv->name = strdupz(name);
    cv->hash = simple_hash(cv->name);
    cv->value = strdupz(value);

    struct config_option *found = appconfig_option_index_add(co, cv);
    if(found != cv) {
        error("indexing of config '%s' in section '%s': already exists - using the existing one.", cv->name, co->name);
        freez(cv->value);
        freez(cv->name);
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

    debug(D_CONFIG, "request to get config in section '%s', name '%s'", section, name);

    struct section *co = appconfig_section_find(root, section);
    if(!co) return 0;

    cv = appconfig_option_index_find(co, name, 0);
    if(!cv) return 0;

    return 1;
}

int appconfig_move(struct config *root, const char *section_old, const char *name_old, const char *section_new, const char *name_new) {
    struct config_option *cv_old, *cv_new;
    int ret = -1;

    debug(D_CONFIG, "request to rename config in section '%s', old name '%s', to section '%s', new name '%s'", section_old, name_old, section_new, name_new);

    struct section *co_old = appconfig_section_find(root, section_old);
    if(!co_old) return ret;

    struct section *co_new = appconfig_section_find(root, section_new);
    if(!co_new) co_new = appconfig_section_create(root, section_new);

    config_section_wrlock(co_old);
    if(co_old != co_new)
        config_section_wrlock(co_new);

    cv_old = appconfig_option_index_find(co_old, name_old, 0);
    if(!cv_old) goto cleanup;

    cv_new = appconfig_option_index_find(co_new, name_new, 0);
    if(cv_new) goto cleanup;

    if(unlikely(appconfig_option_index_del(co_old, cv_old) != cv_old))
        error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted the wrong config entry.", cv_old->name, co_old->name);

    if(co_old->values == cv_old) {
        co_old->values = cv_old->next;
    }
    else {
        struct config_option *t;
        for(t = co_old->values; t && t->next != cv_old ;t = t->next) ;
        if(!t || t->next != cv_old)
            error("INTERNAL ERROR: cannot find variable '%s' in section '%s' of the config - but it should be there.", cv_old->name, co_old->name);
        else
            t->next = cv_old->next;
    }

    freez(cv_old->name);
    cv_old->name = strdupz(name_new);
    cv_old->hash = simple_hash(cv_old->name);

    cv_new = cv_old;
    cv_new->next = co_new->values;
    co_new->values = cv_new;

    if(unlikely(appconfig_option_index_add(co_new, cv_old) != cv_old))
        error("INTERNAL ERROR: re-indexing of config '%s' in section '%s', already exists.", cv_old->name, co_new->name);

    ret = 0;

cleanup:
    if(co_old != co_new)
        config_section_unlock(co_new);
    config_section_unlock(co_old);
    return ret;
}

char *appconfig_get_by_section(struct section *co, const char *name, const char *default_value)
{
    struct config_option *cv;

    // Only calls internal to this file check for a NULL result and they do not supply a NULL arg.
    // External caller should treat NULL as an error case.
    cv = appconfig_option_index_find(co, name, 0);
    if (!cv) {
        if (!default_value) return NULL;
        cv = appconfig_value_create(co, name, default_value);
        if (!cv) return NULL;
    }
    cv->flags |= CONFIG_VALUE_USED;

    if((cv->flags & CONFIG_VALUE_LOADED) || (cv->flags & CONFIG_VALUE_CHANGED)) {
        // this is a loaded value from the config file
        // if it is different than the default, mark it
        if(!(cv->flags & CONFIG_VALUE_CHECKED)) {
            if(default_value && strcmp(cv->value, default_value) != 0) cv->flags |= CONFIG_VALUE_CHANGED;
            cv->flags |= CONFIG_VALUE_CHECKED;
        }
    }

    return(cv->value);
}


char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value)
{
    if (default_value == NULL)
        debug(D_CONFIG, "request to get config in section '%s', name '%s' or fail", section, name);
    else
        debug(D_CONFIG, "request to get config in section '%s', name '%s', default_value '%s'", section, name, default_value);

    struct section *co = appconfig_section_find(root, section);
    if (!co && !default_value)
        return NULL;
    if(!co) co = appconfig_section_create(root, section);

    return appconfig_get_by_section(co, name, default_value);
}

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value)
{
    char buffer[100], *s;
    sprintf(buffer, "%lld", value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return strtoll(s, NULL, 0);
}

NETDATA_DOUBLE appconfig_get_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value)
{
    char buffer[100], *s;
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return str2ndd(s, NULL);
}

inline int appconfig_test_boolean_value(char *s) {
    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on")
       || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand"))
        return 1;

    return 0;
}

int appconfig_get_boolean_by_section(struct section *co, const char *name, int value) {
    char *s;

    s = appconfig_get_by_section(co, name, (!value)?"no":"yes");
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value)
{
    char *s;
    if(value) s = "yes";
    else s = "no";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value)
{
    char *s;

    if(value == CONFIG_BOOLEAN_AUTO)
        s = "auto";

    else if(value == CONFIG_BOOLEAN_NO)
        s = "no";

    else
        s = "yes";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    if(!strcmp(s, "yes"))
        return CONFIG_BOOLEAN_YES;
    else if(!strcmp(s, "no"))
        return CONFIG_BOOLEAN_NO;
    else if(!strcmp(s, "auto") || !strcmp(s, "on demand"))
        return CONFIG_BOOLEAN_AUTO;

    return value;
}

const char *appconfig_set_default(struct config *root, const char *section, const char *name, const char *value)
{
    struct config_option *cv;

    debug(D_CONFIG, "request to set default config in section '%s', name '%s', value '%s'", section, name, value);

    struct section *co = appconfig_section_find(root, section);
    if(!co) return appconfig_set(root, section, name, value);

    cv = appconfig_option_index_find(co, name, 0);
    if(!cv) return appconfig_set(root, section, name, value);

    cv->flags |= CONFIG_VALUE_USED;

    if(cv->flags & CONFIG_VALUE_LOADED)
        return cv->value;

    if(strcmp(cv->value, value) != 0) {
        cv->flags |= CONFIG_VALUE_CHANGED;

        freez(cv->value);
        cv->value = strdupz(value);
    }

    return cv->value;
}

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value)
{
    struct config_option *cv;

    debug(D_CONFIG, "request to set config in section '%s', name '%s', value '%s'", section, name, value);

    struct section *co = appconfig_section_find(root, section);
    if(!co) co = appconfig_section_create(root, section);

    cv = appconfig_option_index_find(co, name, 0);
    if(!cv) cv = appconfig_value_create(co, name, value);
    cv->flags |= CONFIG_VALUE_USED;

    if(strcmp(cv->value, value) != 0) {
        cv->flags |= CONFIG_VALUE_CHANGED;

        freez(cv->value);
        cv->value = strdupz(value);
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

int appconfig_get_duration(struct config *root, const char *section, const char *name, const char *value)
{
    int result = 0;
    const char *s;

    s = appconfig_get(root, section, name, value);
    if(!s) goto fallback;

    if(!config_parse_duration(s, &result)) {
        error("config option '[%s].%s = %s' is configured with an valid duration", section, name, s);
        goto fallback;
    }

    return result;

    fallback:
    if(!config_parse_duration(value, &result))
        error("INTERNAL ERROR: default duration supplied for option '[%s].%s = %s' is not a valid duration", section, name, value);

    return result;
}

// ----------------------------------------------------------------------------
// config load/save

int appconfig_load(struct config *root, char *filename, int overwrite_used, const char *section_name)
{
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

    debug(D_CONFIG, "CONFIG: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        // info("CONFIG: cannot open file '%s'. Using internal defaults.", filename);
        return 0;
    }

    uint32_t section_hash = 0;
    if(section_name) {
        section_hash = simple_hash(section_name);
    }
    is_exporter_config = (strstr(filename, EXPORTING_CONF) != NULL);

    while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if(!s || *s == '#') {
            debug(D_CONFIG, "CONFIG: ignoring line %d of file '%s', it is empty.", line, filename);
            continue;
        }

        int len = (int) strlen(s);
        if(*s == '[' && s[len - 1] == ']') {
            // new section
            s[len - 1] = '\0';
            s++;

            if (is_exporter_config) {
                global_exporting_section =
                    !(strcmp(s, CONFIG_SECTION_EXPORTING)) || !(strcmp(s, CONFIG_SECTION_PROMETHEUS));
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
                            error("Instance (%s) already exists", working_instance);
                            co = NULL;
                            continue;
                        }
                    } else {
                        co = NULL;
                        error("Section (%s) does not specify a valid connector", s);
                        continue;
                    }
                }
            }

            co = appconfig_section_find(root, s);
            if(!co) co = appconfig_section_create(root, s);

            if(co && section_name && overwrite_used && section_hash == co->hash && !strcmp(section_name, co->name)) {
                config_section_wrlock(co);
                struct config_option *cv2 = co->values;
                while (cv2) {
                    struct config_option *save = cv2->next;
                    struct config_option *found = appconfig_option_index_del(co, cv2);
                    if(found != cv2)
                        error("INTERNAL ERROR: Cannot remove '%s' from  section '%s', it was not inserted before.",
                               cv2->name, co->name);

                    freez(cv2->name);
                    freez(cv2->value);
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
            error("CONFIG: ignoring line %d ('%s') of file '%s', it is outside all sections.", line, s, filename);
            continue;
        }

        if(section_name && overwrite_used && section_hash != co->hash && strcmp(section_name, co->name)) {
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(!value) {
            error("CONFIG: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(!name || *name == '#') {
            error("CONFIG: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value) value = "";

        struct config_option *cv = appconfig_option_index_find(co, name, 0);

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
        } else {
            if (((cv->flags & CONFIG_VALUE_USED) && overwrite_used) || !(cv->flags & CONFIG_VALUE_USED)) {
                debug(
                    D_CONFIG, "CONFIG: line %d of file '%s', overwriting '%s/%s'.", line, filename, co->name, cv->name);
                freez(cv->value);
                cv->value = strdupz(value);
            } else
                debug(
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

void appconfig_generate(struct config *root, BUFFER *wb, int only_changed)
{
    int i, pri;
    struct section *co;
    struct config_option *cv;

    {
        int found_host_labels = 0;
        for (co = root->first_section; co; co = co->next)
            if(!strcmp(co->name, CONFIG_SECTION_HOST_LABEL))
                found_host_labels = 1;

        if(!found_host_labels) {
            appconfig_section_create(root, CONFIG_SECTION_HOST_LABEL);
            appconfig_get(root, CONFIG_SECTION_HOST_LABEL, "name", "value");
        }
    }

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

    for(i = 0; i <= 17 ;i++) {
        appconfig_wrlock(root);
        for(co = root->first_section; co ; co = co->next) {
            if(!strcmp(co->name, CONFIG_SECTION_GLOBAL))                 pri = 0;
            else if(!strcmp(co->name, CONFIG_SECTION_DB))                pri = 1;
            else if(!strcmp(co->name, CONFIG_SECTION_DIRECTORIES))       pri = 2;
            else if(!strcmp(co->name, CONFIG_SECTION_LOGS))              pri = 3;
            else if(!strcmp(co->name, CONFIG_SECTION_ENV_VARS))          pri = 4;
            else if(!strcmp(co->name, CONFIG_SECTION_HOST_LABEL))        pri = 5;
            else if(!strcmp(co->name, CONFIG_SECTION_SQLITE))            pri = 6;
            else if(!strcmp(co->name, CONFIG_SECTION_CLOUD))             pri = 7;
            else if(!strcmp(co->name, CONFIG_SECTION_ML))                pri = 8;
            else if(!strcmp(co->name, CONFIG_SECTION_HEALTH))            pri = 9;
            else if(!strcmp(co->name, CONFIG_SECTION_WEB))               pri = 10;
            else if(!strcmp(co->name, CONFIG_SECTION_WEBRTC))            pri = 11;
            // by default, new sections will get pri = 12 (set at the end, below)
            else if(!strcmp(co->name, CONFIG_SECTION_REGISTRY))          pri = 13;
            else if(!strcmp(co->name, CONFIG_SECTION_GLOBAL_STATISTICS)) pri = 14;
            else if(!strcmp(co->name, CONFIG_SECTION_PLUGINS))           pri = 15;
            else if(!strcmp(co->name, CONFIG_SECTION_STATSD))            pri = 16;
            else if(!strncmp(co->name, "plugin:", 7))                    pri = 17; // << change the loop too if you change this
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

                if(!used) {
                    buffer_sprintf(wb, "\n# section '%s' is not used.", co->name);
                }

                buffer_sprintf(wb, "\n[%s]\n", co->name);

                config_section_wrlock(co);
                for(cv = co->values; cv ; cv = cv->next) {

                    if(used && !(cv->flags & CONFIG_VALUE_USED)) {
                        buffer_sprintf(wb, "\n\t# option '%s' is not used.\n", cv->name);
                    }
                    buffer_sprintf(wb, "\t%s%s = %s\n", ((!(cv->flags & CONFIG_VALUE_LOADED)) && (!(cv->flags & CONFIG_VALUE_CHANGED)) && (cv->flags & CONFIG_VALUE_USED))?"# ":"", cv->name, cv->value);
                }
                config_section_unlock(co);
            }
        }
        appconfig_unlock(root);
    }
}

/**
 * Parse Duration
 *
 * Parse the string setting the result
 *
 * @param string  the timestamp string
 * @param result the output variable
 *
 * @return It returns 1 on success and 0 otherwise
 */
int config_parse_duration(const char* string, int* result) {
    while(*string && isspace(*string)) string++;

    if(unlikely(!*string)) goto fallback;

    if(*string == 'n' && !strcmp(string, "never")) {
        // this is a valid option
        *result = 0;
        return 1;
    }

    // make sure it is a number
    if(!(isdigit(*string) || *string == '+' || *string == '-')) goto fallback;

    char *e = NULL;
    NETDATA_DOUBLE n = str2ndd(string, &e);
    if(e && *e) {
        switch (*e) {
            case 'Y':
                *result = (int) (n * 31536000);
                break;
            case 'M':
                *result = (int) (n * 2592000);
                break;
            case 'w':
                *result = (int) (n * 604800);
                break;
            case 'd':
                *result = (int) (n * 86400);
                break;
            case 'h':
                *result = (int) (n * 3600);
                break;
            case 'm':
                *result = (int) (n * 60);
                break;
            case 's':
            default:
                *result = (int) (n);
                break;
        }
    }
    else
        *result = (int)(n);

    return 1;

    fallback:
    *result = 0;
    return 0;
}

struct section *appconfig_get_section(struct config *root, const char *name)
{
    return appconfig_section_find(root, name);
}
