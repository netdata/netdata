// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

ENUM_STR_MAP_DEFINE(CONFIG_VALUE_TYPES) = {
    { .id = CONFIG_VALUE_TYPE_UNKNOWN, .name ="unknown", },
    { .id = CONFIG_VALUE_TYPE_TEXT, .name ="text", },
    { .id = CONFIG_VALUE_TYPE_HOSTNAME, .name ="hostname", },
    { .id = CONFIG_VALUE_TYPE_USERNAME, .name ="username", },
    { .id = CONFIG_VALUE_TYPE_FILENAME, .name ="filename", },
    { .id = CONFIG_VALUE_TYPE_PATH, .name ="path", },
    { .id = CONFIG_VALUE_TYPE_SIMPLE_PATTERN, .name ="simple pattern", },
    { .id = CONFIG_VALUE_TYPE_URL, .name ="URL", },
    { .id = CONFIG_VALUE_TYPE_ENUM, .name ="one of keywords", },
    { .id = CONFIG_VALUE_TYPE_BITMAP, .name ="any of keywords", },
    { .id = CONFIG_VALUE_TYPE_INTEGER, .name ="number (integer)", },
    { .id = CONFIG_VALUE_TYPE_DOUBLE, .name ="number (double)", },
    { .id = CONFIG_VALUE_TYPE_BOOLEAN, .name ="yes or no", },
    { .id = CONFIG_VALUE_TYPE_BOOLEAN_ONDEMAND, .name ="yes, no, or auto", },
    { .id = CONFIG_VALUE_TYPE_DURATION_IN_SECS, .name ="duration (seconds)", },
    { .id = CONFIG_VALUE_TYPE_DURATION_IN_MS, .name ="duration (ms)", },
    { .id = CONFIG_VALUE_TYPE_DURATION_IN_DAYS_TO_SECONDS, .name ="duration (days)", },
    { .id = CONFIG_VALUE_TYPE_SIZE_IN_BYTES, .name ="size (bytes)", },
    { .id = CONFIG_VALUE_TYPE_SIZE_IN_MB, .name ="size (MiB)", },
};

ENUM_STR_DEFINE_FUNCTIONS(CONFIG_VALUE_TYPES, CONFIG_VALUE_TYPE_UNKNOWN, "unknown");


// ----------------------------------------------------------------------------
// config load/save

int inicfg_load(struct config *root, char *filename, int overwrite_used, const char *section_name) {
    int line = 0;
    struct config_section *sect = NULL;
    int is_exporter_config = 0;
    int _connectors = 0;              // number of exporting connector sections we have
    char working_instance[CONFIG_MAX_NAME + 1];
    char working_connector[CONFIG_MAX_NAME + 1];
    struct config_section *working_connector_section = NULL;
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
                        if (unlikely(inicfg_section_find(root, working_instance))) {
                            netdata_log_error("Instance (%s) already exists", working_instance);
                            sect = NULL;
                            continue;
                        }
                    }
                    else {
                        sect = NULL;
                        netdata_log_error("Section (%s) does not specify a valid connector", s);
                        continue;
                    }
                }
            }

            sect = inicfg_section_find(root, s);
            if(!sect)
                sect = inicfg_section_create(root, s);

            if(sect && section_string && overwrite_used && section_string == sect->name) {
                SECTION_LOCK(sect);

                while(sect->values)
                    inicfg_option_remove_and_delete(sect, sect->values, true);

                SECTION_UNLOCK(sect);
            }

            continue;
        }

        if(!sect) {
            // line outside a section
            netdata_log_error("CONFIG: ignoring line %d ('%s') of file '%s', it is outside all sections.", line, s, filename);
            continue;
        }

        if(section_string && overwrite_used && section_string != sect->name)
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

        struct config_option *opt = inicfg_option_find(sect, name);

        if (!opt) {
            opt = inicfg_option_create(sect, name, value);
            if (likely(is_exporter_config) && unlikely(!global_exporting_section)) {
                if (unlikely(!working_connector_section)) {
                    working_connector_section = inicfg_section_find(root, working_connector);
                    if (!working_connector_section)
                        working_connector_section = inicfg_section_create(root, working_connector);
                    if (likely(working_connector_section)) {
                        add_connector_instance(working_connector_section, sect);
                    }
                }
            }
        }
        else {
            if (((opt->flags & CONFIG_VALUE_USED) && overwrite_used) || !(opt->flags & CONFIG_VALUE_USED)) {
                string_freez(opt->value);
                opt->value = string_strdupz(value);
            }
        }
        opt->flags |= CONFIG_VALUE_LOADED;
    }

    fclose(fp);

    return 1;
}

void inicfg_generate(struct config *root, BUFFER *wb, int only_changed, bool netdata_conf)
{
    int i, pri;
    struct config_section *sect;
    struct config_option *opt;

    {
        int found_host_labels = 0;
        for (sect = root->sections; sect; sect = sect->next)
            if(!string_strcmp(sect->name, CONFIG_SECTION_HOST_LABEL))
                found_host_labels = 1;

        if(netdata_conf && !found_host_labels) {
            inicfg_section_create(root, CONFIG_SECTION_HOST_LABEL);
            inicfg_get_raw_value(root, CONFIG_SECTION_HOST_LABEL, "name", "value", CONFIG_VALUE_TYPE_TEXT, NULL);
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
        APPCONFIG_LOCK(root);
        for(sect = root->sections; sect; sect = sect->next) {
            if(!string_strcmp(sect->name, CONFIG_SECTION_GLOBAL))                 pri = 0;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_DB))                pri = 1;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_DIRECTORIES))       pri = 2;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_LOGS))              pri = 3;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_ENV_VARS))          pri = 4;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_HOST_LABEL))        pri = 5;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_SQLITE))            pri = 6;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_CLOUD))             pri = 7;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_ML))                pri = 8;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_HEALTH))            pri = 9;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_WEB))               pri = 10;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_WEBRTC))            pri = 11;
            // by default, new sections will get pri = 12 (set at the end, below)
            else if(!string_strcmp(sect->name, CONFIG_SECTION_REGISTRY))          pri = 13;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_PULSE)) pri = 14;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_PLUGINS))           pri = 15;
            else if(!string_strcmp(sect->name, CONFIG_SECTION_STATSD))            pri = 16;
            else if(!string_strncmp(sect->name, "plugin:", 7))                    pri = 17; // << change the loop too if you change this
            else pri = 12; // this is used for any new (currently unknown) sections

            if(i == pri) {
                int loaded = 0;
                int used = 0;
                int changed = 0;
                int count = 0;

                SECTION_LOCK(sect);
                for(opt = sect->values; opt; opt = opt->next) {
                    used += (opt->flags & CONFIG_VALUE_USED)?1:0;
                    loaded += (opt->flags & CONFIG_VALUE_LOADED)?1:0;
                    changed += (opt->flags & CONFIG_VALUE_CHANGED)?1:0;
                    count++;
                }
                SECTION_UNLOCK(sect);

                if(!count) continue;
                if(only_changed && !changed && !loaded) continue;

                if(!used)
                    buffer_sprintf(wb, "\n# section '%s' is not used.", string2str(sect->name));

                buffer_sprintf(wb, "\n[%s]\n", string2str(sect->name));

                size_t options_added = 0;
                bool last_had_comments = false;
                SECTION_LOCK(sect);
                for(opt = sect->values; opt; opt = opt->next) {
                    bool unused = used && !(opt->flags & CONFIG_VALUE_USED);
                    bool migrated = used && (opt->flags & CONFIG_VALUE_MIGRATED);
                    bool reformatted = used && (opt->flags & CONFIG_VALUE_REFORMATTED);
                    bool show_default = used && (opt->flags & (CONFIG_VALUE_LOADED|CONFIG_VALUE_CHANGED) && opt->value_default);

                    if((unused || migrated || reformatted || show_default)) {
                        if(options_added)
                            buffer_strcat(wb, "\n");

                        buffer_sprintf(wb, "\t#| >>> [%s].%s <<<\n",
                                       string2str(sect->name), string2str(opt->name));

                        last_had_comments = true;
                    }
                    else if(last_had_comments) {
                        buffer_strcat(wb, "\n");
                        last_had_comments = false;
                    }

                    if(unused)
                        buffer_sprintf(wb, "\t#| found in the config file, but is not used\n");

                    if(migrated && reformatted)
                        buffer_sprintf(wb, "\t#| migrated from: [%s].%s = %s\n",
                                       string2str(opt->migrated.section), string2str(opt->migrated.name),
                                       string2str(opt->value_original));
                    else {
                        if (migrated)
                            buffer_sprintf(wb, "\t#| migrated from: [%s].%s\n",
                                           string2str(opt->migrated.section), string2str(opt->migrated.name));

                        if (reformatted)
                            buffer_sprintf(wb, "\t#| reformatted from: %s\n",
                                           string2str(opt->value_original));
                    }

                    if(show_default)
                        buffer_sprintf(wb, "\t#| datatype: %s, default value: %s\n",
                                       CONFIG_VALUE_TYPES_2str(opt->type),
                                       string2str(opt->value_default));

                    buffer_sprintf(wb, "\t%s%s = %s\n",
                                   (
                                       !(opt->flags & CONFIG_VALUE_LOADED) &&
                                       !(opt->flags & CONFIG_VALUE_CHANGED) &&
                                       (opt->flags & CONFIG_VALUE_USED)
                                           ) ? "# " : "",
                                   string2str(opt->name),
                                   string2str(opt->value));

                    options_added++;
                }
                SECTION_UNLOCK(sect);
            }
        }
        APPCONFIG_UNLOCK(root);
    }
}
