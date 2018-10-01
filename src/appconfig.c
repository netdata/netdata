// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

// ----------------------------------------------------------------------------
// definitions

#define CONFIG_VALUE_LOADED  0x01 // has been loaded from the config
#define CONFIG_VALUE_USED    0x02 // has been accessed from the program
#define CONFIG_VALUE_CHANGED 0x04 // has been changed from the loaded value
#define CONFIG_VALUE_CHECKED 0x08 // has been checked if the value is different from the default

struct config_option {
    avl avl;                // the index - this has to be first!

    uint8_t flags;
    uint32_t hash;          // a simple hash to speed up searching
                            // we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;
    char *value;

    struct config_option *next; // config->mutex protects just this
};

struct section {
    avl avl;

    uint32_t hash;          // a simple hash to speed up searching
                            // we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;

    struct section *next;    // gloabl config_mutex protects just this

    struct config_option *values;
    avl_tree_lock values_index;

    netdata_mutex_t mutex;  // this locks only the writers, to ensure atomic updates
                            // readers are protected using the rwlock in avl_tree_lock
};

static int appconfig_section_compare(void *a, void *b);

struct config netdata_config = {
        .sections = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
            { NULL, appconfig_section_compare },
            AVL_LOCK_INITIALIZER
        }
};

struct config stream_config = {
        .sections = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                { NULL, appconfig_section_compare },
                AVL_LOCK_INITIALIZER
        }
};

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
    if(((struct config_option *)a)->hash < ((struct config_option *)b)->hash) return -1;
    else if(((struct config_option *)a)->hash > ((struct config_option *)b)->hash) return 1;
    else return strcmp(((struct config_option *)a)->name, ((struct config_option *)b)->name);
}

#define appconfig_option_index_add(co, cv) (struct config_option *)avl_insert_lock(&((co)->values_index), (avl *)(cv))
#define appconfig_option_index_del(co, cv) (struct config_option *)avl_remove_lock(&((co)->values_index), (avl *)(cv))

static struct config_option *appconfig_option_index_find(struct section *co, const char *name, uint32_t hash) {
    struct config_option tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = (char *)name;

    return (struct config_option *)avl_search_lock(&(co->values_index), (avl *) &tmp);
}


// ----------------------------------------------------------------------------
// config sections index

static int appconfig_section_compare(void *a, void *b) {
    if(((struct section *)a)->hash < ((struct section *)b)->hash) return -1;
    else if(((struct section *)a)->hash > ((struct section *)b)->hash) return 1;
    else return strcmp(((struct section *)a)->name, ((struct section *)b)->name);
}

#define appconfig_index_add(root, cfg) (struct section *)avl_insert_lock(&(root)->index, (avl *)(cfg))
#define appconfig_index_del(root, cfg) (struct section *)avl_remove_lock(&(root)->index, (avl *)(cfg))

static struct section *appconfig_index_find(struct config *root, const char *name, uint32_t hash) {
    struct section tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = (char *)name;

    return (struct section *)avl_search_lock(&root->index, (avl *) &tmp);
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
    struct section *co2 = root->sections;
    if(co2) {
        while (co2->next) co2 = co2->next;
        co2->next = co;
    }
    else root->sections = co;
    appconfig_unlock(root);

    return co;
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
        error("INTERNAL ERROR: deletion of config '%s' from section '%s', deleted tge wrong config entry.", cv_old->name, co_old->name);

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

char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value)
{
    struct config_option *cv;

    debug(D_CONFIG, "request to get config in section '%s', name '%s', default_value '%s'", section, name, default_value);

    struct section *co = appconfig_section_find(root, section);
    if(!co) co = appconfig_section_create(root, section);

    cv = appconfig_option_index_find(co, name, 0);
    if(!cv) {
        cv = appconfig_value_create(co, name, default_value);
        if(!cv) return NULL;
    }
    cv->flags |= CONFIG_VALUE_USED;

    if((cv->flags & CONFIG_VALUE_LOADED) || (cv->flags & CONFIG_VALUE_CHANGED)) {
        // this is a loaded value from the config file
        // if it is different that the default, mark it
        if(!(cv->flags & CONFIG_VALUE_CHECKED)) {
            if(strcmp(cv->value, default_value) != 0) cv->flags |= CONFIG_VALUE_CHANGED;
            cv->flags |= CONFIG_VALUE_CHECKED;
        }
    }

    return(cv->value);
}

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value)
{
    char buffer[100], *s;
    sprintf(buffer, "%lld", value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return strtoll(s, NULL, 0);
}

LONG_DOUBLE appconfig_get_float(struct config *root, const char *section, const char *name, LONG_DOUBLE value)
{
    char buffer[100], *s;
    sprintf(buffer, "%0.5" LONG_DOUBLE_MODIFIER, value);

    s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return str2ld(s, NULL);
}

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value)
{
    char *s;
    if(value) s = "yes";
    else s = "no";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on") || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand")) return 1;
    return 0;
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

LONG_DOUBLE appconfig_set_float(struct config *root, const char *section, const char *name, LONG_DOUBLE value)
{
    char buffer[100];
    sprintf(buffer, "%0.5" LONG_DOUBLE_MODIFIER, value);

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


// ----------------------------------------------------------------------------
// config load/save

int appconfig_load(struct config *root, char *filename, int overwrite_used)
{
    int line = 0;
    struct section *co = NULL;

    char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

    if(!filename) filename = CONFIG_DIR "/" CONFIG_FILENAME;

    debug(D_CONFIG, "CONFIG: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        // info("CONFIG: cannot open file '%s'. Using internal defaults.", filename);
        return 0;
    }

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

            co = appconfig_section_find(root, s);
            if(!co) co = appconfig_section_create(root, s);

            continue;
        }

        if(!co) {
            // line outside a section
            error("CONFIG: ignoring line %d ('%s') of file '%s', it is outside all sections.", line, s, filename);
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

        if(!cv) cv = appconfig_value_create(co, name, value);
        else {
            if(((cv->flags & CONFIG_VALUE_USED) && overwrite_used) || !(cv->flags & CONFIG_VALUE_USED)) {
                debug(D_CONFIG, "CONFIG: line %d of file '%s', overwriting '%s/%s'.", line, filename, co->name, cv->name);
                freez(cv->value);
                cv->value = strdupz(value);
            }
            else
                debug(D_CONFIG, "CONFIG: ignoring line %d of file '%s', '%s/%s' is already present and used.", line, filename, co->name, cv->name);
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

    for(i = 0; i < 3 ;i++) {
        switch(i) {
            case 0:
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
                break;

            case 1:
                buffer_strcat(wb, "\n\n# per plugin configuration\n");
                break;

            case 2:
                buffer_strcat(wb, "\n\n# per chart configuration\n");
                break;
        }

        appconfig_wrlock(root);
        for(co = root->sections; co ; co = co->next) {
            if(!strcmp(co->name, CONFIG_SECTION_GLOBAL)
               || !strcmp(co->name, CONFIG_SECTION_WEB)
               || !strcmp(co->name, CONFIG_SECTION_STATSD)
               || !strcmp(co->name, CONFIG_SECTION_PLUGINS)
               || !strcmp(co->name, CONFIG_SECTION_REGISTRY)
               || !strcmp(co->name, CONFIG_SECTION_HEALTH)
               || !strcmp(co->name, CONFIG_SECTION_BACKEND)
               || !strcmp(co->name, CONFIG_SECTION_STREAM)
                    )
                pri = 0;
            else if(!strncmp(co->name, "plugin:", 7)) pri = 1;
            else pri = 2;

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
