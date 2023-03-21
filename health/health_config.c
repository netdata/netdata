// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

#define HEALTH_CONF_MAX_LINE 4096

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_ALERT_KEY "alert"
#define HEALTH_TEMPLATE_KEY "template"
#define HEALTH_ON_KEY "on"
#define HEALTH_HOST_KEY "hosts"
#define HEALTH_OS_KEY "os"
#define HEALTH_FAMILIES_KEY "families"
#define HEALTH_PLUGIN_KEY "plugin"
#define HEALTH_MODULE_KEY "module"
#define HEALTH_CHARTS_KEY "charts"
#define HEALTH_LOOKUP_KEY "lookup"
#define HEALTH_CALC_KEY "calc"
#define HEALTH_EVERY_KEY "every"
#define HEALTH_GREEN_KEY "green"
#define HEALTH_RED_KEY "red"
#define HEALTH_WARN_KEY "warn"
#define HEALTH_CRIT_KEY "crit"
#define HEALTH_EXEC_KEY "exec"
#define HEALTH_RECIPIENT_KEY "to"
#define HEALTH_UNITS_KEY "units"
#define HEALTH_INFO_KEY "info"
#define HEALTH_CLASS_KEY "class"
#define HEALTH_COMPONENT_KEY "component"
#define HEALTH_TYPE_KEY "type"
#define HEALTH_DELAY_KEY "delay"
#define HEALTH_OPTIONS_KEY "options"
#define HEALTH_REPEAT_KEY "repeat"
#define HEALTH_HOST_LABEL_KEY "host labels"
#define HEALTH_FOREACH_KEY "foreach"

static inline int health_parse_delay(
        size_t line, const char *filename, char *string,
        int *delay_up_duration,
        int *delay_down_duration,
        int *delay_max_duration,
        float *delay_multiplier) {

    char given_up = 0;
    char given_down = 0;
    char given_max = 0;
    char given_multiplier = 0;

    char *s = string;
    while(*s) {
        char *key = s;

        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';

        if(!*key) break;

        char *value = s;
        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';

        if(!strcasecmp(key, "up")) {
            if (!config_parse_duration(value, delay_up_duration)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                        line, filename, value, key);
            }
            else given_up = 1;
        }
        else if(!strcasecmp(key, "down")) {
            if (!config_parse_duration(value, delay_down_duration)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                        line, filename, value, key);
            }
            else given_down = 1;
        }
        else if(!strcasecmp(key, "multiplier")) {
            *delay_multiplier = strtof(value, NULL);
            if(isnan(*delay_multiplier) || isinf(*delay_multiplier) || islessequal(*delay_multiplier, 0)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                        line, filename, value, key);
            }
            else given_multiplier = 1;
        }
        else if(!strcasecmp(key, "max")) {
            if (!config_parse_duration(value, delay_max_duration)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                        line, filename, value, key);
            }
            else given_max = 1;
        }
        else {
            error("Health configuration at line %zu of file '%s': unknown keyword '%s'",
                    line, filename, key);
        }
    }

    if(!given_up)
        *delay_up_duration = 0;

    if(!given_down)
        *delay_down_duration = 0;

    if(!given_multiplier)
        *delay_multiplier = 1.0;

    if(!given_max) {
        if((*delay_max_duration) < (*delay_up_duration) * (*delay_multiplier))
            *delay_max_duration = (int)((*delay_up_duration) * (*delay_multiplier));

        if((*delay_max_duration) < (*delay_down_duration) * (*delay_multiplier))
            *delay_max_duration = (int)((*delay_down_duration) * (*delay_multiplier));
    }

    return 1;
}

static inline uint32_t health_parse_options(const char *s) {
    uint32_t options = 0;
    char buf[100+1] = "";

    while(*s) {
        buf[0] = '\0';

        // skip spaces
        while(*s && isspace(*s))
            s++;

        // find the next space
        size_t count = 0;
        while(*s && count < 100 && !isspace(*s))
            buf[count++] = *s++;

        if(buf[0]) {
            buf[count] = '\0';

            if(!strcasecmp(buf, "no-clear-notification") || !strcasecmp(buf, "no-clear"))
                options |= RRDCALC_OPTION_NO_CLEAR_NOTIFICATION;
            else
                error("Ignoring unknown alarm option '%s'", buf);
        }
    }

    return options;
}

static inline int health_parse_repeat(
        size_t line,
        const char *file,
        char *string,
        uint32_t *warn_repeat_every,
        uint32_t *crit_repeat_every
) {

    char *s = string;
    while(*s) {
        char *key = s;

        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';

        if(!*key) break;

        char *value = s;
        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';

        if(!strcasecmp(key, "off")) {
            *warn_repeat_every = 0;
            *crit_repeat_every = 0;
            return 1;
        }
        if(!strcasecmp(key, "warning")) {
            if (!config_parse_duration(value, (int*)warn_repeat_every)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                      line, file, value, key);
            }
        }
        else if(!strcasecmp(key, "critical")) {
            if (!config_parse_duration(value, (int*)crit_repeat_every)) {
                error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                      line, file, value, key);
            }
        }
    }

    return 1;
}

static inline int isvariableterm(const char s) {
    if(isalnum(s) || s == '.' || s == '_')
        return 0;

    return 1;
}

static inline void parse_variables_and_store_in_health_rrdvars(char *value, size_t len) {
    const char *s = value;
    char buffer[RRDVAR_MAX_LENGTH];

    // $
    while (*s) {
        if(*s == '$') {
            size_t i = 0;
            s++;

            if(*s == '{') {
                // ${variable_name}

                s++;
                while (*s && *s != '}' && i < len)
                    buffer[i++] = *s++;

                if(*s == '}')
                    s++;
            }
            else {
                // $variable_name

                while (*s && !isvariableterm(*s) && i < len)
                    buffer[i++] = *s++;
            }

            buffer[i] = '\0';

            //TODO: check and try to store only variables
            STRING *name_string = rrdvar_name_to_string(buffer);
            rrdvar_add("health", health_rrdvars, name_string, RRDVAR_TYPE_CALCULATED, RRDVAR_FLAG_CONFIG_VAR, NULL);
            string_freez(name_string);
        } else
            s++;
    }
}

/**
 * Health pattern from Foreach
 *
 * Create a new simple pattern using the user input
 *
 * @param s the string that will be used to create the simple pattern.
 */

static void dimension_remove_pipe_comma(char *str) {
    while(*str) {
        if(*str == '|' || *str == ',') *str = ' ';
        str++;
    }
}

static SIMPLE_PATTERN *health_pattern_from_foreach(const char *s) {
    char *convert= strdupz(s);
    SIMPLE_PATTERN *val = NULL;

    if(convert) {
        dimension_remove_pipe_comma(convert);
        val = simple_pattern_create(convert, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(convert);
    }

    return val;
}

static inline int health_parse_db_lookup(
        size_t line, const char *filename, char *string,
        RRDR_TIME_GROUPING *group_method, int *after, int *before, int *every,
        RRDCALC_OPTIONS *options, STRING **dimensions, STRING **foreachdim
) {
    debug(D_HEALTH, "Health configuration parsing database lookup %zu@%s: %s", line, filename, string);

    if(*dimensions) string_freez(*dimensions);
    if(*foreachdim) string_freez(*foreachdim);
    *dimensions = NULL;
    *foreachdim = NULL;
    *after = 0;
    *before = 0;
    *every = 0;
    *options = (*options) & RRDCALC_ALL_OPTIONS_EXCLUDING_THE_RRDR_ONES; // preserve rrdcalc options

    char *s = string, *key;

    // first is the group method
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';
    if(!*s) {
        error("Health configuration invalid chart calculation at line %zu of file '%s': expected group method followed by the 'after' time, but got '%s'",
                line, filename, key);
        return 0;
    }

    if((*group_method = time_grouping_parse(key, RRDR_GROUPING_UNDEFINED)) == RRDR_GROUPING_UNDEFINED) {
        error("Health configuration at line %zu of file '%s': invalid group method '%s'",
                line, filename, key);
        return 0;
    }

    // then is the 'after' time
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';

    if(!config_parse_duration(key, after)) {
        error("Health configuration at line %zu of file '%s': invalid duration '%s' after group method",
                line, filename, key);
        return 0;
    }

    // sane defaults
    *every = ABS(*after);

    // now we may have optional parameters
    while(*s) {
        key = s;
        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';
        if(!*key) break;

        if(!strcasecmp(key, "at")) {
            char *value = s;
            while(*s && !isspace(*s)) s++;
            while(*s && isspace(*s)) *s++ = '\0';

            if (!config_parse_duration(value, before)) {
                error("Health configuration at line %zu of file '%s': invalid duration '%s' for '%s' keyword",
                        line, filename, value, key);
            }
        }
        else if(!strcasecmp(key, HEALTH_EVERY_KEY)) {
            char *value = s;
            while(*s && !isspace(*s)) s++;
            while(*s && isspace(*s)) *s++ = '\0';

            if (!config_parse_duration(value, every)) {
                error("Health configuration at line %zu of file '%s': invalid duration '%s' for '%s' keyword",
                        line, filename, value, key);
            }
        }
        else if(!strcasecmp(key, "absolute") || !strcasecmp(key, "abs") || !strcasecmp(key, "absolute_sum")) {
            *options |= RRDR_OPTION_ABSOLUTE;
        }
        else if(!strcasecmp(key, "min2max")) {
            *options |= RRDR_OPTION_MIN2MAX;
        }
        else if(!strcasecmp(key, "null2zero")) {
            *options |= RRDR_OPTION_NULL2ZERO;
        }
        else if(!strcasecmp(key, "percentage")) {
            *options |= RRDR_OPTION_PERCENTAGE;
        }
        else if(!strcasecmp(key, "unaligned")) {
            *options |= RRDR_OPTION_NOT_ALIGNED;
        }
        else if(!strcasecmp(key, "anomaly-bit")) {
            *options |= RRDR_OPTION_ANOMALY_BIT;
        }
        else if(!strcasecmp(key, "match-ids") || !strcasecmp(key, "match_ids")) {
            *options |= RRDR_OPTION_MATCH_IDS;
        }
        else if(!strcasecmp(key, "match-names") || !strcasecmp(key, "match_names")) {
            *options |= RRDR_OPTION_MATCH_NAMES;
        }
        else if(!strcasecmp(key, "of")) {
            char *find = NULL;
            if(*s && strcasecmp(s, "all") != 0) {
                find = strcasestr(s, " foreach");
                if(find) {
                    *find = '\0';
                }
                *dimensions = string_strdupz(s);
            }

            if(!find) {
                break;
            }
            s = ++find;
        }
        else if(!strcasecmp(key, HEALTH_FOREACH_KEY )) {
            *foreachdim = string_strdupz(s);
            break;
        }
        else {
            error("Health configuration at line %zu of file '%s': unknown keyword '%s'",
                    line, filename, key);
        }
    }

    return 1;
}

static inline STRING *health_source_file(size_t line, const char *file) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%zu@%s", line, file);
    return string_strdupz(buffer);
}

char *health_edit_command_from_source(const char *source)
{
    char buffer[FILENAME_MAX + 1];
    char *temp = strdupz(source);
    char *line_num = strchr(temp, '@');
    char *file_no_path = strrchr(temp, '/');

    if (likely(file_no_path && line_num)) {
        *line_num = '\0';
        snprintfz(
            buffer,
            FILENAME_MAX,
            "sudo %s/edit-config health.d/%s=%s=%s",
            netdata_configured_user_config_dir,
            file_no_path + 1,
            temp,
            rrdhost_registry_hostname(localhost));
    } else
        buffer[0] = '\0';

    freez(temp);
    return strdupz(buffer);
}

static inline void strip_quotes(char *s) {
    while(*s) {
        if(*s == '\'' || *s == '"') *s = ' ';
        s++;
    }
}

static inline void alert_config_free(struct alert_config *cfg)
{
    string_freez(cfg->alarm);                  cfg->alarm = NULL;
    string_freez(cfg->template_key);           cfg->template_key = NULL;
    string_freez(cfg->os);                     cfg->os = NULL;
    string_freez(cfg->host);                   cfg->host = NULL;
    string_freez(cfg->on);                     cfg->on = NULL;
    string_freez(cfg->families);               cfg->families = NULL;
    string_freez(cfg->plugin);                 cfg->plugin = NULL;
    string_freez(cfg->module);                 cfg->module = NULL;
    string_freez(cfg->charts);                 cfg->charts = NULL;
    string_freez(cfg->lookup);                 cfg->lookup = NULL;
    string_freez(cfg->calc);                   cfg->calc=NULL;
    string_freez(cfg->warn);                   cfg->warn = NULL;
    string_freez(cfg->crit);                   cfg->crit = NULL;
    string_freez(cfg->every);                  cfg->every = NULL;
    string_freez(cfg->green);                  cfg->green = NULL;
    string_freez(cfg->red);                    cfg->red = NULL;
    string_freez(cfg->exec);                   cfg->exec = NULL;
    string_freez(cfg->to);                     cfg->to = NULL;
    string_freez(cfg->units);                  cfg->units = NULL;
    string_freez(cfg->info);                   cfg->info = NULL;
    string_freez(cfg->classification);         cfg->classification = NULL;
    string_freez(cfg->component);              cfg->component = NULL;
    string_freez(cfg->type);                   cfg->type = NULL;
    string_freez(cfg->delay);                  cfg->delay = NULL;
    string_freez(cfg->options);                cfg->options = NULL;
    string_freez(cfg->repeat);                 cfg->repeat = NULL;
    string_freez(cfg->host_labels);            cfg->host_labels = NULL;
    string_freez(cfg->p_db_lookup_dimensions); cfg->p_db_lookup_dimensions = NULL;
    string_freez(cfg->p_db_lookup_method);     cfg->p_db_lookup_method = NULL;
}

void health_config_store_key(struct alert_config *cfg, const char *key, const char *value) {
    static uint32_t
            hash_alarm = 0,
            hash_alert = 0,
            hash_template = 0,
            hash_os = 0,
            hash_on = 0,
            hash_host = 0,
            hash_families = 0,
            hash_plugin = 0,
            hash_module = 0,
            hash_charts = 0,
            hash_calc = 0,
            hash_green = 0,
            hash_red = 0,
            hash_warn = 0,
            hash_crit = 0,
            hash_exec = 0,
            hash_every = 0,
            hash_lookup = 0,
            hash_units = 0,
            hash_info = 0,
            hash_class = 0,
            hash_component = 0,
            hash_type = 0,
            hash_recipient = 0,
            hash_delay = 0,
            hash_options = 0,
            hash_repeat = 0,
            hash_host_label = 0;

    if(unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_alert = simple_uhash(HEALTH_ALERT_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_on = simple_uhash(HEALTH_ON_KEY);
        hash_os = simple_uhash(HEALTH_OS_KEY);
        hash_host = simple_uhash(HEALTH_HOST_KEY);
        hash_families = simple_uhash(HEALTH_FAMILIES_KEY);
        hash_plugin = simple_uhash(HEALTH_PLUGIN_KEY);
        hash_module = simple_uhash(HEALTH_MODULE_KEY);
        hash_charts = simple_uhash(HEALTH_CHARTS_KEY);
        hash_calc = simple_uhash(HEALTH_CALC_KEY);
        hash_lookup = simple_uhash(HEALTH_LOOKUP_KEY);
        hash_green = simple_uhash(HEALTH_GREEN_KEY);
        hash_red = simple_uhash(HEALTH_RED_KEY);
        hash_warn = simple_uhash(HEALTH_WARN_KEY);
        hash_crit = simple_uhash(HEALTH_CRIT_KEY);
        hash_exec = simple_uhash(HEALTH_EXEC_KEY);
        hash_every = simple_uhash(HEALTH_EVERY_KEY);
        hash_units = simple_hash(HEALTH_UNITS_KEY);
        hash_info = simple_hash(HEALTH_INFO_KEY);
        hash_class = simple_uhash(HEALTH_CLASS_KEY);
        hash_component = simple_uhash(HEALTH_COMPONENT_KEY);
        hash_type = simple_uhash(HEALTH_TYPE_KEY);
        hash_recipient = simple_hash(HEALTH_RECIPIENT_KEY);
        hash_delay = simple_uhash(HEALTH_DELAY_KEY);
        hash_options = simple_uhash(HEALTH_OPTIONS_KEY);
        hash_repeat = simple_uhash(HEALTH_REPEAT_KEY);
        hash_host_label = simple_uhash(HEALTH_HOST_LABEL_KEY);
    }

    uint32_t hash = simple_uhash(key);

    if(hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY))
        cfg->alarm = string_strdupz(value);
    else if(hash == hash_alert && !strcasecmp(key, HEALTH_ALERT_KEY))
        cfg->alarm = string_strdupz(value);
    else if (hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY))
        cfg->template_key = string_strdupz(value);
    else if (hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY))
        cfg->on = string_strdupz(value);
    else if (hash == hash_os && !strcasecmp(key, HEALTH_OS_KEY))
        cfg->os = string_strdupz(value);
    else if (hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY))
        cfg->host = string_strdupz(value);
    else if (hash == hash_families && !strcasecmp(key, HEALTH_FAMILIES_KEY))
        cfg->families = string_strdupz(value);
    else if (hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY))
        cfg->plugin = string_strdupz(value);
    else if (hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY))
        cfg->module = string_strdupz(value);
    else if (hash == hash_charts && !strcasecmp(key, HEALTH_CHARTS_KEY))
        cfg->charts = string_strdupz(value);
    else if (hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY))
        cfg->calc = string_strdupz(value);
    else if (hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY))
        cfg->lookup = string_strdupz(value);
    else if (hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY))
        cfg->green = string_strdupz(value);
    else if (hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY))
        cfg->red = string_strdupz(value);
    else if (hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY))
        cfg->warn = string_strdupz(value);
    else if (hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY))
        cfg->crit = string_strdupz(value);
    else if (hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY))
        cfg->exec = string_strdupz(value);
    else if (hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY))
        cfg->every = string_strdupz(value);
    else if (hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY))
        cfg->units = string_strdupz(value);
    else if (hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY))
        cfg->info = string_strdupz(value);
    else if (hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY))
        cfg->classification = string_strdupz(value);
    else if (hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY))
        cfg->component = string_strdupz(value);
    else if (hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY))
        cfg->type = string_strdupz(value);
    else if (hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY))
        cfg->to = string_strdupz(value);
    else if (hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY))
        cfg->delay = string_strdupz(value);
    else if (hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY))
        cfg->options = string_strdupz(value);
    else if (hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY))
        cfg->repeat = string_strdupz(value);
    else if (hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY))
        cfg->host_labels = string_strdupz(value);
}

void health_create_alert_from_config(struct alert_config *doc_cfg, struct alert_config *sec_cfg, RRDHOST *host, char *filename, size_t line)
{
    if (!sec_cfg->on && doc_cfg->on) sec_cfg->on = string_dup(doc_cfg->on);
    if (!sec_cfg->os && doc_cfg->os) sec_cfg->os = string_dup(doc_cfg->os);
    if (!sec_cfg->host && doc_cfg->host) sec_cfg->host = string_dup(doc_cfg->host);
    if (!sec_cfg->families && doc_cfg->families) sec_cfg->families = string_dup(doc_cfg->families);
    if (!sec_cfg->plugin && doc_cfg->plugin) sec_cfg->plugin = string_dup(doc_cfg->plugin);
    if (!sec_cfg->module && doc_cfg->module) sec_cfg->module = string_dup(doc_cfg->module);
    if (!sec_cfg->charts && doc_cfg->charts) sec_cfg->charts = string_dup(doc_cfg->charts);
    if (!sec_cfg->calc && doc_cfg->calc) sec_cfg->calc = string_dup(doc_cfg->calc);
    if (!sec_cfg->lookup && doc_cfg->lookup) sec_cfg->lookup = string_dup(doc_cfg->lookup);
    if (!sec_cfg->green && doc_cfg->green) sec_cfg->green = string_dup(doc_cfg->green);
    if (!sec_cfg->red && doc_cfg->red) sec_cfg->red = string_dup(doc_cfg->red);
    if (!sec_cfg->warn && doc_cfg->warn) sec_cfg->warn = string_dup(doc_cfg->warn);
    if (!sec_cfg->crit && doc_cfg->crit) sec_cfg->crit = string_dup(doc_cfg->crit);
    if (!sec_cfg->exec && doc_cfg->exec) sec_cfg->exec = string_dup(doc_cfg->exec);
    if (!sec_cfg->every && doc_cfg->every) sec_cfg->every = string_dup(doc_cfg->every);
    if (!sec_cfg->units && doc_cfg->units) sec_cfg->units = string_dup(doc_cfg->units);
    if (!sec_cfg->info && doc_cfg->info) sec_cfg->info = string_dup(doc_cfg->info);
    if (!sec_cfg->classification && doc_cfg->classification) sec_cfg->classification = string_dup(doc_cfg->classification);
    if (!sec_cfg->component && doc_cfg->component) sec_cfg->component = string_dup(doc_cfg->component);
    if (!sec_cfg->type && doc_cfg->type) sec_cfg->type = string_dup(doc_cfg->type);
    if (!sec_cfg->to && doc_cfg->to) sec_cfg->to = string_dup(doc_cfg->to);
    if (!sec_cfg->delay && doc_cfg->delay) sec_cfg->delay = string_dup(doc_cfg->delay);
    if (!sec_cfg->options && doc_cfg->options) sec_cfg->options = string_dup(doc_cfg->options);
    if (!sec_cfg->repeat && doc_cfg->repeat) sec_cfg->repeat = string_dup(doc_cfg->repeat);
    if (!sec_cfg->host_labels && doc_cfg->host_labels) sec_cfg->host_labels = string_dup(doc_cfg->host_labels);

    RRDCALC *rc = NULL;
    RRDCALCTEMPLATE *rt = NULL;

    if (sec_cfg->os) {
        SIMPLE_PATTERN *os_pattern = simple_pattern_create(string2str(sec_cfg->os), NULL, SIMPLE_PATTERN_EXACT, true);

        if(!simple_pattern_matches_string(os_pattern, host->os)) {
            if(rc)
                debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: host O/S does not match '%s'", rrdhost_hostname(host), rrdcalc_name(rc), line, filename, string2str(sec_cfg->os));

            if(rt)
                debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: host O/S does not match '%s'", rrdhost_hostname(host), rrdcalctemplate_name(rt), line, filename, string2str(sec_cfg->os));

            simple_pattern_free(os_pattern);
            return;
        }
        simple_pattern_free(os_pattern);
    }

    if (sec_cfg->host) {
        SIMPLE_PATTERN *host_pattern = simple_pattern_create(string2str(sec_cfg->host), NULL, SIMPLE_PATTERN_EXACT, true);

        if(!simple_pattern_matches_string(host_pattern, host->hostname)) {
            if(rc)
                debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: hostname does not match '%s'", rrdhost_hostname(host), rrdcalc_name(rc), line, filename, string2str(sec_cfg->host));

            if(rt)
                debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: hostname does not match '%s'", rrdhost_hostname(host), rrdcalctemplate_name(rt), line, filename, string2str(sec_cfg->host));

            simple_pattern_free(host_pattern);
            return;
        }
        simple_pattern_free(host_pattern);
    }


    if (sec_cfg->template_key) {
        if (simple_pattern_matches(conf_enabled_alarms, string2str(sec_cfg->template_key))) {
            rt = callocz(1, sizeof(RRDCALCTEMPLATE));

            {
                char *tmp = strdupz(string2str(sec_cfg->template_key));
                if(rrdvar_fix_name(tmp))
                    error("Health configuration renamed template '%s' to '%s'", string2str(sec_cfg->template_key), tmp);

                rt->name = string_strdupz(tmp);
                freez(tmp);
            }

            //rt->source = health_source_file(line, filename); TODO
            rt->green = NAN;
            rt->red = NAN;
            rt->delay_multiplier = (float)1.0;
            rt->warn_repeat_every = host->health.health_default_warn_repeat_every;
            rt->crit_repeat_every = host->health.health_default_crit_repeat_every;
            //ignore_this = 0;
        }
    } else if (sec_cfg->alarm) {
        if (simple_pattern_matches(conf_enabled_alarms, string2str(sec_cfg->alarm))) {
            rc = callocz(1, sizeof(RRDCALC));
            rc->next_event_id = 1;

            {
                char *tmp = strdupz(string2str(sec_cfg->alarm));
                if(rrdvar_fix_name(tmp))
                    error("Health configuration renamed alarm '%s' to '%s'", string2str(sec_cfg->alarm), tmp);

                rc->name = string_strdupz(tmp);
                freez(tmp);
            }

            //rc->source = health_source_file(line, filename); TODO
            rc->green = NAN;
            rc->red = NAN;
            rc->value = NAN;
            rc->old_value = NAN;
            rc->delay_multiplier = 1.0;
            rc->old_status = RRDCALC_STATUS_UNINITIALIZED;
            rc->warn_repeat_every = host->health.health_default_warn_repeat_every;
            rc->crit_repeat_every = host->health.health_default_crit_repeat_every;
            //ignore_this = 0;
        }
    }

    if (sec_cfg->on) {
        if (rc) {
            rc->chart = string_dup(sec_cfg->on);
        } else if (rt) {
            rt->context = string_dup(sec_cfg->on);
        }
    } else {
        //required field, free rc/rt and exit
    }

    if (sec_cfg->classification) {
        //strip_quotes
        if (rc)
            rc->classification = string_dup(sec_cfg->classification);
        else if (rt)
            rt->classification = string_dup(sec_cfg->classification);
    }
    if (sec_cfg->component) {
        if (rc)
            rc->component = string_dup(sec_cfg->component);
        else if (rt)
            rt->component = string_dup(sec_cfg->component);
    }
    if (sec_cfg->type) {
        if (rc)
            rc->type = string_dup(sec_cfg->type);
        else if (rt)
            rt->type = string_dup(sec_cfg->type);
    }

    if (sec_cfg->lookup) {
        if (rc) {
            health_parse_db_lookup(line, filename, (char *)string2str(sec_cfg->lookup), &rc->group, &rc->after, &rc->before,
                        &rc->update_every, &rc->options, &rc->dimensions, &rc->foreach_dimension);

            if(rc->foreach_dimension)
                rc->foreach_dimension_pattern = health_pattern_from_foreach(rrdcalc_foreachdim(rc));

            if (rc->after) {
                if (rc->dimensions)
                    sec_cfg->p_db_lookup_dimensions = string_dup(rc->dimensions);
                if (rc->group)
                    sec_cfg->p_db_lookup_method = string_strdupz(time_grouping_method2string(rc->group));
                sec_cfg->p_db_lookup_options = rc->options;
                sec_cfg->p_db_lookup_after = rc->after;
                sec_cfg->p_db_lookup_before = rc->before;
                sec_cfg->p_update_every = rc->update_every;
            }
        } else if (rt) {
            health_parse_db_lookup(line, filename, (char *)string2str(sec_cfg->lookup), &rt->group, &rt->after, &rt->before,
                        &rt->update_every, &rt->options, &rt->dimensions, &rt->foreach_dimension);

            if(rt->foreach_dimension)
                rt->foreach_dimension_pattern = health_pattern_from_foreach(rrdcalctemplate_foreachdim(rt));

            if (rt->after) {
                if (rt->dimensions)
                    sec_cfg->p_db_lookup_dimensions = string_dup(rt->dimensions);

                if (rt->group)
                    sec_cfg->p_db_lookup_method = string_strdupz(time_grouping_method2string(rt->group));

                sec_cfg->p_db_lookup_options = rt->options;
                sec_cfg->p_db_lookup_after = rt->after;
                sec_cfg->p_db_lookup_before = rt->before;
                sec_cfg->p_update_every = rt->update_every;
            }
        }
    }
    if (sec_cfg->every) {
        if (rc) {
            if(!config_parse_duration(string2str(sec_cfg->every), &rc->update_every))
                error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' cannot parse duration: '%s'.",
                      line, filename, rrdcalc_name(rc), "every", string2str(sec_cfg->every));
            sec_cfg->p_update_every = rc->update_every;
        } else if (rt) {
            if(!config_parse_duration(string2str(sec_cfg->every), &rt->update_every))
                error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' cannot parse duration: '%s'.",
                      line, filename, rrdcalctemplate_name(rt), "every", string2str(sec_cfg->every));
            sec_cfg->p_update_every = rt->update_every;
        }
    }
    if(sec_cfg->green) {
        if (rc) {
            char *e;
            rc->green = str2ndd(string2str(sec_cfg->green), &e);
            if(e && *e) {
                error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                      line, filename, rrdcalc_name(rc), "green", e);
            }
        } else if (rt) {
            char *e;
            rt->green = str2ndd(string2str(sec_cfg->green), &e);
            if(e && *e) {
                error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                      line, filename, rrdcalc_name(rt), "green", e);
            }
        }
    }
    if(sec_cfg->red) {
        if (rc) {
            char *e;
            rc->red = str2ndd(string2str(sec_cfg->red), &e);
            if(e && *e) {
                error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                      line, filename, rrdcalc_name(rc), "red", e);
            }
        } else if (rt) {
            char *e;
            rt->red = str2ndd(string2str(sec_cfg->red), &e);
            if(e && *e) {
                error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                      line, filename, rrdcalc_name(rt), "red", e);
            }
        }
    }
    if (sec_cfg->calc) {
        if (rc) {
            const char *failed_at = NULL;
            int error = 0;
            rc->calculation = expression_parse(string2str(sec_cfg->calc), &failed_at, &error);
            if(!rc->calculation) {
                log_health("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalctemplate_name(rc), "calc", string2str(sec_cfg->calc), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->calc), HEALTH_CONF_MAX_LINE);
        } else if (rt) {
            const char *failed_at = NULL;
            int error = 0;
            rt->calculation = expression_parse(string2str(sec_cfg->calc), &failed_at, &error);
            if(!rt->calculation) {
                log_health("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalctemplate_name(rt), "calc", string2str(sec_cfg->calc), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->calc), HEALTH_CONF_MAX_LINE);
        }
    }
    if (sec_cfg->warn) {
        if (rc) {
            const char *failed_at = NULL;
            int error = 0;
            rc->warning = expression_parse(string2str(sec_cfg->warn), &failed_at, &error);
            if(!rc->warning) {
                error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalc_name(rc), "warn", string2str(sec_cfg->warn), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->warn), HEALTH_CONF_MAX_LINE);
        } else if (rt) {
            const char *failed_at = NULL;
            int error = 0;
            rt->warning = expression_parse(string2str(sec_cfg->warn), &failed_at, &error);
            if(!rt->warning) {
                error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalc_name(rt), "warn", string2str(sec_cfg->warn), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->warn), HEALTH_CONF_MAX_LINE);
        }
    }
    if (sec_cfg->crit) {
        if (rc) {
            const char *failed_at = NULL;
            int error = 0;
            rc->critical = expression_parse(string2str(sec_cfg->crit), &failed_at, &error);
            if(!rc->warning) {
                error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalc_name(rc), "crit", string2str(sec_cfg->crit), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->crit), HEALTH_CONF_MAX_LINE);
        } else if (rt) {
            const char *failed_at = NULL;
            int error = 0;
            rt->critical = expression_parse(string2str(sec_cfg->crit), &failed_at, &error);
            if(!rt->warning) {
                error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                      line, filename, rrdcalc_name(rt), "crit", string2str(sec_cfg->crit), expression_strerror(error), failed_at);
            }
            parse_variables_and_store_in_health_rrdvars((char *)string2str(sec_cfg->crit), HEALTH_CONF_MAX_LINE);
        }
    }
    if (sec_cfg->exec) {
        if (rc) {
            rc->exec = string_dup(sec_cfg->exec);
        } else if (rt) {
            rt->exec = string_dup(sec_cfg->exec);
        }
    }
    if (sec_cfg->to) {
        if (rc) {
            rc->recipient = string_dup(sec_cfg->to);
        } else if (rt) {
            rt->recipient = string_dup(sec_cfg->to);
        }
    }
    if (sec_cfg->units) {
        if (rc) {
            rc->units = string_dup(sec_cfg->units);
        } else if (rt) {
            rt->units = string_dup(sec_cfg->units);
        }
    }
    if (sec_cfg->info) {
        if (rc) {
            rc->info = string_dup(sec_cfg->info);
            rc->original_info = string_dup(sec_cfg->info);
        } else if (rt) {
            rt->info = string_dup(sec_cfg->info);
        }
    }
    if (sec_cfg->delay) {
        if (rc) {
            health_parse_delay(line, filename, (char *)string2str(sec_cfg->delay), &rc->delay_up_duration, &rc->delay_down_duration, &rc->delay_max_duration, &rc->delay_multiplier);
        } else if (rt) {
            health_parse_delay(line, filename, (char *)string2str(sec_cfg->delay), &rt->delay_up_duration, &rt->delay_down_duration, &rt->delay_max_duration, &rt->delay_multiplier);
        }
    }
    if (sec_cfg->options) {
        if (rc) {
            rc->options |= health_parse_options((char *)string2str(sec_cfg->options));
        } else if (rt) {
            rt->options |= health_parse_options((char *)string2str(sec_cfg->options));
        }
    }
    if (sec_cfg->repeat) {
        if (rc) {
            health_parse_repeat(line, filename, (char *)string2str(sec_cfg->repeat), &rc->warn_repeat_every, &rc->crit_repeat_every);
        } else if (rt) {
            health_parse_repeat(line, filename, (char *)string2str(sec_cfg->repeat), &rt->warn_repeat_every, &rt->crit_repeat_every);
        }
    }
    if (sec_cfg->host_labels) {
        char *tmp = simple_pattern_trim_around_equal((char *)string2str(sec_cfg->host_labels));
        if (rc) {
            rc->host_labels = string_strdupz(tmp);
            rc->host_labels_pattern = simple_pattern_create(rrdcalc_host_labels(rc), NULL, SIMPLE_PATTERN_EXACT, true);
        } else if (rt) {
            rt->host_labels = string_strdupz(tmp);
            rt->host_labels_pattern = simple_pattern_create(rrdcalc_host_labels(rt), NULL, SIMPLE_PATTERN_EXACT, true);
        }
        freez(tmp);
    }
    if(sec_cfg->plugin) {
        if (rc) {
            string_freez(rc->plugin_match);
            simple_pattern_free(rc->plugin_pattern);

            rc->plugin_match = string_dup(sec_cfg->plugin);
            rc->plugin_pattern = simple_pattern_create(rrdcalc_plugin_match(rc), NULL, SIMPLE_PATTERN_EXACT, true);
        } else if (rt) {
            string_freez(rt->plugin_match);
            simple_pattern_free(rt->plugin_pattern);

            rt->plugin_match = string_dup(sec_cfg->plugin);
            rt->plugin_pattern = simple_pattern_create(rrdcalc_plugin_match(rt), NULL, SIMPLE_PATTERN_EXACT, true);
        }
    }
    if(sec_cfg->module) {
        if (rc) {
            string_freez(rc->module_match);
            simple_pattern_free(rc->module_pattern);

            rc->module_match = string_dup(sec_cfg->module);
            rc->module_pattern = simple_pattern_create(rrdcalc_module_match(rc), NULL, SIMPLE_PATTERN_EXACT, true);
        } else if (rt) {
            string_freez(rt->module_match);
            simple_pattern_free(rt->module_pattern);

            rt->module_match = string_dup(sec_cfg->module);
            rt->module_pattern = simple_pattern_create(rrdcalc_module_match(rt), NULL, SIMPLE_PATTERN_EXACT, true);
        }
    }
    if (sec_cfg->families) {
        if (rt) {
            string_freez(rt->family_match);
            simple_pattern_free(rt->family_pattern);

            rt->family_match = string_dup(sec_cfg->families);
            rt->family_pattern = simple_pattern_create(rrdcalctemplate_family_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                       true);
        }
    }
    if (sec_cfg->charts) {
        if (rt) {
            string_freez(rt->charts_match);
            simple_pattern_free(rt->charts_pattern);

            rt->charts_match = string_dup(sec_cfg->charts);
            rt->charts_pattern = simple_pattern_create(rrdcalctemplate_charts_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                       true);
        }
    }

    if (rc) {
        alert_hash_and_store_config(rc->config_hash_id, sec_cfg, 1);//fix
        rrdcalc_add_from_config(host, rc);
    } else if (rt) {
        rrdcalctemplate_add_from_config(host, rt);
    }
}

int sql_store_hashes = 1;
static int health_legacy_readfile(const char *filename, void *data) {
    RRDHOST *host = (RRDHOST *)data;

    debug(D_HEALTH, "Health configuration reading file '%s'", filename);

    static uint32_t
            hash_alarm = 0,
            hash_template = 0,
            hash_os = 0,
            hash_on = 0,
            hash_host = 0,
            hash_families = 0,
            hash_plugin = 0,
            hash_module = 0,
            hash_charts = 0,
            hash_calc = 0,
            hash_green = 0,
            hash_red = 0,
            hash_warn = 0,
            hash_crit = 0,
            hash_exec = 0,
            hash_every = 0,
            hash_lookup = 0,
            hash_units = 0,
            hash_info = 0,
            hash_class = 0,
            hash_component = 0,
            hash_type = 0,
            hash_recipient = 0,
            hash_delay = 0,
            hash_options = 0,
            hash_repeat = 0,
            hash_host_label = 0;

    char buffer[HEALTH_CONF_MAX_LINE + 1];

    if(unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_on = simple_uhash(HEALTH_ON_KEY);
        hash_os = simple_uhash(HEALTH_OS_KEY);
        hash_host = simple_uhash(HEALTH_HOST_KEY);
        hash_families = simple_uhash(HEALTH_FAMILIES_KEY);
        hash_plugin = simple_uhash(HEALTH_PLUGIN_KEY);
        hash_module = simple_uhash(HEALTH_MODULE_KEY);
        hash_charts = simple_uhash(HEALTH_CHARTS_KEY);
        hash_calc = simple_uhash(HEALTH_CALC_KEY);
        hash_lookup = simple_uhash(HEALTH_LOOKUP_KEY);
        hash_green = simple_uhash(HEALTH_GREEN_KEY);
        hash_red = simple_uhash(HEALTH_RED_KEY);
        hash_warn = simple_uhash(HEALTH_WARN_KEY);
        hash_crit = simple_uhash(HEALTH_CRIT_KEY);
        hash_exec = simple_uhash(HEALTH_EXEC_KEY);
        hash_every = simple_uhash(HEALTH_EVERY_KEY);
        hash_units = simple_hash(HEALTH_UNITS_KEY);
        hash_info = simple_hash(HEALTH_INFO_KEY);
        hash_class = simple_uhash(HEALTH_CLASS_KEY);
        hash_component = simple_uhash(HEALTH_COMPONENT_KEY);
        hash_type = simple_uhash(HEALTH_TYPE_KEY);
        hash_recipient = simple_hash(HEALTH_RECIPIENT_KEY);
        hash_delay = simple_uhash(HEALTH_DELAY_KEY);
        hash_options = simple_uhash(HEALTH_OPTIONS_KEY);
        hash_repeat = simple_uhash(HEALTH_REPEAT_KEY);
        hash_host_label = simple_uhash(HEALTH_HOST_LABEL_KEY);
    }

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        error("Health configuration cannot read file '%s'.", filename);
        return 0;
    }

    RRDCALC *rc = NULL;
    RRDCALCTEMPLATE *rt = NULL;
    struct alert_config *alert_cfg = NULL;

    int ignore_this = 0;
    size_t line = 0, append = 0;
    char *s;
    while((s = fgets(&buffer[append], (int)(HEALTH_CONF_MAX_LINE - append), fp)) || append) {
        int stop_appending = !s;
        line++;
        s = trim(buffer);
        if(!s || *s == '#') continue;

        append = strlen(s);
        if(!stop_appending && s[append - 1] == '\\') {
            s[append - 1] = ' ';
            append = &s[append] - buffer;
            if(append < HEALTH_CONF_MAX_LINE)
                continue;
            else {
                error("Health configuration has too long multi-line at line %zu of file '%s'.", line, filename);
            }
        }
        append = 0;

        char *key = s;
        while(*s && *s != ':') s++;
        if(!*s) {
            error("Health configuration has invalid line %zu of file '%s'. It does not contain a ':'. Ignoring it.", line, filename);
            continue;
        }
        *s = '\0';
        s++;

        char *value = s;
        key = trim_all(key);
        value = trim_all(value);

        if(!key) {
            error("Health configuration has invalid line %zu of file '%s'. Keyword is empty. Ignoring it.", line, filename);
            continue;
        }

        if(!value) {
            error("Health configuration has invalid line %zu of file '%s'. value is empty. Ignoring it.", line, filename);
            continue;
        }

        uint32_t hash = simple_uhash(key);

        if(hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) {
            if(rc) {
                if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
                    rrdcalc_free_unused_rrdcalc_loaded_from_config(rc);
                else
                    rrdcalc_add_from_config(host, rc);

               // health_add_alarms_loop(host, rc, ignore_this) ;
            }

            if(rt) {
                if(!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
                    rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(rt);
                else
                    rrdcalctemplate_add_from_config(host, rt);

                rt = NULL;
            }

            if (simple_pattern_matches(conf_enabled_alarms, value)) {
                rc = callocz(1, sizeof(RRDCALC));
                rc->next_event_id = 1;

                {
                    char *tmp = strdupz(value);
                    if(rrdvar_fix_name(tmp))
                        error("Health configuration renamed alarm '%s' to '%s'", value, tmp);

                    rc->name = string_strdupz(tmp);
                    freez(tmp);
                }

                rc->source = health_source_file(line, filename);
                rc->green = NAN;
                rc->red = NAN;
                rc->value = NAN;
                rc->old_value = NAN;
                rc->delay_multiplier = 1.0;
                rc->old_status = RRDCALC_STATUS_UNINITIALIZED;
                rc->warn_repeat_every = host->health.health_default_warn_repeat_every;
                rc->crit_repeat_every = host->health.health_default_crit_repeat_every;
                if (alert_cfg)
                    alert_config_free(alert_cfg);
                alert_cfg = callocz(1, sizeof(struct alert_config));

                alert_cfg->alarm = string_dup(rc->name);
                ignore_this = 0;
            } else {
                rc = NULL;
            }
        }
        else if(hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY)) {
            if(rc) {
//                health_add_alarms_loop(host, rc, ignore_this) ;
                if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
                    rrdcalc_free_unused_rrdcalc_loaded_from_config(rc);
                else
                    rrdcalc_add_from_config(host, rc);

                rc = NULL;
            }

            if(rt) {
                if(!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
                    rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(rt);
                else
                    rrdcalctemplate_add_from_config(host, rt);
            }

            if (simple_pattern_matches(conf_enabled_alarms, value)) {
                rt = callocz(1, sizeof(RRDCALCTEMPLATE));

                {
                    char *tmp = strdupz(value);
                    if(rrdvar_fix_name(tmp))
                        error("Health configuration renamed template '%s' to '%s'", value, tmp);

                    rt->name = string_strdupz(tmp);
                    freez(tmp);
                }

                rt->source = health_source_file(line, filename);
                rt->green = NAN;
                rt->red = NAN;
                rt->delay_multiplier = (float)1.0;
                rt->warn_repeat_every = host->health.health_default_warn_repeat_every;
                rt->crit_repeat_every = host->health.health_default_crit_repeat_every;
                if (alert_cfg)
                    alert_config_free(alert_cfg);
                alert_cfg = callocz(1, sizeof(struct alert_config));

                alert_cfg->template_key = string_dup(rt->name);
                ignore_this = 0;
            } else {
                rt = NULL;
            }
        }
        else if(hash == hash_os && !strcasecmp(key, HEALTH_OS_KEY)) {
            char *os_match = value;
            if (alert_cfg) alert_cfg->os = string_strdupz(value);
            SIMPLE_PATTERN *os_pattern = simple_pattern_create(os_match, NULL, SIMPLE_PATTERN_EXACT, true);

            if(!simple_pattern_matches_string(os_pattern, host->os)) {
                if(rc)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: host O/S does not match '%s'", rrdhost_hostname(host), rrdcalc_name(rc), line, filename, os_match);

                if(rt)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: host O/S does not match '%s'", rrdhost_hostname(host), rrdcalctemplate_name(rt), line, filename, os_match);

                ignore_this = 1;
            }

            simple_pattern_free(os_pattern);
        }
        else if(hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) {
            char *host_match = value;
            if (alert_cfg) alert_cfg->host = string_strdupz(value);
            SIMPLE_PATTERN *host_pattern = simple_pattern_create(host_match, NULL, SIMPLE_PATTERN_EXACT, true);

            if(!simple_pattern_matches_string(host_pattern, host->hostname)) {
                if(rc)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: hostname does not match '%s'", rrdhost_hostname(host), rrdcalc_name(rc), line, filename, host_match);

                if(rt)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: hostname does not match '%s'", rrdhost_hostname(host), rrdcalctemplate_name(rt), line, filename, host_match);

                ignore_this = 1;
            }

            simple_pattern_free(host_pattern);
        }
        else if(rc) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                alert_cfg->on = string_strdupz(value);
                if(rc->chart) {
                    if(strcmp(rrdcalc_chart_name(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_chart_name(rc), value, value);

                    string_freez(rc->chart);
                }
                rc->chart = string_strdupz(value);
            }
            else if(hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY)) {
                strip_quotes(value);

                alert_cfg->classification = string_strdupz(value);
                if(rc->classification) {
                    if(strcmp(rrdcalc_classification(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_classification(rc), value, value);

                    string_freez(rc->classification);
                }
                rc->classification = string_strdupz(value);
            }
            else if(hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY)) {
                strip_quotes(value);

                alert_cfg->component = string_strdupz(value);
                if(rc->component) {
                    if(strcmp(rrdcalc_component(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_component(rc), value, value);

                    string_freez(rc->component);
                }
                rc->component = string_strdupz(value);
            }
            else if(hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY)) {
                strip_quotes(value);

                alert_cfg->type = string_strdupz(value);
                if(rc->type) {
                    if(strcmp(rrdcalc_type(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_type(rc), value, value);

                    string_freez(rc->type);
                }
                rc->type = string_strdupz(value);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                alert_cfg->lookup = string_strdupz(value);
                health_parse_db_lookup(line, filename, value, &rc->group, &rc->after, &rc->before,
                        &rc->update_every, &rc->options, &rc->dimensions, &rc->foreach_dimension);

                if(rc->foreach_dimension)
                    rc->foreach_dimension_pattern = health_pattern_from_foreach(rrdcalc_foreachdim(rc));

                if (rc->after) {
                    if (rc->dimensions)
                        alert_cfg->p_db_lookup_dimensions = string_dup(rc->dimensions);
                    if (rc->group)
                        alert_cfg->p_db_lookup_method = string_strdupz(time_grouping_method2string(rc->group));
                    alert_cfg->p_db_lookup_options = rc->options;
                    alert_cfg->p_db_lookup_after = rc->after;
                    alert_cfg->p_db_lookup_before = rc->before;
                    alert_cfg->p_update_every = rc->update_every;
                }
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                alert_cfg->every = string_strdupz(value);
                if(!config_parse_duration(value, &rc->update_every))
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' cannot parse duration: '%s'.",
                            line, filename, rrdcalc_name(rc), key, value);
                alert_cfg->p_update_every = rc->update_every;
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                alert_cfg->green = string_strdupz(value);
                char *e;
                rc->green = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rrdcalc_name(rc), key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                alert_cfg->red = string_strdupz(value);
                char *e;
                rc->red = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rrdcalc_name(rc), key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                alert_cfg->calc = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->calculation = expression_parse(value, &failed_at, &error);
                if(!rc->calculation) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalc_name(rc), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                alert_cfg->warn = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->warning = expression_parse(value, &failed_at, &error);
                if(!rc->warning) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalc_name(rc), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                alert_cfg->crit = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->critical = expression_parse(value, &failed_at, &error);
                if(!rc->critical) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalc_name(rc), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                alert_cfg->exec = string_strdupz(value);
                if(rc->exec) {
                    if(strcmp(rrdcalc_exec(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_exec(rc), value, value);

                    string_freez(rc->exec);
                }
                rc->exec = string_strdupz(value);
            }
            else if(hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY)) {
                alert_cfg->to = string_strdupz(value);
                if(rc->recipient) {
                    if(strcmp(rrdcalc_recipient(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_recipient(rc), value, value);

                    string_freez(rc->recipient);
                }
                rc->recipient = string_strdupz(value);
            }
            else if(hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY)) {
                strip_quotes(value);

                alert_cfg->units = string_strdupz(value);
                if(rc->units) {
                    if(strcmp(rrdcalc_units(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_units(rc), value, value);

                    string_freez(rc->units);
                }
                rc->units = string_strdupz(value);
            }
            else if(hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY)) {
                strip_quotes(value);

                alert_cfg->info = string_strdupz(value);
                if(rc->info) {
                    if(strcmp(rrdcalc_info(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalc_name(rc), key, rrdcalc_info(rc), value, value);

                    string_freez(rc->info);
                    string_freez(rc->original_info);
                }
                rc->info = string_strdupz(value);
                rc->original_info = string_dup(rc->info);
            }
            else if(hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY)) {
                alert_cfg->delay = string_strdupz(value);
                health_parse_delay(line, filename, value, &rc->delay_up_duration, &rc->delay_down_duration, &rc->delay_max_duration, &rc->delay_multiplier);
            }
            else if(hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY)) {
                alert_cfg->options = string_strdupz(value);
                rc->options |= health_parse_options(value);
            }
            else if(hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY)){
                alert_cfg->repeat = string_strdupz(value);
                health_parse_repeat(line, filename, value,
                                    &rc->warn_repeat_every,
                                    &rc->crit_repeat_every);
            }
            else if(hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY)) {
                alert_cfg->host_labels = string_strdupz(value);
                if(rc->host_labels) {
                    if(strcmp(rrdcalc_host_labels(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'.",
                              line, filename, rrdcalc_name(rc), key, value, value);

                    string_freez(rc->host_labels);
                    simple_pattern_free(rc->host_labels_pattern);
                }

                {
                    char *tmp = simple_pattern_trim_around_equal(value);
                    rc->host_labels = string_strdupz(tmp);
                    freez(tmp);
                }
                rc->host_labels_pattern = simple_pattern_create(rrdcalc_host_labels(rc), NULL, SIMPLE_PATTERN_EXACT,
                                                                true);
            }
            else if(hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY)) {
                alert_cfg->plugin = string_strdupz(value);
                string_freez(rc->plugin_match);
                simple_pattern_free(rc->plugin_pattern);

                rc->plugin_match = string_strdupz(value);
                rc->plugin_pattern = simple_pattern_create(rrdcalc_plugin_match(rc), NULL, SIMPLE_PATTERN_EXACT, true);
            }
            else if(hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY)) {
                alert_cfg->module = string_strdupz(value);
                string_freez(rc->module_match);
                simple_pattern_free(rc->module_pattern);

                rc->module_match = string_strdupz(value);
                rc->module_pattern = simple_pattern_create(rrdcalc_module_match(rc), NULL, SIMPLE_PATTERN_EXACT, true);
            }
            else {
                error("Health configuration at line %zu of file '%s' for alarm '%s' has unknown key '%s'.",
                        line, filename, rrdcalc_name(rc), key);
            }
        }
        else if(rt) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                alert_cfg->on = string_strdupz(value);
                if(rt->context) {
                    if(strcmp(string2str(rt->context), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalctemplate_name(rt), key, string2str(rt->context), value, value);

                    string_freez(rt->context);
                }
                rt->context = string_strdupz(value);
            }
            else if(hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY)) {
                strip_quotes(value);

                alert_cfg->classification = string_strdupz(value);
                if(rt->classification) {
                    if(strcmp(rrdcalctemplate_classification(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_classification(rt), value, value);

                    string_freez(rt->classification);
                }
                rt->classification = string_strdupz(value);
            }
            else if(hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY)) {
                strip_quotes(value);

                alert_cfg->component = string_strdupz(value);
                if(rt->component) {
                    if(strcmp(rrdcalctemplate_component(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_component(rt), value, value);

                    string_freez(rt->component);
                }
                rt->component = string_strdupz(value);
            }
            else if(hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY)) {
                strip_quotes(value);

                alert_cfg->type = string_strdupz(value);
                if(rt->type) {
                    if(strcmp(rrdcalctemplate_type(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_type(rt), value, value);

                    string_freez(rt->type);
                }
                rt->type = string_strdupz(value);
            }
            else if(hash == hash_families && !strcasecmp(key, HEALTH_FAMILIES_KEY)) {
                alert_cfg->families = string_strdupz(value);
                string_freez(rt->family_match);
                simple_pattern_free(rt->family_pattern);

                rt->family_match = string_strdupz(value);
                rt->family_pattern = simple_pattern_create(rrdcalctemplate_family_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                           true);
            }
            else if(hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY)) {
                alert_cfg->plugin = string_strdupz(value);
                string_freez(rt->plugin_match);
                simple_pattern_free(rt->plugin_pattern);

                rt->plugin_match = string_strdupz(value);
                rt->plugin_pattern = simple_pattern_create(rrdcalctemplate_plugin_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                           true);
            }
            else if(hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY)) {
                alert_cfg->module = string_strdupz(value);
                string_freez(rt->module_match);
                simple_pattern_free(rt->module_pattern);

                rt->module_match = string_strdupz(value);
                rt->module_pattern = simple_pattern_create(rrdcalctemplate_module_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                           true);
            }
            else if(hash == hash_charts && !strcasecmp(key, HEALTH_CHARTS_KEY)) {
                alert_cfg->charts = string_strdupz(value);
                string_freez(rt->charts_match);
                simple_pattern_free(rt->charts_pattern);

                rt->charts_match = string_strdupz(value);
                rt->charts_pattern = simple_pattern_create(rrdcalctemplate_charts_match(rt), NULL, SIMPLE_PATTERN_EXACT,
                                                           true);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                alert_cfg->lookup = string_strdupz(value);
                health_parse_db_lookup(line, filename, value, &rt->group, &rt->after, &rt->before,
                        &rt->update_every, &rt->options, &rt->dimensions, &rt->foreach_dimension);

                if(rt->foreach_dimension)
                    rt->foreach_dimension_pattern = health_pattern_from_foreach(rrdcalctemplate_foreachdim(rt));

                if (rt->after) {
                    if (rt->dimensions)
                        alert_cfg->p_db_lookup_dimensions = string_dup(rt->dimensions);

                    if (rt->group)
                        alert_cfg->p_db_lookup_method = string_strdupz(time_grouping_method2string(rt->group));

                    alert_cfg->p_db_lookup_options = rt->options;
                    alert_cfg->p_db_lookup_after = rt->after;
                    alert_cfg->p_db_lookup_before = rt->before;
                    alert_cfg->p_update_every = rt->update_every;
                }
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                alert_cfg->every = string_strdupz(value);
                if(!config_parse_duration(value, &rt->update_every))
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' cannot parse duration: '%s'.",
                            line, filename, rrdcalctemplate_name(rt), key, value);
                alert_cfg->p_update_every = rt->update_every;
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                alert_cfg->green = string_strdupz(value);
                char *e;
                rt->green = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rrdcalctemplate_name(rt), key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                alert_cfg->red = string_strdupz(value);
                char *e;
                rt->red = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rrdcalctemplate_name(rt), key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                alert_cfg->calc = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->calculation = expression_parse(value, &failed_at, &error);
                if(!rt->calculation) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalctemplate_name(rt), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                alert_cfg->warn = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->warning = expression_parse(value, &failed_at, &error);
                if(!rt->warning) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalctemplate_name(rt), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                alert_cfg->crit = string_strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->critical = expression_parse(value, &failed_at, &error);
                if(!rt->critical) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rrdcalctemplate_name(rt), key, value, expression_strerror(error), failed_at);
                }
                parse_variables_and_store_in_health_rrdvars(value, HEALTH_CONF_MAX_LINE);
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                alert_cfg->exec = string_strdupz(value);
                if(rt->exec) {
                    if(strcmp(rrdcalctemplate_exec(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_exec(rt), value, value);

                    string_freez(rt->exec);
                }
                rt->exec = string_strdupz(value);
            }
            else if(hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY)) {
                alert_cfg->to = string_strdupz(value);
                if(rt->recipient) {
                    if(strcmp(rrdcalctemplate_recipient(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_recipient(rt), value, value);

                    string_freez(rt->recipient);
                }
                rt->recipient = string_strdupz(value);
            }
            else if(hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY)) {
                strip_quotes(value);

                alert_cfg->units = string_strdupz(value);
                if(rt->units) {
                    if(strcmp(rrdcalctemplate_units(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_units(rt), value, value);

                    string_freez(rt->units);
                }
                rt->units = string_strdupz(value);
            }
            else if(hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY)) {
                strip_quotes(value);

                alert_cfg->info = string_strdupz(value);
                if(rt->info) {
                    if(strcmp(rrdcalctemplate_info(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_info(rt), value, value);

                    string_freez(rt->info);
                }
                rt->info = string_strdupz(value);
            }
            else if(hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY)) {
                alert_cfg->delay = string_strdupz(value);
                health_parse_delay(line, filename, value, &rt->delay_up_duration, &rt->delay_down_duration, &rt->delay_max_duration, &rt->delay_multiplier);
            }
            else if(hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY)) {
                alert_cfg->options = string_strdupz(value);
                rt->options |= health_parse_options(value);
            }
            else if(hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY)){
                alert_cfg->repeat = string_strdupz(value);
                health_parse_repeat(line, filename, value,
                                    &rt->warn_repeat_every,
                                    &rt->crit_repeat_every);
            }
            else if(hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY)) {
                alert_cfg->host_labels = string_strdupz(value);
                if(rt->host_labels) {
                    if(strcmp(rrdcalctemplate_host_labels(rt), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rrdcalctemplate_name(rt), key, rrdcalctemplate_host_labels(rt), value, value);

                    string_freez(rt->host_labels);
                    simple_pattern_free(rt->host_labels_pattern);
                }

                {
                    char *tmp = simple_pattern_trim_around_equal(value);
                    rt->host_labels = string_strdupz(tmp);
                    freez(tmp);
                }
                rt->host_labels_pattern = simple_pattern_create(rrdcalctemplate_host_labels(rt), NULL,
                                                                SIMPLE_PATTERN_EXACT, true);
            }
            else {
                error("Health configuration at line %zu of file '%s' for template '%s' has unknown key '%s'.",
                        line, filename, rrdcalctemplate_name(rt), key);
            }
        }
        else {
            error("Health configuration at line %zu of file '%s' has unknown key '%s'. Expected either '" HEALTH_ALARM_KEY "' or '" HEALTH_TEMPLATE_KEY "'.",
                    line, filename, key);
        }
    }

    if(rc) {
        //health_add_alarms_loop(host, rc, ignore_this) ;
        if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
            rrdcalc_free_unused_rrdcalc_loaded_from_config(rc);
        else
            rrdcalc_add_from_config(host, rc);
    }

    if(rt) {
        if(!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this)
            rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(rt);
        else
            rrdcalctemplate_add_from_config(host, rt);
    }

    if (alert_cfg)
        alert_config_free(alert_cfg);

    fclose(fp);
    return 1;
}

void health_yaml_config_parse_node(yaml_document_t *document_p, yaml_node_t *node, struct alert_config *doc_cfg, struct alert_config *sec_cfg, yaml_node_t *key, RRDHOST *host)
{
    static int working_config = 0;
	yaml_node_t *next_node_p, *key_node_p;

    switch (node->type) {
		case YAML_NO_NODE:
			break;
		case YAML_SCALAR_NODE:
            if (!strcmp((char *)key->data.scalar.value, "template") || !strcmp((char *)key->data.scalar.value, "alert")) { //change this if to be if in section alerts:
                if (working_config == 1) {
                    health_create_alert_from_config(doc_cfg, sec_cfg, host, "todo", 1);
                    alert_config_free(sec_cfg);
                } else {
                    working_config = 1;
                }
            }
            if (working_config == 1)
                health_config_store_key(sec_cfg, (const char *)key->data.scalar.value, (const char *)node->data.scalar.value);
            else
                health_config_store_key(doc_cfg, (const char *)key->data.scalar.value, (const char *)node->data.scalar.value);
			break;
		case YAML_SEQUENCE_NODE:
			for (yaml_node_item_t *i_node = node->data.sequence.items.start; i_node < node->data.sequence.items.top; i_node++) {
				next_node_p = yaml_document_get_node(document_p, *i_node);
				if (next_node_p)
					health_yaml_config_parse_node(document_p, next_node_p, doc_cfg, sec_cfg, key, host);
			}
			break;
		case YAML_MAPPING_NODE:
			for (yaml_node_pair_t *i_node_p = node->data.mapping.pairs.start; i_node_p < node->data.mapping.pairs.top; i_node_p++) {
				key_node_p = yaml_document_get_node(document_p, i_node_p->key);
				next_node_p = yaml_document_get_node(document_p, i_node_p->value);
                health_yaml_config_parse_node(document_p, next_node_p, doc_cfg, sec_cfg, key_node_p, host);
			}
			break;
		default:
			break;
	}
}

void health_yaml_config_handle_document(yaml_document_t *document_p, RRDHOST *host)
{
    yaml_node_t *node = yaml_document_get_root_node(document_p);

    struct alert_config *doc_alert_cfg = NULL;
    struct alert_config *sec_alert_cfg = NULL;
    doc_alert_cfg = callocz(1, sizeof(struct alert_config));
    sec_alert_cfg = callocz(1, sizeof(struct alert_config));

	health_yaml_config_parse_node(document_p, node, doc_alert_cfg, sec_alert_cfg, NULL, host);
    health_create_alert_from_config(doc_alert_cfg, sec_alert_cfg, host, "todo", 1);
    alert_config_free(sec_alert_cfg);
    alert_config_free(doc_alert_cfg);
    freez(sec_alert_cfg);
    freez(doc_alert_cfg);
}

static int health_readfile(const char *filename, void *data) {
    RRDHOST *host = (RRDHOST *)data;

    yaml_parser_t parser;
    yaml_document_t document;
    FILE *fp;
    int yaml_error = 0;

    fp = fopen(filename, "r");
    yaml_parser_initialize(&parser);
    yaml_parser_set_input_file(&parser, fp);

    int done = 0;
	while (!done)
	{
		if (!yaml_parser_load(&parser, &document)) {
			yaml_error = 1;
			break;
		}

		done = (!yaml_document_get_root_node(&document));

		if (!done)
			health_yaml_config_handle_document(&document, host);

		yaml_document_delete(&document);
	}

	yaml_parser_delete(&parser);

    if (yaml_error == 1) {
        fclose(fp);
        health_legacy_readfile(filename, host);
    }
    return 1;
}

void sql_refresh_hashes(void)
{
    sql_store_hashes = 1;
}

void health_readdir(RRDHOST *host, const char *user_path, const char *stock_path, const char *subpath) {
    if(unlikely((!host->health.health_enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH)) ||
        !service_running(SERVICE_HEALTH)) {
        debug(D_HEALTH, "CONFIG health is not enabled for host '%s'", rrdhost_hostname(host));
        return;
    }

    int stock_enabled = (int)config_get_boolean(CONFIG_SECTION_HEALTH, "enable stock health configuration",
                                                CONFIG_BOOLEAN_YES);

    if (!stock_enabled) {
        log_health("[%s]: Netdata will not load stock alarms.", rrdhost_hostname(host));
        stock_path = user_path;
    }

    if (!health_rrdvars)
        health_rrdvars = health_rrdvariables_create();

    recursive_config_double_dir_load(user_path, stock_path, subpath, health_readfile, (void *) host, 0);
    log_health("[%s]: Read health configuration.", rrdhost_hostname(host));
    sql_store_hashes = 0;
}
