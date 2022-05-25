// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

/*
 * This is the action defined for the FLUSH command
 */
PARSER_RC pluginsd_set_action(void *user, RRDSET *st, RRDDIM *rd, long long int value)
{
    UNUSED(user);

    rrddim_set_by_pointer(st, rd, value);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush_action(void *user, RRDSET *st)
{
    UNUSED(user);
    UNUSED(st);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_begin_action(void *user, RRDSET *st, usec_t microseconds, int trust_durations)
{
    UNUSED(user);
    if (likely(st->counter_done)) {
        if (likely(microseconds)) {
            if (trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        } else
            rrdset_next(st);
    }
    return PARSER_RC_OK;
}


PARSER_RC pluginsd_end_action(void *user, RRDSET *st)
{
    UNUSED(user);

    rrdset_done(st);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context, char *title, char *units, char *plugin,
           char *module, int priority, int update_every, RRDSET_TYPE chart_type, char *options)
{
    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    st = rrdset_create(
        host, type, id, name, family, context, title, units,
        plugin, module, priority, update_every,
        chart_type);

    if (options && *options) {
        if (strstr(options, "obsolete"))
            rrdset_is_obsolete(st);
        else
            rrdset_isnot_obsolete(st);

        if (strstr(options, "detail"))
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);
        else
            rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);

        if (strstr(options, "hidden"))
            rrdset_flag_set(st, RRDSET_FLAG_HIDDEN);
        else
            rrdset_flag_clear(st, RRDSET_FLAG_HIDDEN);

        if (strstr(options, "store_first"))
            rrdset_flag_set(st, RRDSET_FLAG_STORE_FIRST);
        else
            rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
    } else {
        rrdset_isnot_obsolete(st);
        rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
        rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
    }
    ((PARSER_USER_OBJECT *)user)->st = st;

    return PARSER_RC_OK;
}


PARSER_RC pluginsd_disable_action(void *user)
{
    UNUSED(user);

    info("called DISABLE. Disabling it.");
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}


PARSER_RC pluginsd_variable_action(void *user, RRDHOST *host, RRDSET *st, char *name, int global, calculated_number value)
{
    UNUSED(user);

    if (global) {
        RRDVAR *rv = rrdvar_custom_host_variable_create(host, name);
        if (rv)
            rrdvar_custom_host_variable_set(host, rv, value);
        else
            error("cannot find/create HOST VARIABLE '%s' on host '%s'", name, host->hostname);
    } else {
        RRDSETVAR *rs = rrdsetvar_custom_chart_variable_create(st, name);
        if (rs)
            rrdsetvar_custom_chart_variable_set(rs, value);
        else
            error("cannot find/create CHART VARIABLE '%s' on host '%s', chart '%s'", name, host->hostname, st->id);
    }
    return PARSER_RC_OK;
}



PARSER_RC pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm, long multiplier, long divisor, char *options,
                                    RRD_ALGORITHM algorithm_type)
{
    UNUSED(user);
    UNUSED(algorithm);

    RRDDIM *rd = rrddim_add(st, id, name, multiplier, divisor, algorithm_type);
    int unhide_dimension = 1;

    rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    if (options && *options) {
        if (strstr(options, "obsolete") != NULL)
            rrddim_is_obsolete(st, rd);
        else
            rrddim_isnot_obsolete(st, rd);

        unhide_dimension = !strstr(options, "hidden");

        if (strstr(options, "noreset") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
        if (strstr(options, "nooverflow") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    } else
        rrddim_isnot_obsolete(st, rd);

    if (likely(unhide_dimension)) {
        rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
        if (rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
            (void)sql_set_dimension_option(&rd->state->metric_uuid, NULL);
            rrddim_flag_clear(rd, RRDDIM_FLAG_META_HIDDEN);
        }
    } else {
        rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
        if (!rrddim_flag_check(rd, RRDDIM_FLAG_META_HIDDEN)) {
           (void)sql_set_dimension_option(&rd->state->metric_uuid, "hidden");
            rrddim_flag_set(rd, RRDDIM_FLAG_META_HIDDEN);
        }
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_label_action(void *user, char *key, char *value, LABEL_SOURCE source)
{

    ((PARSER_USER_OBJECT *) user)->new_labels = add_label_to_list(((PARSER_USER_OBJECT *) user)->new_labels, key, value, source);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_action(void *user, char *key, char *value, LABEL_SOURCE source)
{
    ((PARSER_USER_OBJECT *) user)->chart_labels = add_label_to_list(((PARSER_USER_OBJECT *) user)->chart_labels, key, value, source);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_commit_action(void *user, RRDHOST *host, struct label *new_labels)
{
    RRDSET *st = ((PARSER_USER_OBJECT *)user)->st;
    if (unlikely(!st)) {
        error("requested CLABEL_COMMIT on host '%s', without a BEGIN, ignoring it.", host->hostname);
        return PARSER_RC_OK;
    }

    rrdset_update_labels(st, new_labels);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite_action(void *user, RRDHOST *host, struct label *new_labels)
{
    UNUSED(user);

    if (!host->labels.head) {
        host->labels.head = new_labels;
    } else {
        rrdhost_rdlock(host);
        replace_label_list(&host->labels, new_labels);
        rrdhost_unlock(host);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_set(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *dimension = words[1];
    char *value = words[2];

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!dimension || !*dimension)) {
        error("requested a SET on chart '%s' of host '%s', without a dimension. Disabling it.", st->id, host->hostname);
        goto disable;
    }

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!st)) {
        error(
            "requested a SET on dimension %s with value %s on host '%s', without a BEGIN. Disabling it.", dimension,
            value ? value : "<nothing>", host->hostname);
        goto disable;
    }

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "is setting dimension %s/%s to %s", st->id, dimension, value ? value : "<nothing>");

    if (value) {
        RRDDIM *rd = rrddim_find(st, dimension);
        if (unlikely(!rd)) {
            error(
                "requested a SET to dimension with id '%s' on stats '%s' (%s) on host '%s', which does not exist. Disabling it.",
                dimension, st->name, st->id, st->rrdhost->hostname);
            goto disable;
        } else {
            if (plugins_action->set_action) {
                return plugins_action->set_action(
                    user, st, rd, strtoll(value, NULL, 0));
            }
        }
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *id = words[1];
    char *microseconds_txt = words[2];

    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;

    if (unlikely(!id)) {
        error("requested a BEGIN without a chart id for host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

    st = rrdset_find(host, id);
    if (unlikely(!st)) {
        error("requested a BEGIN on chart '%s', which does not exist on host '%s'. Disabling it.", id, host->hostname);
        goto disable;
    }
    ((PARSER_USER_OBJECT *)user)->st = st;

    usec_t microseconds = 0;
    if (microseconds_txt && *microseconds_txt)
        microseconds = str2ull(microseconds_txt);

    if (plugins_action->begin_action) {
        return plugins_action->begin_action(user, st, microseconds,
                                            ((PARSER_USER_OBJECT *)user)->trust_durations);
    }
    return PARSER_RC_OK;
disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_end(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);
    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("requested an END, without a BEGIN on host '%s'. Disabling it.", host->hostname);
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "requested an END on chart %s", st->id);

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    ((PARSER_USER_OBJECT *) user)->count++;
    if (plugins_action->end_action) {
        return plugins_action->end_action(user, st);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    if (unlikely(!host && !((PARSER_USER_OBJECT *) user)->host_exists)) {
        debug(D_PLUGINSD, "Ignoring chart belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }

    char *type = words[1];
    char *name = words[2];
    char *title = words[3];
    char *units = words[4];
    char *family = words[5];
    char *context = words[6];
    char *chart = words[7];
    char *priority_s = words[8];
    char *update_every_s = words[9];
    char *options = words[10];
    char *plugin = words[11];
    char *module = words[12];

    int have_action = ((plugins_action->chart_action) != NULL);

    // parse the id from type
    char *id = NULL;
    if (likely(type && (id = strchr(type, '.')))) {
        *id = '\0';
        id++;
    }

    // make sure we have the required variables
    if (unlikely((!type || !*type || !id || !*id))) {
        if (likely(host))
            error("requested a CHART, without a type.id, on host '%s'. Disabling it.", host->hostname);
        else
            error("requested a CHART, without a type.id. Disabling it.");
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    // parse the name, and make sure it does not include 'type.'
    if (unlikely(name && *name)) {
        // when data are streamed from child nodes
        // name will be type.name
        // so we have to remove 'type.' from name too
        size_t len = strlen(type);
        if (strncmp(type, name, len) == 0 && name[len] == '.')
            name = &name[len + 1];

        // if the name is the same with the id,
        // or is just 'NULL', clear it.
        if (unlikely(strcmp(name, id) == 0 || strcasecmp(name, "NULL") == 0 || strcasecmp(name, "(NULL)") == 0))
            name = NULL;
    }

    int priority = 1000;
    if (likely(priority_s && *priority_s))
        priority = str2i(priority_s);

    int update_every = ((PARSER_USER_OBJECT *) user)->cd->update_every;
    if (likely(update_every_s && *update_every_s))
        update_every = str2i(update_every_s);
    if (unlikely(!update_every))
        update_every = ((PARSER_USER_OBJECT *) user)->cd->update_every;

    RRDSET_TYPE chart_type = RRDSET_TYPE_LINE;
    if (unlikely(chart))
        chart_type = rrdset_type_id(chart);

    if (unlikely(name && !*name))
        name = NULL;
    if (unlikely(family && !*family))
        family = NULL;
    if (unlikely(context && !*context))
        context = NULL;
    if (unlikely(!title))
        title = "";
    if (unlikely(!units))
        units = "unknown";

    debug(
        D_PLUGINSD,
        "creating chart type='%s', id='%s', name='%s', family='%s', context='%s', chart='%s', priority=%d, update_every=%d",
        type, id, name ? name : "", family ? family : "", context ? context : "", rrdset_type_name(chart_type),
        priority, update_every);

    if (have_action) {
        return plugins_action->chart_action(
            user, type, id, name, family, context, title, units,
            (plugin && *plugin) ? plugin : ((PARSER_USER_OBJECT *)user)->cd->filename, module, priority, update_every,
            chart_type, options);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_dimension(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *id = words[1];
    char *name = words[2];
    char *algorithm = words[3];
    char *multiplier_s = words[4];
    char *divisor_s = words[5];
    char *options = words[6];

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    if (unlikely(!host && !((PARSER_USER_OBJECT *) user)->host_exists)) {
        debug(D_PLUGINSD, "Ignoring dimension belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }

    if (unlikely(!id)) {
        error(
            "requested a DIMENSION, without an id, host '%s' and chart '%s'. Disabling it.", host->hostname,
            st ? st->id : "UNSET");
        goto disable;
    }

    if (unlikely(!st && !((PARSER_USER_OBJECT *) user)->st_exists)) {
        error("requested a DIMENSION, without a CHART, on host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

    long multiplier = 1;
    if (multiplier_s && *multiplier_s) {
        multiplier = strtol(multiplier_s, NULL, 0);
        if (unlikely(!multiplier))
            multiplier = 1;
    }

    long divisor = 1;
    if (likely(divisor_s && *divisor_s)) {
        divisor = strtol(divisor_s, NULL, 0);
        if (unlikely(!divisor))
            divisor = 1;
    }

    if (unlikely(!algorithm || !*algorithm))
        algorithm = "absolute";

    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(
            D_PLUGINSD,
            "creating dimension in chart %s, id='%s', name='%s', algorithm='%s', multiplier=%ld, divisor=%ld, hidden='%s'",
            st->id, id, name ? name : "", rrd_algorithm_name(rrd_algorithm_id(algorithm)), multiplier, divisor,
            options ? options : "");

    if (plugins_action->dimension_action) {
        return plugins_action->dimension_action(
                user, st, id, name, algorithm,
            multiplier, divisor, (options && *options)?options:NULL, rrd_algorithm_id(algorithm));
    }

    return PARSER_RC_OK;
disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_variable(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *name = words[1];
    char *value = words[2];
    calculated_number v;

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    int global = (st) ? 0 : 1;

    if (name && *name) {
        if ((strcmp(name, "GLOBAL") == 0 || strcmp(name, "HOST") == 0)) {
            global = 1;
            name = words[2];
            value = words[3];
        } else if ((strcmp(name, "LOCAL") == 0 || strcmp(name, "CHART") == 0)) {
            global = 0;
            name = words[2];
            value = words[3];
        }
    }

    if (unlikely(!name || !*name)) {
        error("requested a VARIABLE on host '%s', without a variable name. Disabling it.", host->hostname);
        ((PARSER_USER_OBJECT *)user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (unlikely(!value || !*value))
        value = NULL;

    if (unlikely(!value)) {
        error("cannot set %s VARIABLE '%s' on host '%s' to an empty value", (global) ? "HOST" : "CHART", name,
              host->hostname);
        return PARSER_RC_OK;
    }

    if (!global && !st) {
        error("cannot find/create CHART VARIABLE '%s' on host '%s' without a chart", name, host->hostname);
        return PARSER_RC_OK;
    }

    char *endptr = NULL;
    v = (calculated_number)str2ld(value, &endptr);
    if (unlikely(endptr && *endptr)) {
        if (endptr == value)
            error(
                "the value '%s' of VARIABLE '%s' on host '%s' cannot be parsed as a number", value, name,
                host->hostname);
        else
            error(
                "the value '%s' of VARIABLE '%s' on host '%s' has leftovers: '%s'", value, name, host->hostname,
                endptr);
    }

    if (plugins_action->variable_action) {
        return plugins_action->variable_action(user, host, st, name, global, v);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);
    debug(D_PLUGINSD, "requested a FLUSH");
    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    ((PARSER_USER_OBJECT *) user)->st = NULL;
    if (plugins_action->flush_action) {
        return plugins_action->flush_action(user, st);
    }
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_disable(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(user);
    UNUSED(words);

    if (plugins_action->disable_action) {
        return plugins_action->disable_action(user);
    }
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_label(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *store;

    if (!words[1] || !words[2] || !words[3]) {
        error("Ignoring malformed or empty LABEL command.");
        return PARSER_RC_OK;
    }
    if (!words[4])
        store = words[3];
    else {
        store = callocz(PLUGINSD_LINE_MAX + 1, sizeof(char));
        size_t remaining = PLUGINSD_LINE_MAX;
        char *move = store;
        int i = 3;
        while (i < PLUGINSD_MAX_WORDS) {
            size_t length = strlen(words[i]);
            if ((length + 1) >= remaining)
                break;

            remaining -= (length + 1);
            memcpy(move, words[i], length);
            move += length;
            *move++ = ' ';

            i++;
            if (!words[i])
                break;
        }
    }

    if (plugins_action->label_action) {
        PARSER_RC rc = plugins_action->label_action(user, words[1], store, strtol(words[2], NULL, 10));
        if (store != words[3])
            freez(store);
        return rc;
    }

    if (store != words[3])
        freez(store);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    if (!words[1] || !words[2] || !words[3]) {
        error("Ignoring malformed or empty CHART LABEL command.");
        return PARSER_RC_OK;
    }

    if (plugins_action->clabel_action) {
        PARSER_RC rc = plugins_action->clabel_action(user, words[1], words[2], strtol(words[3], NULL, 10));
        return rc;
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_clabel_commit(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);

    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    debug(D_PLUGINSD, "requested to commit chart labels");

    struct label *chart_labels = ((PARSER_USER_OBJECT *)user)->chart_labels;
    ((PARSER_USER_OBJECT *)user)->chart_labels = NULL;

    if (plugins_action->clabel_commit_action) {
        return plugins_action->clabel_commit_action(user, host, chart_labels);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    UNUSED(words);

    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    debug(D_PLUGINSD, "requested a OVERWRITE a variable");

    struct label *new_labels = ((PARSER_USER_OBJECT *)user)->new_labels;
    ((PARSER_USER_OBJECT *)user)->new_labels = NULL;

    if (plugins_action->overwrite_action) {
        return plugins_action->overwrite_action(user, host, new_labels);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_guid(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a GUID, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a GUID, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->guid_action) {
        return plugins_action->guid_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_context(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a CONTEXT, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a CONTEXT, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->context_action) {
        return plugins_action->context_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_tombstone(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    char *uuid_str = words[1];
    uuid_t uuid;

    if (unlikely(!uuid_str)) {
        error("requested a TOMBSTONE, without a uuid.");
        return PARSER_RC_ERROR;
    }
    if (unlikely(strlen(uuid_str) != GUID_LEN || uuid_parse(uuid_str, uuid) == -1)) {
        error("requested a TOMBSTONE, without a valid uuid string.");
        return PARSER_RC_ERROR;
    }

    debug(D_PLUGINSD, "Parsed uuid=%s", uuid_str);
    if (plugins_action->tombstone_action) {
        return plugins_action->tombstone_action(user, &uuid);
    }

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_host(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *machine_guid = words[1];
    char *hostname = words[2];
    char *registry_hostname = words[3];
    char *update_every_s = words[4];
    char *os = words[5];
    char *timezone = words[6];
    char *tags = words[7];

    int update_every = 1;
    if (likely(update_every_s && *update_every_s))
        update_every = str2i(update_every_s);
    if (unlikely(!update_every))
        update_every = 1;

    debug(D_PLUGINSD, "HOST PARSED: guid=%s, hostname=%s, reg_host=%s, update=%d, os=%s, timezone=%s, tags=%s",
         machine_guid, hostname, registry_hostname, update_every, os, timezone, tags);

    if (plugins_action->host_action) {
        return plugins_action->host_action(
            user, machine_guid, hostname, registry_hostname, update_every, os, timezone, tags);
    }

    return PARSER_RC_OK;
}

static void pluginsd_process_thread_cleanup(void *ptr) {
    PARSER *parser = (PARSER *)ptr;
    parser_destroy(parser);
}

// New plugins.d parser
#ifdef  ENABLE_REPLICATION
// Pluginsd_action for the replication commands
PARSER_RC pluginsd_rep_action(void *user, REP_ARG command)
{
    info("%s: REP command - pluginsd_rep_action\n", REPLICATION_MSG);
    if(!user || !((PARSER_USER_OBJECT *)user)->host || !((PARSER_USER_OBJECT *)user)->opaque) {
        infoerr("%s: Parser user object was not set properly - user, host or opaque is NULL - Exiting Parser!", REPLICATION_MSG);
        return PARSER_RC_ERROR;
    }
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;
    REPLICATION_STATE *rep_state = ((PARSER_USER_OBJECT *)user)->opaque;

    switch (command)
    {
      case REP_OFF:
        info("%s: REP OFF command is received!\n", REPLICATION_MSG);
        // Shutdown the replication thread.
        rep_state->shutdown = 1;
        // For now simply return an error to exit
        ((PARSER_USER_OBJECT *)user)->enabled = 0;
        return PARSER_RC_ERROR;
      case REP_ON:
        info("%s: REP ON command is received!\n", REPLICATION_MSG);
        if (finish_gap_replication(host, rep_state)) {
            // Exit the Rx parser
            ((PARSER_USER_OBJECT *)user)->enabled = 0;
            return PARSER_RC_ERROR;
        }
        send_gap_for_replication(host, rep_state);
        return PARSER_RC_OK;
      case REP_ACK:
        info("%s: REP ACK command is received!\n", REPLICATION_MSG);
        // REP ACK - A full REP transmission of all the charts
        // Clear the replicated GAP
        cleanup_after_gap_replication(host->gaps_timeline);
        if (finish_gap_replication(host, rep_state)) {
            // Exit the Rx parser
            ((PARSER_USER_OBJECT *)user)->enabled = 0;
            return PARSER_RC_ERROR;
        }
        send_gap_for_replication(host, rep_state);
        return PARSER_RC_OK;       
      default:
        info("%s: REP %u command is unknown!\n", REPLICATION_MSG, command);
    }

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_gap_action(void *user, GAP rx_gap)
{
    info("%s: GAP command - pluginsd_gap_action\n", REPLICATION_MSG);
    if(!user || !((PARSER_USER_OBJECT *)user)->opaque) {
        infoerr("%s: Parser user object was not set properly - user, host or opaque is NULL - Exiting Parser!", REPLICATION_MSG);
        return PARSER_RC_ERROR;
    }
    REPLICATION_STATE *rep_state = ((PARSER_USER_OBJECT *)user)->opaque;
    //Check if there is GAP and send GAP command, otherwise send REP OFF command
    sender_gap_filling(rep_state, rx_gap);
    info("%s: COMPLETE GAP - Send REP ACK\n", REPLICATION_MSG);
    send_message(rep_state, "REP 3\n");
    info("%s: EXITING GAP command - All charts sent\n", REPLICATION_MSG);

    // return PARSER_RC_OK;
    // ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_rdata_action(void *user, GAP meta_rx_rdata, int block_id, char *chart_id, char *dim_id)
{
    if(!user || !((PARSER_USER_OBJECT *)user)->opaque) {
        infoerr("%s: Parser user object was not set properly - user, or opaque is NULL - Exiting Parser!", REPLICATION_MSG);
        return PARSER_RC_ERROR;
    }
    REPLICATION_STATE *rep_state = ((PARSER_USER_OBJECT *)user)->opaque;

    // info("%s: RDATA command - pluginsd_rdata_action\n", REPLICATION_MSG);
    char gap_uuid_str[UUID_STR_LEN];
    uuid_unparse(meta_rx_rdata.gap_uuid, gap_uuid_str);
    info("%s: Receiving RDATA block id#%d for gap(%s): %s\n", REPLICATION_MSG, block_id, meta_rx_rdata.status,gap_uuid_str);
    replication_collect_past_metric_init(rep_state, chart_id, dim_id);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_fill_action(void *user, time_t timestamp, storage_number value)
{
    if(!user || !((PARSER_USER_OBJECT *)user)->opaque) {
        infoerr("%s: Parser user object was not set properly - user, or opaque is NULL - Exiting Parser!", REPLICATION_MSG);
        return PARSER_RC_ERROR;
    }    
    REPLICATION_STATE *rep_state = ((PARSER_USER_OBJECT *)user)->opaque;
    // info("%s: FILL command - pluginsd_fill_action\n", REPLICATION_MSG);
    //rrddim_find
    //call a similar void rrdeng_store_metric_next(RRDDIM *rd, usec_t point_in_time, storage_number number)
    //function to save the fill value in the page
    //collect_replication_gap_data(rd, timestamp, value);
    replication_collect_past_metric(rep_state, timestamp, value);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_fill_end_action(void *user, int block_id)
{
    UNUSED(block_id);
    REPLICATION_STATE *rep_state = (REPLICATION_STATE *)((PARSER_USER_OBJECT *)user)->opaque;
    info("%s: FILLEND command - pluginsd_fill_end_action\n", REPLICATION_MSG);
    replication_collect_past_metric_done(rep_state);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_rep(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{
    char *arg_str = words[1];
    REP_ARG command = strtoul(arg_str, NULL, 10);
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    // info("Pluginsd_rep received! %s %s(%d)\n", PLUGINSD_KEYWORD_REP, words[1], command);

    if (unlikely((!arg_str || command == REP_ARG_ERROR || errno == ERANGE))) {
        error("REP command either is missing or is wrong for host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

    if (plugins_action->rep_action) {
        return plugins_action->rep_action((PARSER_USER_OBJECT *) user, command);
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_gap(char **words, void *user, PLUGINSD_ACTION  *plugins_action){

    info("%s: GAP command - pluginsd_gap\n", REPLICATION_MSG);
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    
    // This needs to enable the LOCALHOST->gaps_timeline->gap_data struct in order to work
    // GAP rx_gap = host->gaps_timeline->gap_data;
    GAP rx_gap;
    int rc = uuid_parse(words[1], rx_gap.gap_uuid);
    rx_gap.t_window.t_start = (time_t) strtol(words[2], NULL, 10);
    rx_gap.t_window.t_first = (time_t) strtol(words[3], NULL, 10);
    rx_gap.t_window.t_end = (time_t) strtol(words[4], NULL, 10);
    rx_gap.status = "onrequest";

    if (unlikely((rc == -1) || ((!rx_gap.t_window.t_start || !rx_gap.t_window.t_first  || !rx_gap.t_window.t_end ) || errno == ERANGE))) {
        error("requested a GAP with wrong parameters for host '%s'. Disabling it.",
        host->hostname);
        goto disable;
    }

    if (plugins_action->gap_action) {
        return plugins_action->gap_action((PARSER_USER_OBJECT *) user, rx_gap);
    }

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_rdata(char **words, void *user, PLUGINSD_ACTION  *plugins_action){
    
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    GAP meta_rx_rdata;
    int rc = uuid_parse(words[1], meta_rx_rdata.gap_uuid);
    char *chart_id = strdupz(words[2]);
    char *dim_id = strdupz(words[3]);
    meta_rx_rdata.t_window.t_start = (time_t) strtol(words[4], NULL, 10);
    meta_rx_rdata.t_window.t_end = (time_t) strtol(words[5], NULL, 10);
    int block_id = strtol(words[6], NULL, 10);
    meta_rx_rdata.status = "onreceive";

    if (unlikely((rc == -1) || !chart_id || !dim_id || ((!meta_rx_rdata.t_window.t_start || !meta_rx_rdata.t_window.t_end ) || errno == ERANGE))) {
        error("requested a RDATA without parameters for host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

    if(block_id == 9)
        meta_rx_rdata.status = "rx_complete";

    //Call RDATA function with parameters    
    if (plugins_action->rdata_action) {
        return plugins_action->rdata_action((PARSER_USER_OBJECT *) user, meta_rx_rdata, block_id, chart_id, dim_id);
    }    

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_fill(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{    
    REPLICATION_STATE *rep_state = (REPLICATION_STATE *)((PARSER_USER_OBJECT *)user)->opaque;
    char *chart_id = strdupz(words[1]);
    char *dim_id = strdupz(words[2]);
    time_t timestamp = (time_t) strtol(words[3], NULL, 10);
    storage_number value = str2uint32_t(words[4]);

    if (unlikely( !chart_id || !dim_id || !timestamp || errno == ERANGE)) {
        error("Parsing FILL command parameters has failed for host '%s'. Disabling it.", rep_state->host->hostname);
        goto disable;
    }

    // info("%s: FILL %s.%s %ld %d", REPLICATION_MSG, chart_id, dim_id, timestamp, value);
    //Call the replication function to save the parameters.
    if (plugins_action->fill_action) {
        return plugins_action->fill_action((PARSER_USER_OBJECT *) user, timestamp, value);
    }

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_fill_end(char **words, void *user, PLUGINSD_ACTION  *plugins_action)
{    
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;
    int numofpoints = strtol(words[1], NULL, 10);
    int block_id = strtol(words[2], NULL, 10);

    if (unlikely(errno == ERANGE)) {
        error("%s: FILLEND parameters parsing failed for host '%s'. Disabling it.", REPLICATION_MSG, host->hostname);
        goto disable;
    }

    if(unlikely(!numofpoints)) {
        infoerr("%s: FILLEND Zero #samples - Continue to the next dimension - host '%s'.", REPLICATION_MSG, host->hostname);
        return PARSER_RC_OK;
    }

    //Call the replication function to save the parameters.
    if (plugins_action->fill_end_action) {
        return plugins_action->fill_end_action((PARSER_USER_OBJECT *) user, block_id);
    }    

    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *)user)->enabled = 0;
    return PARSER_RC_ERROR;
}

// Add an incative plugins_action for security reasons. All the parser instances should not be able to use all the available actions.
PARSER_RC pluginsd_suspend_this_action(void *user, RRDSET *st, usec_t microseconds, int trust_durations)
{
    UNUSED(st);
    UNUSED(user);    
    UNUSED(microseconds);
    UNUSED(trust_durations);
    error("This keyword is not supported from this parser!");
    //Parser continues?
    return PARSER_RC_OK;
    //Parser stops?
    // return PARSER_RC_ERROR;
}
#endif  //ENABLE_REPLICATION

inline size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp, int trust_durations)
{
    int enabled = cd->enabled;

    if (!fp || !enabled) {
        cd->enabled = 0;
        return 0;
    }

    if (unlikely(fileno(fp) == -1)) {
        error("file descriptor given is not a valid stream");
        cd->serial_failures++;
        return 0;
    }
    clearerr(fp);

    PARSER_USER_OBJECT user = {
        .enabled = cd->enabled,
        .host = host,
        .cd = cd,
        .trust_durations = trust_durations
    };

    PARSER *parser = parser_init(host, &user, fp, PARSER_INPUT_SPLIT);

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser);

    parser->plugins_action->begin_action          = &pluginsd_begin_action;
    parser->plugins_action->flush_action          = &pluginsd_flush_action;
    parser->plugins_action->end_action            = &pluginsd_end_action;
    parser->plugins_action->disable_action        = &pluginsd_disable_action;
    parser->plugins_action->variable_action       = &pluginsd_variable_action;
    parser->plugins_action->dimension_action      = &pluginsd_dimension_action;
    parser->plugins_action->label_action          = &pluginsd_label_action;
    parser->plugins_action->overwrite_action      = &pluginsd_overwrite_action;
    parser->plugins_action->chart_action          = &pluginsd_chart_action;
    parser->plugins_action->set_action            = &pluginsd_set_action;
    parser->plugins_action->clabel_commit_action  = &pluginsd_clabel_commit_action;
    parser->plugins_action->clabel_action         = &pluginsd_clabel_action;

    user.parser = parser;

    while (likely(!parser_next(parser))) {
        if (unlikely(netdata_exit || parser_action(parser,  NULL)))
            break;
    }

    // free parser with the pop function
    netdata_thread_cleanup_pop(1);

    cd->enabled = user.enabled;
    size_t count = user.count;

    if (likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    }
    else
        cd->serial_failures++;

    return count;
}
