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

#if defined(OS_WINDOWS)
static bool inicfg_windows_is_path_list_env_var(const struct config_section *sect, const struct config_option *opt) {
    return !string_strcmp(sect->name, CONFIG_SECTION_ENV_VARS) &&
           (!string_strcmp(opt->name, "PATH") || !string_strcmp(opt->name, "PYTHONPATH"));
}

static bool inicfg_windows_is_quoted_path_list_dir_var(const struct config_section *sect, const struct config_option *opt) {
    return !string_strcmp(sect->name, CONFIG_SECTION_DIRECTORIES) &&
           !string_strcmp(opt->name, "plugins");
}

static bool inicfg_windows_is_log_path_setting(const struct config_section *sect, const struct config_option *opt) {
    return ((!string_strcmp(sect->name, CONFIG_SECTION_LOGS) &&
             (!string_strcmp(opt->name, "debug") ||
              !string_strcmp(opt->name, "daemon") ||
              !string_strcmp(opt->name, "collector") ||
              !string_strcmp(opt->name, "access") ||
              !string_strcmp(opt->name, "health"))) ||
            (!string_strcmp(sect->name, CONFIG_SECTION_CLOUD) &&
             !string_strcmp(opt->name, "conversation log file")));
}

static const char *inicfg_windows_path_list_for_display(const char *value, char *dst, size_t dst_size) {
    if (!value || !*value || !dst || dst_size == 0)
        return value ? value : "";

    // Internal storage always uses ':' as separator (normalized on read).
    // A ';' here means the value was never normalized, so treat it as already Windows-format.
    if (strchr(value, ';')) {
        snprintfz(dst, dst_size, "%s", value);
        return dst;
    }

    dst[0] = '\0';
    size_t len = 0;
    size_t value_len = strnlen(value, CONFIG_MAX_VALUE);
    const char *value_end = value + value_len;
    bool first = true;

    const char *segment_start = value;
    while (segment_start < value_end) {
        const char *separator = memchr(segment_start, ':', (size_t)(value_end - segment_start));
        size_t segment_len = separator
            ? (size_t)(separator - segment_start)
            : value_len - (size_t)(segment_start - value);

        char segment[CONFIG_MAX_VALUE + 1];
        if (segment_len > CONFIG_MAX_VALUE)
            segment_len = CONFIG_MAX_VALUE;

        memcpy(segment, segment_start, segment_len);
        segment[segment_len] = '\0';

        CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(segment);
        if (!first)
            len = strcatz(dst, len, ";", dst_size);
        len = strcatz(dst, len, translated, dst_size);
        first = false;

        if (!separator)
            break;

        segment_start = separator + 1;
    }

    return dst;
}

static const char *inicfg_windows_quoted_path_list_for_display(const char *value, char *dst, size_t dst_size) {
    if (!value || !*value || !dst || dst_size == 0)
        return value ? value : "";

    CLEAN_CHAR_P *copy = strdupz(value);
    // 256 slots matches the limit in reformat_quoted_path_list; entries beyond this are silently ignored.
    char *words[256] = { 0 };
    size_t num_words = quoted_strings_splitter_config(copy, words, _countof(words));
    if (!num_words) {
        snprintfz(dst, dst_size, "%s", value);
        return dst;
    }

    dst[0] = '\0';
    size_t len = 0;
    for(size_t i = 0; i < num_words; i++) {
        CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(words[i]);
        if (i)
            len = strcatz(dst, len, " ", dst_size);
        len = strcatz(dst, len, "\"", dst_size);
        len = strcatz(dst, len, translated, dst_size);
        len = strcatz(dst, len, "\"", dst_size);
    }

    return dst;
}

static const char *inicfg_windows_value_for_display(
    const struct config_section *sect,
    const struct config_option *opt,
    const char *value,
    char *dst,
    size_t dst_size)
{
    if (!value || !*value)
        return value ? value : "";

    if (inicfg_windows_is_path_list_env_var(sect, opt))
        return inicfg_windows_path_list_for_display(value, dst, dst_size);

    if (inicfg_windows_is_quoted_path_list_dir_var(sect, opt))
        return inicfg_windows_quoted_path_list_for_display(value, dst, dst_size);

    if (inicfg_windows_is_log_path_setting(sect, opt))
        return inicfg_log_path_setting_for_display(value, dst, dst_size);

    // Translate all PATH/FILENAME-typed options, plus all remaining DIRECTORIES keys.
    // The list-type keys handled above are ENV_VARS.PATH/PYTHONPATH and DIRECTORIES.plugins.
    // If a new list-type key is added to either section, add a dedicated check before this fallback.
    if ((opt->type != CONFIG_VALUE_TYPE_PATH && opt->type != CONFIG_VALUE_TYPE_FILENAME) &&
        string_strcmp(sect->name, CONFIG_SECTION_DIRECTORIES) != 0)
        return value;

    return os_translate_path(dst, value, dst_size);
}
#endif


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
        if(!s || !*s || *s == '#') {
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

                    const char *current_value = string2str(opt->value);
                    const char *default_value = opt->value_default ? string2str(opt->value_default) : "";
#if defined(OS_WINDOWS)
                    char current_value_windows[CONFIG_MAX_VALUE + 1];
                    char default_value_windows[CONFIG_MAX_VALUE + 1];

                    current_value = inicfg_windows_value_for_display(
                        sect, opt, current_value, current_value_windows, sizeof(current_value_windows));
                    if(opt->value_default)
                        default_value = inicfg_windows_value_for_display(
                            sect, opt, default_value, default_value_windows, sizeof(default_value_windows));
#endif

                    if(show_default)
                        buffer_sprintf(wb, "\t#| datatype: %s, default value: %s\n",
                                       CONFIG_VALUE_TYPES_2str(opt->type),
                                       default_value);

                    buffer_sprintf(wb, "\t%s%s = %s\n",
                                   (
                                       !(opt->flags & CONFIG_VALUE_LOADED) &&
                                       !(opt->flags & CONFIG_VALUE_CHANGED) &&
                                       (opt->flags & CONFIG_VALUE_USED)
                                           ) ? "# " : "",
                                   string2str(opt->name),
                                   current_value);

                    options_added++;
                }
                SECTION_UNLOCK(sect);
            }
        }
        APPCONFIG_UNLOCK(root);
    }
}
