// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDCALCTEMPLATE management
/**
 * RRDCALC TEMPLATE LINK MATCHING
 *
 * @param rt is the template used to create the chart.
 * @param st is the chart where the alarm will be attached.
 */

static char *rrdcalc_alert_name_with_dimension(const char *name, size_t namelen, const char *dim, size_t dimlen) {
    char *newname,*move;

    newname = mallocz(namelen + dimlen + 2);
    move = newname;
    memcpy(move, name, namelen);
    move += namelen;

    *move++ = '_';
    memcpy(move, dim, dimlen);
    move += dimlen;
    *move = '\0';

    return newname;
}

bool rrdcalctemplate_check_rrdset_conditions(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host) {
    if(rt->context != st->context)
        return false;

    if(rt->foreach_dimension_pattern && !rrdset_number_of_dimensions(st))
        return false;

    if (rt->charts_pattern && !simple_pattern_matches_string(rt->charts_pattern, st->name) && !simple_pattern_matches_string(rt->charts_pattern, st->id))
        return false;

    if (rt->family_pattern && !simple_pattern_matches_string(rt->family_pattern, st->family))
        return false;

    if (rt->module_pattern && !simple_pattern_matches_string(rt->module_pattern, st->module_name))
        return false;

    if (rt->plugin_pattern && !simple_pattern_matches_string(rt->plugin_pattern, st->plugin_name))
        return false;

    if(host->rrdlabels && rt->host_labels_pattern && !rrdlabels_match_simple_pattern_parsed(host->rrdlabels,
                                                                                            rt->host_labels_pattern,
                                                                                            '=', NULL))
        return false;

    if(st->rrdlabels && rt->chart_labels_pattern && !rrdlabels_match_simple_pattern_parsed(st->rrdlabels,
                                                                                            rt->chart_labels_pattern,
                                                                                            '=', NULL))
        return false;

    return true;
}

void rrdcalctemplate_check_rrddim_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDDIM *rd, RRDHOST *host) {
    if (simple_pattern_matches_string(rt->foreach_dimension_pattern, rd->id) ||
        simple_pattern_matches_string(rt->foreach_dimension_pattern, rd->name)) {
        char *overwrite_alert_name = rrdcalc_alert_name_with_dimension(
            rrdcalctemplate_name(rt), string_strlen(rt->name), rrddim_name(rd), string_strlen(rd->name));
        rrdcalc_add_from_rrdcalctemplate(host, rt, st, overwrite_alert_name, rrddim_name(rd));
        freez(overwrite_alert_name);
    }
}

void rrdcalctemplate_check_conditions_and_link(RRDCALCTEMPLATE *rt, RRDSET *st, RRDHOST *host) {
    if(!rrdcalctemplate_check_rrdset_conditions(rt, st, host))
        return;

    if(!rt->foreach_dimension_pattern) {
        rrdcalc_add_from_rrdcalctemplate(host, rt, st, NULL, NULL);
        return;
    }

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rrdcalctemplate_check_rrddim_conditions_and_link(rt, st, rd, host);
    }
    rrddim_foreach_done(rd);
}

void rrdcalctemplate_link_matching_templates_to_rrdset(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    RRDCALCTEMPLATE *rt;
    foreach_rrdcalctemplate_read(host, rt) {
        rrdcalctemplate_check_conditions_and_link(rt, st, host);
    }
    foreach_rrdcalctemplate_done(rt);
}

static void rrdcalctemplate_free_internals(RRDCALCTEMPLATE *rt) {
    expression_free(rt->calculation);
    expression_free(rt->warning);
    expression_free(rt->critical);

    string_freez(rt->family_match);
    simple_pattern_free(rt->family_pattern);

    string_freez(rt->plugin_match);
    simple_pattern_free(rt->plugin_pattern);

    string_freez(rt->module_match);
    simple_pattern_free(rt->module_pattern);

    string_freez(rt->charts_match);
    simple_pattern_free(rt->charts_pattern);

    string_freez(rt->name);
    string_freez(rt->exec);
    string_freez(rt->recipient);
    string_freez(rt->classification);
    string_freez(rt->component);
    string_freez(rt->type);
    string_freez(rt->context);
    string_freez(rt->source);
    string_freez(rt->units);
    string_freez(rt->info);
    string_freez(rt->dimensions);
    string_freez(rt->foreach_dimension);
    string_freez(rt->host_labels);
    string_freez(rt->chart_labels);
    simple_pattern_free(rt->foreach_dimension_pattern);
    simple_pattern_free(rt->host_labels_pattern);
    simple_pattern_free(rt->chart_labels_pattern);
}

void rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt)) return;

    rrdcalctemplate_free_internals(rt);
    freez(rt);
}
static void rrdcalctemplate_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalctemplate, void *added_bool) {
    RRDCALCTEMPLATE *rt = rrdcalctemplate; (void)rt;

    bool *added = added_bool;
    *added = true;

    debug(D_HEALTH, "Health configuration adding template '%s'"
                    ": context '%s'"
                    ", exec '%s'"
                    ", recipient '%s'"
                    ", green " NETDATA_DOUBLE_FORMAT_AUTO
                    ", red " NETDATA_DOUBLE_FORMAT_AUTO
                    ", lookup: group %d"
                    ", after %d"
                    ", before %d"
                    ", options %u"
                    ", dimensions '%s'"
                    ", for each dimension '%s'"
                    ", update every %d"
                    ", calculation '%s'"
                    ", warning '%s'"
                    ", critical '%s'"
                    ", source '%s'"
                    ", delay up %d"
                    ", delay down %d"
                    ", delay max %d"
                    ", delay_multiplier %f"
                    ", warn_repeat_every %u"
                    ", crit_repeat_every %u",
          rrdcalctemplate_name(rt),
          (rt->context)?string2str(rt->context):"NONE",
          (rt->exec)?rrdcalctemplate_exec(rt):"DEFAULT",
          (rt->recipient)?rrdcalctemplate_recipient(rt):"DEFAULT",
          rt->green,
          rt->red,
          (int)rt->group,
          rt->after,
          rt->before,
          rt->options,
          (rt->dimensions)?rrdcalctemplate_dimensions(rt):"NONE",
          (rt->foreach_dimension)?rrdcalctemplate_foreachdim(rt):"NONE",
          rt->update_every,
          (rt->calculation)?rt->calculation->parsed_as:"NONE",
          (rt->warning)?rt->warning->parsed_as:"NONE",
          (rt->critical)?rt->critical->parsed_as:"NONE",
          rrdcalctemplate_source(rt),
          rt->delay_up_duration,
          rt->delay_down_duration,
          rt->delay_max_duration,
          rt->delay_multiplier,
          rt->warn_repeat_every,
          rt->crit_repeat_every
    );
}

static void rrdcalctemplate_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdcalctemplate, void *rrdhost __maybe_unused) {
    RRDCALCTEMPLATE *rt = rrdcalctemplate;
    rrdcalctemplate_free_internals(rt);
}

void rrdcalctemplate_index_init(RRDHOST *host) {
    if(!host->rrdcalctemplate_root_index) {
        host->rrdcalctemplate_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                                      &dictionary_stats_category_rrdhealth, sizeof(RRDCALCTEMPLATE));

        dictionary_register_insert_callback(host->rrdcalctemplate_root_index, rrdcalctemplate_insert_callback, NULL);
        dictionary_register_delete_callback(host->rrdcalctemplate_root_index, rrdcalctemplate_delete_callback, host);
    }
}

void rrdcalctemplate_index_destroy(RRDHOST *host) {
    dictionary_destroy(host->rrdcalctemplate_root_index);
    host->rrdcalctemplate_root_index = NULL;
}

inline void rrdcalctemplate_delete_all(RRDHOST *host) {
    dictionary_flush(host->rrdcalctemplate_root_index);
}

#define RRDCALCTEMPLATE_MAX_KEY_SIZE 1024
static size_t rrdcalctemplate_key(char *dst, size_t dst_len, const char *name, const char *family_match) {
    return snprintfz(dst, dst_len, "%s/%s", name, (family_match && *family_match)?family_match:"*");
}

void rrdcalctemplate_add_from_config(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    if(unlikely(!rt->context)) {
        error("Health configuration for template '%s' does not have a context", rrdcalctemplate_name(rt));
        return;
    }

    if(unlikely(!rt->update_every)) {
        error("Health configuration for template '%s' has no frequency (parameter 'every'). Ignoring it.", rrdcalctemplate_name(rt));
        return;
    }

    if(unlikely(!RRDCALCTEMPLATE_HAS_DB_LOOKUP(rt) && !rt->calculation && !rt->warning && !rt->critical)) {
        error("Health configuration for template '%s' is useless (no calculation, no warning and no critical evaluation)", rrdcalctemplate_name(rt));
        return;
    }

    char key[RRDCALCTEMPLATE_MAX_KEY_SIZE + 1];
    size_t key_len = rrdcalctemplate_key(key, RRDCALCTEMPLATE_MAX_KEY_SIZE, rrdcalctemplate_name(rt), rrdcalctemplate_family_match(rt));

    bool added = false;
    dictionary_set_advanced(host->rrdcalctemplate_root_index, key, (ssize_t)(key_len + 1), rt, sizeof(*rt), &added);

    if(added)
        freez(rt);
    else {
        info("Health configuration template '%s' already exists for host '%s'.", rrdcalctemplate_name(rt), rrdhost_hostname(host));
        rrdcalctemplate_free_unused_rrdcalctemplate_loaded_from_config(rt);
    }
}
