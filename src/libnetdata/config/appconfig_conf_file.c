// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

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
                    struct config_option *found = appconfig_option_del(co, cv2);
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

        struct config_option *cv = appconfig_option_find(co, name);

        if (!cv) {
            cv = appconfig_option_create(co, name, value);
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
