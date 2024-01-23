// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

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
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                                  line, filename, value, key);
            }
            else given_up = 1;
        }
        else if(!strcasecmp(key, "down")) {
            if (!config_parse_duration(value, delay_down_duration)) {
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                                  line, filename, value, key);
            }
            else given_down = 1;
        }
        else if(!strcasecmp(key, "multiplier")) {
            *delay_multiplier = strtof(value, NULL);
            if(isnan(*delay_multiplier) || isinf(*delay_multiplier) || islessequal(*delay_multiplier, 0)) {
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                                  line, filename, value, key);
            }
            else given_multiplier = 1;
        }
        else if(!strcasecmp(key, "max")) {
            if (!config_parse_duration(value, delay_max_duration)) {
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                                  line, filename, value, key);
            }
            else given_max = 1;
        }
        else {
            netdata_log_error("Health configuration at line %zu of file '%s': unknown keyword '%s'",
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

static inline ALERT_ACTION_OPTIONS health_parse_options(const char *s) {
    ALERT_ACTION_OPTIONS options = ALERT_ACTION_OPTION_NONE;
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
                options |= ALERT_ACTION_OPTION_NO_CLEAR_NOTIFICATION;
            else
                netdata_log_error("Ignoring unknown alarm option '%s'", buf);
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
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
                                  line, file, value, key);
            }
        }
        else if(!strcasecmp(key, "critical")) {
            if (!config_parse_duration(value, (int*)crit_repeat_every)) {
                netdata_log_error("Health configuration at line %zu of file '%s': invalid value '%s' for '%s' keyword",
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

static inline int health_parse_db_lookup(
        size_t line, const char *filename, char *string,
        RRDR_TIME_GROUPING *group_method, int *after, int *before, int *every,
        RRDR_OPTIONS *options, STRING **dimensions
) {
    netdata_log_debug(D_HEALTH, "Health configuration parsing database lookup %zu@%s: %s", line, filename, string);

    if(*dimensions) string_freez(*dimensions);
    *dimensions = NULL;
    *after = 0;
    *before = 0;
    *every = 0;
    *options = 0;

    char *s = string, *key;

    // first is the group method
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';
    if(!*s) {
        netdata_log_error("Health configuration invalid chart calculation at line %zu of file '%s': expected group method followed by the 'after' time, but got '%s'",
                          line, filename, key);
        return 0;
    }

    if((*group_method = time_grouping_parse(key, RRDR_GROUPING_UNDEFINED)) == RRDR_GROUPING_UNDEFINED) {
        netdata_log_error("Health configuration at line %zu of file '%s': invalid group method '%s'",
                          line, filename, key);
        return 0;
    }

    // then is the 'after' time
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';

    if(!config_parse_duration(key, after)) {
        netdata_log_error("Health configuration at line %zu of file '%s': invalid duration '%s' after group method",
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
                netdata_log_error("Health configuration at line %zu of file '%s': invalid duration '%s' for '%s' keyword",
                                  line, filename, value, key);
            }
        }
        else if(!strcasecmp(key, HEALTH_EVERY_KEY)) {
            char *value = s;
            while(*s && !isspace(*s)) s++;
            while(*s && isspace(*s)) *s++ = '\0';

            if (!config_parse_duration(value, every)) {
                netdata_log_error("Health configuration at line %zu of file '%s': invalid duration '%s' for '%s' keyword",
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
        else {
            netdata_log_error("Health configuration at line %zu of file '%s': unknown keyword '%s'",
                              line, filename, key);
        }
    }

    return 1;
}

static inline STRING *health_source_file(size_t line, const char *file) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "line=%zu,file=%s", line, file);
    return string_strdupz(buffer);
}

char *health_edit_command_from_source(const char *source)
{
    char buffer[FILENAME_MAX + 1];
    char *temp = strdupz(source);
    char *line_num = strchr(temp, '@');
    char *line_p = temp;
    char *file_no_path = strrchr(temp, '/');

    // Check for the 'line=' format if '@' is not found
    if (!line_num) {
        line_num = strstr(temp, "line=");
        file_no_path = strstr(temp, "file=/");
    }

    if (likely(file_no_path && line_num)) {
        if (line_num == strchr(temp, '@')) {
            *line_num = '\0';  // Handle the old format
        } else {
            line_num += strlen("line=");
            file_no_path = strrchr(file_no_path + strlen("file="), '/');
            char *line_end = strchr(line_num, ',');
            if (line_end) {
                line_p = line_num;
                *line_end = '\0';
            }
        }

        snprintfz(
            buffer,
            FILENAME_MAX,
            "sudo %s/edit-config health.d/%s=%s=%s",
            netdata_configured_user_config_dir,
            file_no_path + 1,
            line_p,
            rrdhost_registry_hostname(localhost));
    } else {
        buffer[0] = '\0';
    }

    freez(temp);
    return strdupz(buffer);
}


static inline void strip_quotes(char *s) {
    while(*s) {
        if(*s == '\'' || *s == '"') *s = ' ';
        s++;
    }
}

#define PARSE_HEALTH_CONFIG_DUPLICATE_STRING_MSG(ax, member) do {                                   \
    if(strcmp(string2str(ax->member), value) != 0)                                                  \
        netdata_log_error(                                                                          \
            "Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, "     \
            "once with value '%s' and later with value '%s'. Using ('%s').",                        \
            line, filename, string2str(ac->name), key,                                              \
            string2str(ax->member), value, value);                                                  \
} while(0)

#define PARSE_HEALTH_CONFIG_LINE_STRING(ax, member) do {                                            \
    if(ax->member) {                                                                                \
        PARSE_HEALTH_CONFIG_DUPLICATE_STRING_MSG(ax, member);                                       \
        string_freez(ax->member);                                                                   \
    }                                                                                               \
    ax->member = string_strdupz(value);                                                             \
} while(0)

#define PARSE_HEALTH_CONFIG_LINE_PATTERN(ax, member) do {                                           \
    if(ax->member) {                                                                                \
        PARSE_HEALTH_CONFIG_DUPLICATE_STRING_MSG(ax, member);                                       \
        string_freez(ax->member);                                                                   \
    }                                                                                               \
    if(value && strcmp(value, "*") == 0)                                                            \
        value = NULL;                                                                               \
    else if(value && (strcmp(value, "!* *") == 0 || strcmp(value, "!*") == 0)) {                    \
        value = NULL;                                                                               \
        ap->match.enabled = false;                                                                  \
    }                                                                                               \
    ax->member = string_strdupz(value);                                                             \
} while(0)

int health_readfile(const char *filename, void *data __maybe_unused, bool stock_config) {
    netdata_log_debug(D_HEALTH, "Health configuration reading file '%s'", filename);

    static uint32_t
            hash_alarm = 0,
            hash_template = 0,
            hash_os = 0,
            hash_on = 0,
            hash_host = 0,
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
            hash_summary = 0,
            hash_info = 0,
            hash_class = 0,
            hash_component = 0,
            hash_type = 0,
            hash_recipient = 0,
            hash_delay = 0,
            hash_options = 0,
            hash_repeat = 0,
            hash_host_label = 0,
            hash_chart_label = 0;

    char buffer[HEALTH_CONF_MAX_LINE + 1];

    if(unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_on = simple_uhash(HEALTH_ON_KEY);
        hash_os = simple_uhash(HEALTH_OS_KEY);
        hash_host = simple_uhash(HEALTH_HOST_KEY);
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
        hash_summary = simple_hash(HEALTH_SUMMARY_KEY);
        hash_info = simple_hash(HEALTH_INFO_KEY);
        hash_class = simple_uhash(HEALTH_CLASS_KEY);
        hash_component = simple_uhash(HEALTH_COMPONENT_KEY);
        hash_type = simple_uhash(HEALTH_TYPE_KEY);
        hash_recipient = simple_hash(HEALTH_RECIPIENT_KEY);
        hash_delay = simple_uhash(HEALTH_DELAY_KEY);
        hash_options = simple_uhash(HEALTH_OPTIONS_KEY);
        hash_repeat = simple_uhash(HEALTH_REPEAT_KEY);
        hash_host_label = simple_uhash(HEALTH_HOST_LABEL_KEY);
        hash_chart_label = simple_uhash(HEALTH_CHART_LABEL_KEY);
    }

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        netdata_log_error("Health configuration cannot read file '%s'.", filename);
        return 0;
    }

    RRD_ALERT_PROTOTYPE *ap = NULL;
    struct rrd_alert_config *ac = NULL;
    struct rrd_alert_match *am = NULL;

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
            else
                netdata_log_error(
                    "Health configuration has too long multi-line at line %zu of file '%s'.",
                    line, filename);
        }
        append = 0;

        char *key = s;
        while(*s && *s != ':') s++;
        if(!*s) {
            netdata_log_error(
                "Health configuration has invalid line %zu of file '%s'. It does not contain a ':'. Ignoring it.",
                line, filename);
            continue;
        }
        *s = '\0';
        s++;

        char *value = s;
        key = trim_all(key);
        value = trim_all(value);

        if(!key) {
            netdata_log_error(
                "Health configuration has invalid line %zu of file '%s'. Keyword is empty. Ignoring it.",
                line, filename);

            continue;
        }

        if(!value) {
            netdata_log_error(
                "Health configuration has invalid line %zu of file '%s'. value is empty. Ignoring it.",
                line, filename);
            continue;
        }

        uint32_t hash = simple_uhash(key);

        if((hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) || (hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY))) {
            if(ap) {
                health_prototype_add(ap);
                freez(ap);
            }

            ap = callocz(1, sizeof(*ap));
            am = &ap->match;
            ac = &ap->config;

            {
                char *tmp = strdupz(value);
                if(rrdvar_fix_name(tmp))
                    netdata_log_error("Health configuration renamed alarm '%s' to '%s'", value, tmp);

                ap->config.name = string_strdupz(tmp);
                freez(tmp);
            }

            ap->_internal.enabled = true;
            ap->match.enabled = true;
            ap->match.is_template = (hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY));
            ap->config.source = health_source_file(line, filename);
            ap->config.source_type = stock_config ? DYNCFG_SOURCE_TYPE_STOCK : DYNCFG_SOURCE_TYPE_USER;
            ap->config.green = NAN;
            ap->config.red = NAN;
            ap->config.delay_multiplier = 1;
            ap->config.warn_repeat_every = health_globals.config.default_warn_repeat_every;
            ap->config.crit_repeat_every = health_globals.config.default_crit_repeat_every;
        }
        else if(!am || !ac || !ap) {
            netdata_log_error(
                "Health configuration at line %zu of file '%s' has unknown key '%s'. "
                "Expected either '" HEALTH_ALARM_KEY "' or '" HEALTH_TEMPLATE_KEY "'.",
                line, filename, key);
        }
        else if(!am->is_template && hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_STRING(am, on.chart);
        }
        else if(am->is_template && hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_STRING(am, on.context);
        }
        else if(am->is_template && hash == hash_charts && !strcasecmp(key, HEALTH_CHARTS_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, charts);
        }
        else if(hash == hash_os && !strcasecmp(key, HEALTH_OS_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, os);
        }
        else if(hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, host);
        }
        else if(hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, host_labels);
        }
        else if(hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, plugin);
        }
        else if(hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, module);
        }
        else if(hash == hash_chart_label && !strcasecmp(key, HEALTH_CHART_LABEL_KEY)) {
            PARSE_HEALTH_CONFIG_LINE_PATTERN(am, chart_labels);
        }
        else if(hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, classification);
        }
        else if(hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, component);
        }
        else if(hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, type);
        }
        else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
            ac->lookup = string_strdupz(value);
            health_parse_db_lookup(line, filename, value,
                                   &ac->group, &ac->after, &ac->before,
                                   &ac->update_every, &ac->options,
                                   &ac->dimensions);
        }
        else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
            if(!config_parse_duration(value, &ac->update_every))
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "cannot parse duration: '%s'.",
                    line, filename, string2str(ac->name), key, value);
        }
        else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
            char *e;
            ac->green = str2ndd(value, &e);
            if(e && *e) {
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "leaves this string unmatched: '%s'.",
                    line, filename, string2str(ac->name), key, e);
            }
        }
        else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
            char *e;
            ac->red = str2ndd(value, &e);
            if(e && *e) {
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "leaves this string unmatched: '%s'.",
                    line, filename, string2str(ac->name), key, e);
            }
        }
        else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
            const char *failed_at = NULL;
            int error = 0;
            ac->calculation = expression_parse(value, &failed_at, &error);
            if(!ac->calculation) {
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "has non-parseable expression '%s': %s at '%s'",
                    line, filename, string2str(ac->name), key, value, expression_strerror(error), failed_at);
                am->enabled = false;
            }
        }
        else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
            const char *failed_at = NULL;
            int error = 0;
            ac->warning = expression_parse(value, &failed_at, &error);
            if(!ac->warning) {
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "has non-parseable expression '%s': %s at '%s'",
                    line, filename, string2str(ac->name), key, value, expression_strerror(error), failed_at);
                am->enabled = false;
            }
        }
        else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
            const char *failed_at = NULL;
            int error = 0;
            ac->critical = expression_parse(value, &failed_at, &error);
            if(!ac->critical) {
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' "
                    "has non-parseable expression '%s': %s at '%s'",
                    line, filename, string2str(ac->name), key, value, expression_strerror(error), failed_at);
                am->enabled = false;
            }
        }
        else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, exec);
        }
        else if(hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, recipient);
        }
        else if(hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, units);
        }
        else if(hash == hash_summary && !strcasecmp(key, HEALTH_SUMMARY_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, summary);
        }
        else if(hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY)) {
            strip_quotes(value);
            PARSE_HEALTH_CONFIG_LINE_STRING(ac, info);
        }
        else if(hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY)) {
            health_parse_delay(line, filename, value,
                               &ac->delay_up_duration, &ac->delay_down_duration,
                               &ac->delay_max_duration, &ac->delay_multiplier);
        }
        else if(hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY)) {
            ac->alert_action_options |= health_parse_options(value);
        }
        else if(hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY)){
            health_parse_repeat(line, filename, value,
                                &ac->warn_repeat_every,
                                &ac->crit_repeat_every);
            ac->has_custom_repeat_config = true;
        }
        else {
            if (strcmp(key, "families") != 0)
                netdata_log_error(
                    "Health configuration at line %zu of file '%s' for alarm/template '%s' has unknown key '%s'.",
                    line, filename, string2str(ac->name), key);
        }
    }

    if(ap) {
        health_prototype_add(ap);
        freez(ap);
    }

    fclose(fp);
    return 1;
}
