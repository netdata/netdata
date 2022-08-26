// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"

#define HEALTH_CONF_MAX_LINE 4096

#define HEALTH_ALARM_KEY "alarm"
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

static inline int rrdcalc_add_alarm_from_config(RRDHOST *host, RRDCALC *rc) {
    if(!rc->chart) {
        error("Health configuration for alarm '%s' does not have a chart", rc->name);
        return 0;
    }

    if(!rc->update_every) {
        error("Health configuration for alarm '%s.%s' has no frequency (parameter 'every'). Ignoring it.", rrdcalc_chart_name(rc), rc->name);
        return 0;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(rc) && !rc->calculation && !rc->warning && !rc->critical) {
        error("Health configuration for alarm '%s.%s' is useless (no db lookup, no calculation, no warning and no critical expressions)", rrdcalc_chart_name(rc), rc->name);
        return 0;
    }

    if (rrdcalc_exists(host, rrdcalc_chart_name(rc), rc->name, rc->hash_chart, rc->hash))
        return 0;

    rc->id = rrdcalc_get_unique_id(host, rrdcalc_chart_name(rc), rc->name, &rc->next_event_id);

    debug(D_HEALTH, "Health configuration adding alarm '%s.%s' (%u): exec '%s', recipient '%s', green " NETDATA_DOUBLE_FORMAT_AUTO
        ", red " NETDATA_DOUBLE_FORMAT_AUTO
        ", lookup: group %d, after %d, before %d, options %u, dimensions '%s', for each dimension '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s', delay up %d, delay down %d, delay max %d, delay_multiplier %f, warn_repeat_every %u, crit_repeat_every %u",
            rrdcalc_chart_name(rc),
            rc->name,
            rc->id,
            (rc->exec)?rc->exec:"DEFAULT",
            (rc->recipient)?rc->recipient:"DEFAULT",
            rc->green,
            rc->red,
            (int)rc->group,
            rc->after,
            rc->before,
            rc->options,
            (rc->dimensions)?rc->dimensions:"NONE",
            (rc->foreachdim)?rc->foreachdim:"NONE",
            rc->update_every,
            (rc->calculation)?rc->calculation->parsed_as:"NONE",
            (rc->warning)?rc->warning->parsed_as:"NONE",
            (rc->critical)?rc->critical->parsed_as:"NONE",
            rc->source,
            rc->delay_up_duration,
            rc->delay_down_duration,
            rc->delay_max_duration,
            rc->delay_multiplier,
            rc->warn_repeat_every,
            rc->crit_repeat_every
    );

    rrdcalc_add_to_host(host, rc);

    return 1;
}

static inline int rrdcalctemplate_add_template_from_config(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt->context)) {
        error("Health configuration for template '%s' does not have a context", rt->name);
        return 0;
    }

    if(unlikely(!rt->update_every)) {
        error("Health configuration for template '%s' has no frequency (parameter 'every'). Ignoring it.", rt->name);
        return 0;
    }

    if(unlikely(!RRDCALCTEMPLATE_HAS_DB_LOOKUP(rt) && !rt->calculation && !rt->warning && !rt->critical)) {
        error("Health configuration for template '%s' is useless (no calculation, no warning and no critical evaluation)", rt->name);
        return 0;
    }

    RRDCALCTEMPLATE *t, *last = NULL;
    if(!rt->foreachdim) {
        for (t = host->templates; t ; last = t, t = t->next) {
            if(unlikely(t->hash_name == rt->hash_name
                        && !strcmp(t->name, rt->name)
                        && !strcmp(t->family_match?t->family_match:"*", rt->family_match?rt->family_match:"*")
            )) {
                info("Health configuration template '%s' already exists for host '%s'.", rt->name, host->hostname);
                return 0;
            }
        }

        if(likely(last)) {
            last->next = rt;
        }
        else {
            rt->next = host->templates;
            host->templates = rt;
        }
    } else {
        for (t = host->alarms_template_with_foreach; t ; last = t, t = t->next) {
            if(unlikely(t->hash_name == rt->hash_name
                        && !strcmp(t->name, rt->name)
                        && !strcmp(t->family_match?t->family_match:"*", rt->family_match?rt->family_match:"*")
            )) {
                info("Health configuration template '%s' already exists for host '%s'.", rt->name, host->hostname);
                return 0;
            }
        }

        if(likely(last)) {
            last->next = rt;
        }
        else {
            rt->next = host->alarms_template_with_foreach;
            host->alarms_template_with_foreach = rt;
        }
    }

    debug(D_HEALTH, "Health configuration adding template '%s': context '%s', exec '%s', recipient '%s', green " NETDATA_DOUBLE_FORMAT_AUTO
        ", red " NETDATA_DOUBLE_FORMAT_AUTO
        ", lookup: group %d, after %d, before %d, options %u, dimensions '%s', for each dimension '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s', delay up %d, delay down %d, delay max %d, delay_multiplier %f, warn_repeat_every %u, crit_repeat_every %u",
          rt->name,
          (rt->context)?string2str(rt->context):"NONE",
          (rt->exec)?rt->exec:"DEFAULT",
          (rt->recipient)?rt->recipient:"DEFAULT",
          rt->green,
          rt->red,
          (int)rt->group,
          rt->after,
          rt->before,
          rt->options,
          (rt->dimensions)?rt->dimensions:"NONE",
          (rt->foreachdim)?rt->foreachdim:"NONE",
          rt->update_every,
          (rt->calculation)?rt->calculation->parsed_as:"NONE",
          (rt->warning)?rt->warning->parsed_as:"NONE",
          (rt->critical)?rt->critical->parsed_as:"NONE",
          rt->source,
          rt->delay_up_duration,
          rt->delay_down_duration,
          rt->delay_max_duration,
          rt->delay_multiplier,
          rt->warn_repeat_every,
          rt->crit_repeat_every
    );


    return 1;
}

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
                options |= RRDCALC_FLAG_NO_CLEAR_NOTIFICATION;
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

/**
 * Health pattern from Foreach
 *
 * Create a new simple pattern using the user input
 *
 * @param s the string that will be used to create the simple pattern.
 */
SIMPLE_PATTERN *health_pattern_from_foreach(char *s) {
    char *convert= strdupz(s);
    SIMPLE_PATTERN *val = NULL;
    if(convert) {
        dimension_remove_pipe_comma(convert);
        val = simple_pattern_create(convert, NULL, SIMPLE_PATTERN_EXACT);

        freez(convert);
    }

    return val;
}

static inline int health_parse_db_lookup(
        size_t line, const char *filename, char *string,
        RRDR_GROUPING *group_method, int *after, int *before, int *every,
        uint32_t *options, char **dimensions, char **foreachdim
) {
    debug(D_HEALTH, "Health configuration parsing database lookup %zu@%s: %s", line, filename, string);

    if(*dimensions) freez(*dimensions);
    if(*foreachdim) freez(*foreachdim);
    *dimensions = NULL;
    *foreachdim = NULL;
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
        error("Health configuration invalid chart calculation at line %zu of file '%s': expected group method followed by the 'after' time, but got '%s'",
                line, filename, key);
        return 0;
    }

    if((*group_method = web_client_api_request_v1_data_group(key, RRDR_GROUPING_UNDEFINED)) == RRDR_GROUPING_UNDEFINED) {
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
                *dimensions = strdupz(s);
            }

            if(!find) {
                break;
            }
            s = ++find;
        }
        else if(!strcasecmp(key, HEALTH_FOREACH_KEY )) {
            *foreachdim = strdupz(s);
            break;
        }
        else {
            error("Health configuration at line %zu of file '%s': unknown keyword '%s'",
                    line, filename, key);
        }
    }

    return 1;
}

static inline char *health_source_file(size_t line, const char *file) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%zu@%s", line, file);
    return strdupz(buffer);
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
            localhost->registry_hostname);
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
    freez(cfg->alarm);
    freez(cfg->template_key);
    freez(cfg->os);
    freez(cfg->host);
    freez(cfg->on);
    freez(cfg->families);
    freez(cfg->plugin);
    freez(cfg->module);
    freez(cfg->charts);
    freez(cfg->lookup);
    freez(cfg->calc);
    freez(cfg->warn);
    freez(cfg->crit);
    freez(cfg->every);
    freez(cfg->green);
    freez(cfg->red);
    freez(cfg->exec);
    freez(cfg->to);
    freez(cfg->units);
    freez(cfg->info);
    freez(cfg->classification);
    freez(cfg->component);
    freez(cfg->type);
    freez(cfg->delay);
    freez(cfg->options);
    freez(cfg->repeat);
    freez(cfg->host_labels);
    freez(cfg->p_db_lookup_dimensions);
    freez(cfg->p_db_lookup_method);
    freez(cfg);
}

int sql_store_hashes = 1;
static int health_readfile(const char *filename, void *data) {
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
                if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalc_add_alarm_from_config(host, rc)) {
                    rrdcalc_free(rc);
                }
               // health_add_alarms_loop(host, rc, ignore_this) ;
            }

            if(rt) {
                if (!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalctemplate_add_template_from_config(host, rt)) {
                    rrdcalctemplate_free(rt);
                }
                rt = NULL;
            }

            rc = callocz(1, sizeof(RRDCALC));
            rc->next_event_id = 1;
            rc->name = strdupz(value);
            rc->source = health_source_file(line, filename);
            rc->green = NAN;
            rc->red = NAN;
            rc->value = NAN;
            rc->old_value = NAN;
            rc->delay_multiplier = 1.0;
            rc->old_status = RRDCALC_STATUS_UNINITIALIZED;
            rc->warn_repeat_every = host->health_default_warn_repeat_every;
            rc->crit_repeat_every = host->health_default_crit_repeat_every;
            if (alert_cfg)
                alert_config_free(alert_cfg);
            alert_cfg = callocz(1, sizeof(struct alert_config));

            if(rrdvar_fix_name(rc->name))
                error("Health configuration renamed alarm '%s' to '%s'", value, rc->name);

            rc->hash = simple_hash(rc->name);
            alert_cfg->alarm = strdupz(rc->name);
            ignore_this = 0;
        }
        else if(hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY)) {
            if(rc) {
//                health_add_alarms_loop(host, rc, ignore_this) ;
                if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalc_add_alarm_from_config(host, rc)) {
                    rrdcalc_free(rc);
                }

                rc = NULL;
            }

            if(rt) {
                if(!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalctemplate_add_template_from_config(host, rt)) {
                    rrdcalctemplate_free(rt);
                }
            }

            rt = callocz(1, sizeof(RRDCALCTEMPLATE));
            rt->name = strdupz(value);
            rt->source = health_source_file(line, filename);
            rt->green = NAN;
            rt->red = NAN;
            rt->delay_multiplier = 1.0;
            rt->warn_repeat_every = host->health_default_warn_repeat_every;
            rt->crit_repeat_every = host->health_default_crit_repeat_every;
            if (alert_cfg)
                alert_config_free(alert_cfg);
            alert_cfg = callocz(1, sizeof(struct alert_config));

            if(rrdvar_fix_name(rt->name))
                error("Health configuration renamed template '%s' to '%s'", value, rt->name);

            rt->hash_name = simple_hash(rt->name);
            alert_cfg->template_key = strdupz(rt->name);
            ignore_this = 0;
        }
        else if(hash == hash_os && !strcasecmp(key, HEALTH_OS_KEY)) {
            char *os_match = value;
            if (alert_cfg) alert_cfg->os = strdupz(value);
            SIMPLE_PATTERN *os_pattern = simple_pattern_create(os_match, NULL, SIMPLE_PATTERN_EXACT);

            if(!simple_pattern_matches(os_pattern, host->os)) {
                if(rc)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: host O/S does not match '%s'", host->hostname, rc->name, line, filename, os_match);

                if(rt)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: host O/S does not match '%s'", host->hostname, rt->name, line, filename, os_match);

                ignore_this = 1;
            }

            simple_pattern_free(os_pattern);
        }
        else if(hash == hash_host && !strcasecmp(key, HEALTH_HOST_KEY)) {
            char *host_match = value;
            if (alert_cfg) alert_cfg->host = strdupz(value);
            SIMPLE_PATTERN *host_pattern = simple_pattern_create(host_match, NULL, SIMPLE_PATTERN_EXACT);

            if(!simple_pattern_matches(host_pattern, host->hostname)) {
                if(rc)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring alarm '%s' defined at %zu@%s: hostname does not match '%s'", host->hostname, rc->name, line, filename, host_match);

                if(rt)
                    debug(D_HEALTH, "HEALTH on '%s' ignoring template '%s' defined at %zu@%s: hostname does not match '%s'", host->hostname, rt->name, line, filename, host_match);

                ignore_this = 1;
            }

            simple_pattern_free(host_pattern);
        }
        else if(rc) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                alert_cfg->on = strdupz(value);
                if(rc->chart) {
                    if(strcmp(rrdcalc_chart_name(rc), value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rrdcalc_chart_name(rc), value, value);

                    string_freez(rc->chart);
                }
                rc->chart = string_strdupz(value);
                rc->hash_chart = simple_hash(rrdcalc_chart_name(rc));
            }
            else if(hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY)) {
                alert_cfg->classification = strdupz(value);
                if(rc->classification) {
                    if(strcmp(rc->classification, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->classification, value, value);

                    freez(rc->classification);
                }
                rc->classification = strdupz(value);
                strip_quotes(rc->classification);
            }
            else if(hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY)) {
                alert_cfg->component = strdupz(value);
                if(rc->component) {
                    if(strcmp(rc->component, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->component, value, value);

                    freez(rc->component);
                }
                rc->component = strdupz(value);
                strip_quotes(rc->component);
            }
            else if(hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY)) {
                alert_cfg->type = strdupz(value);
                if(rc->type) {
                    if(strcmp(rc->type, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->type, value, value);

                    freez(rc->type);
                }
                rc->type = strdupz(value);
                strip_quotes(rc->type);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                alert_cfg->lookup = strdupz(value);
                health_parse_db_lookup(line, filename, value, &rc->group, &rc->after, &rc->before,
                        &rc->update_every, &rc->options, &rc->dimensions, &rc->foreachdim);
                if(rc->foreachdim) {
                    rc->spdim = health_pattern_from_foreach(rc->foreachdim);
                }
                if (rc->after) {
                    if (rc->dimensions)
                        alert_cfg->p_db_lookup_dimensions = strdupz(rc->dimensions);
                    if (rc->group)
                        alert_cfg->p_db_lookup_method = strdupz(group_method2string(rc->group));
                    alert_cfg->p_db_lookup_options = rc->options;
                    alert_cfg->p_db_lookup_after = rc->after;
                    alert_cfg->p_db_lookup_before = rc->before;
                    alert_cfg->p_update_every = rc->update_every;
                }
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                alert_cfg->every = strdupz(value);
                if(!config_parse_duration(value, &rc->update_every))
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' cannot parse duration: '%s'.",
                            line, filename, rc->name, key, value);
                alert_cfg->p_update_every = rc->update_every;
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                alert_cfg->green = strdupz(value);
                char *e;
                rc->green = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rc->name, key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                alert_cfg->red = strdupz(value);
                char *e;
                rc->red = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rc->name, key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                alert_cfg->calc = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->calculation = expression_parse(value, &failed_at, &error);
                if(!rc->calculation) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                alert_cfg->warn = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->warning = expression_parse(value, &failed_at, &error);
                if(!rc->warning) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                alert_cfg->crit = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rc->critical = expression_parse(value, &failed_at, &error);
                if(!rc->critical) {
                    error("Health configuration at line %zu of file '%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                alert_cfg->exec = strdupz(value);
                if(rc->exec) {
                    if(strcmp(rc->exec, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->exec, value, value);

                    freez(rc->exec);
                }
                rc->exec = strdupz(value);
            }
            else if(hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY)) {
                alert_cfg->to = strdupz(value);
                if(rc->recipient) {
                    if(strcmp(rc->recipient, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->recipient, value, value);

                    freez(rc->recipient);
                }
                rc->recipient = strdupz(value);
            }
            else if(hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY)) {
                alert_cfg->units = strdupz(value);
                if(rc->units) {
                    if(strcmp(rc->units, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->units, value, value);

                    freez(rc->units);
                }
                rc->units = strdupz(value);
                strip_quotes(rc->units);
            }
            else if(hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY)) {
                alert_cfg->info = strdupz(value);
                if(rc->info) {
                    if(strcmp(rc->info, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rc->name, key, rc->info, value, value);

                    freez(rc->info);
                }
                rc->info = strdupz(value);
                strip_quotes(rc->info);
                rc->original_info = strdupz(value);
                strip_quotes(rc->original_info);
            }
            else if(hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY)) {
                alert_cfg->delay = strdupz(value);
                health_parse_delay(line, filename, value, &rc->delay_up_duration, &rc->delay_down_duration, &rc->delay_max_duration, &rc->delay_multiplier);
            }
            else if(hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY)) {
                alert_cfg->options = strdupz(value);
                rc->options |= health_parse_options(value);
            }
            else if(hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY)){
                alert_cfg->repeat = strdupz(value);
                health_parse_repeat(line, filename, value,
                                    &rc->warn_repeat_every,
                                    &rc->crit_repeat_every);
            }
            else if(hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY)) {
                alert_cfg->host_labels = strdupz(value);
                if(rc->host_labels) {
                    if(strcmp(rc->host_labels, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'.",
                              line, filename, rc->name, key, value, value);

                    freez(rc->host_labels);
                    simple_pattern_free(rc->host_labels_pattern);
                }

                rc->host_labels = simple_pattern_trim_around_equal(value);
                rc->host_labels_pattern = simple_pattern_create(rc->host_labels, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY)) {
                alert_cfg->plugin = strdupz(value);
                freez(rc->plugin_match);
                simple_pattern_free(rc->plugin_pattern);

                rc->plugin_match = strdupz(value);
                rc->plugin_pattern = simple_pattern_create(rc->plugin_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY)) {
                alert_cfg->module = strdupz(value);
                freez(rc->module_match);
                simple_pattern_free(rc->module_pattern);

                rc->module_match = strdupz(value);
                rc->module_pattern = simple_pattern_create(rc->module_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else {
                error("Health configuration at line %zu of file '%s' for alarm '%s' has unknown key '%s'.",
                        line, filename, rc->name, key);
            }
        }
        else if(rt) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                alert_cfg->on = strdupz(value);
                if(rt->context) {
                    if(strcmp(string2str(rt->context), value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rt->name, key, string2str(rt->context), value, value);

                    string_freez(rt->context);
                }
                rt->context = string_strdupz(value);
            }
            else if(hash == hash_class && !strcasecmp(key, HEALTH_CLASS_KEY)) {
                alert_cfg->classification = strdupz(value);
                if(rt->classification) {
                    if(strcmp(rt->classification, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rt->name, key, rt->classification, value, value);

                    freez(rt->classification);
                }
                rt->classification = strdupz(value);
                strip_quotes(rt->classification);
            }
            else if(hash == hash_component && !strcasecmp(key, HEALTH_COMPONENT_KEY)) {
                alert_cfg->component = strdupz(value);
                if(rt->component) {
                    if(strcmp(rt->component, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rt->name, key, rt->component, value, value);

                    freez(rt->component);
                }
                rt->component = strdupz(value);
                strip_quotes(rt->component);
            }
            else if(hash == hash_type && !strcasecmp(key, HEALTH_TYPE_KEY)) {
                alert_cfg->type = strdupz(value);
                if(rt->type) {
                    if(strcmp(rt->type, value) != 0)
                        error("Health configuration at line %zu of file '%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rt->name, key, rt->type, value, value);

                    freez(rt->type);
                }
                rt->type = strdupz(value);
                strip_quotes(rt->type);
            }
            else if(hash == hash_families && !strcasecmp(key, HEALTH_FAMILIES_KEY)) {
                alert_cfg->families = strdupz(value);
                freez(rt->family_match);
                simple_pattern_free(rt->family_pattern);

                rt->family_match = strdupz(value);
                rt->family_pattern = simple_pattern_create(rt->family_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_plugin && !strcasecmp(key, HEALTH_PLUGIN_KEY)) {
                alert_cfg->plugin = strdupz(value);
                freez(rt->plugin_match);
                simple_pattern_free(rt->plugin_pattern);

                rt->plugin_match = strdupz(value);
                rt->plugin_pattern = simple_pattern_create(rt->plugin_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_module && !strcasecmp(key, HEALTH_MODULE_KEY)) {
                alert_cfg->module = strdupz(value);
                freez(rt->module_match);
                simple_pattern_free(rt->module_pattern);

                rt->module_match = strdupz(value);
                rt->module_pattern = simple_pattern_create(rt->module_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_charts && !strcasecmp(key, HEALTH_CHARTS_KEY)) {
                alert_cfg->charts = strdupz(value);
                freez(rt->charts_match);
                simple_pattern_free(rt->charts_pattern);

                rt->charts_match = strdupz(value);
                rt->charts_pattern = simple_pattern_create(rt->charts_match, NULL, SIMPLE_PATTERN_EXACT);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                alert_cfg->lookup = strdupz(value);
                health_parse_db_lookup(line, filename, value, &rt->group, &rt->after, &rt->before,
                        &rt->update_every, &rt->options, &rt->dimensions, &rt->foreachdim);
                if(rt->foreachdim) {
                    rt->spdim = health_pattern_from_foreach(rt->foreachdim);
                }
                if (rt->after) {
                    if (rt->dimensions)
                        alert_cfg->p_db_lookup_dimensions = strdupz(rt->dimensions);
                    if (rt->group)
                        alert_cfg->p_db_lookup_method = strdupz(group_method2string(rt->group));
                    alert_cfg->p_db_lookup_options = rt->options;
                    alert_cfg->p_db_lookup_after = rt->after;
                    alert_cfg->p_db_lookup_before = rt->before;
                    alert_cfg->p_update_every = rt->update_every;
                }
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                alert_cfg->every = strdupz(value);
                if(!config_parse_duration(value, &rt->update_every))
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' cannot parse duration: '%s'.",
                            line, filename, rt->name, key, value);
                alert_cfg->p_update_every = rt->update_every;
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                alert_cfg->green = strdupz(value);
                char *e;
                rt->green = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rt->name, key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                alert_cfg->red = strdupz(value);
                char *e;
                rt->red = str2ndd(value, &e);
                if(e && *e) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                            line, filename, rt->name, key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                alert_cfg->calc = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->calculation = expression_parse(value, &failed_at, &error);
                if(!rt->calculation) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                alert_cfg->warn = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->warning = expression_parse(value, &failed_at, &error);
                if(!rt->warning) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                alert_cfg->crit = strdupz(value);
                const char *failed_at = NULL;
                int error = 0;
                rt->critical = expression_parse(value, &failed_at, &error);
                if(!rt->critical) {
                    error("Health configuration at line %zu of file '%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                            line, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                alert_cfg->exec = strdupz(value);
                if(rt->exec) {
                    if(strcmp(rt->exec, value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rt->name, key, rt->exec, value, value);

                    freez(rt->exec);
                }
                rt->exec = strdupz(value);
            }
            else if(hash == hash_recipient && !strcasecmp(key, HEALTH_RECIPIENT_KEY)) {
                alert_cfg->to = strdupz(value);
                if(rt->recipient) {
                    if(strcmp(rt->recipient, value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rt->name, key, rt->recipient, value, value);

                    freez(rt->recipient);
                }
                rt->recipient = strdupz(value);
            }
            else if(hash == hash_units && !strcasecmp(key, HEALTH_UNITS_KEY)) {
                alert_cfg->units = strdupz(value);
                if(rt->units) {
                    if(strcmp(rt->units, value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rt->name, key, rt->units, value, value);

                    freez(rt->units);
                }
                rt->units = strdupz(value);
                strip_quotes(rt->units);
            }
            else if(hash == hash_info && !strcasecmp(key, HEALTH_INFO_KEY)) {
                alert_cfg->info = strdupz(value);
                if(rt->info) {
                    if(strcmp(rt->info, value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                                line, filename, rt->name, key, rt->info, value, value);

                    freez(rt->info);
                }
                rt->info = strdupz(value);
                strip_quotes(rt->info);
            }
            else if(hash == hash_delay && !strcasecmp(key, HEALTH_DELAY_KEY)) {
                alert_cfg->delay = strdupz(value);
                health_parse_delay(line, filename, value, &rt->delay_up_duration, &rt->delay_down_duration, &rt->delay_max_duration, &rt->delay_multiplier);
            }
            else if(hash == hash_options && !strcasecmp(key, HEALTH_OPTIONS_KEY)) {
                alert_cfg->options = strdupz(value);
                rt->options |= health_parse_options(value);
            }
            else if(hash == hash_repeat && !strcasecmp(key, HEALTH_REPEAT_KEY)){
                alert_cfg->repeat = strdupz(value);
                health_parse_repeat(line, filename, value,
                                    &rt->warn_repeat_every,
                                    &rt->crit_repeat_every);
            }
            else if(hash == hash_host_label && !strcasecmp(key, HEALTH_HOST_LABEL_KEY)) {
                alert_cfg->host_labels = strdupz(value);
                if(rt->host_labels) {
                    if(strcmp(rt->host_labels, value) != 0)
                        error("Health configuration at line %zu of file '%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                              line, filename, rt->name, key, rt->host_labels, value, value);

                    freez(rt->host_labels);
                    simple_pattern_free(rt->host_labels_pattern);
                }

                rt->host_labels = simple_pattern_trim_around_equal(value);
                rt->host_labels_pattern = simple_pattern_create(rt->host_labels, NULL, SIMPLE_PATTERN_EXACT);
            }
            else {
                error("Health configuration at line %zu of file '%s' for template '%s' has unknown key '%s'.",
                        line, filename, rt->name, key);
            }
        }
        else {
            error("Health configuration at line %zu of file '%s' has unknown key '%s'. Expected either '" HEALTH_ALARM_KEY "' or '" HEALTH_TEMPLATE_KEY "'.",
                    line, filename, key);
        }
    }

    if(rc) {
        //health_add_alarms_loop(host, rc, ignore_this) ;
        if(!alert_hash_and_store_config(rc->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalc_add_alarm_from_config(host, rc)) {
            rrdcalc_free(rc);
        }
    }

    if(rt) {
        if(!alert_hash_and_store_config(rt->config_hash_id, alert_cfg, sql_store_hashes) || ignore_this || !rrdcalctemplate_add_template_from_config(host, rt)) {
            rrdcalctemplate_free(rt);
        }
    }

    if (alert_cfg)
        alert_config_free(alert_cfg);

    fclose(fp);
    return 1;
}

void sql_refresh_hashes(void)
{
    sql_store_hashes = 1;
}

void health_readdir(RRDHOST *host, const char *user_path, const char *stock_path, const char *subpath) {
    if(unlikely(!host->health_enabled)) {
        debug(D_HEALTH, "CONFIG health is not enabled for host '%s'", host->hostname);
        return;
    }

    int stock_enabled = (int)config_get_boolean(CONFIG_SECTION_HEALTH, "enable stock health configuration",
                                                CONFIG_BOOLEAN_YES);

    if (!stock_enabled) {
        info("Netdata will not load stock alarms.");
        stock_path = user_path;
    }

    recursive_config_double_dir_load(user_path, stock_path, subpath, health_readfile, (void *) host, 0);
    sql_store_hashes = 0;
}
