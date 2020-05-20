// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_parser.h"

// Sample action here

PARSER_RC pluginsd_context_action(void *user, char *context)
{
    UNUSED(user);
    UNUSED(context);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_guid_action(void *user, char *guid)
{
    UNUSED(user);
    UNUSED(guid);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_host_action(void *user, char *context)      // TODO: 7 params
{
    UNUSED(user);
    UNUSED(context);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_tombstone_action(void *user, char *guid)
{
    UNUSED(user);
    UNUSED(guid);

    return PARSER_RC_OK;
}


// Callbacks

PARSER_RC pluginsd_context(char **words, void *user)
{
    UNUSED(user);
    UNUSED(words);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_guid(char **words, void *user)
{
    UNUSED(user);
    UNUSED(words);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_host(char **words, void *user)
{
    UNUSED(user);
    UNUSED(words);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_tombstone(char **words, void *user)
{
    UNUSED(user);
    UNUSED(words);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_begin_action(void *user, char *chart_id, usec_t microseconds)
{
    UNUSED(user);
    info("This is the begin action on %s -- %d", chart_id, (int ) microseconds);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_set(char **words, void *user)
{
    char *dimension = words[1];
    char *value = words[2];

    if (((PARSER_USER_OBJECT *)user)->plugins_action->set_action) {
        return ((PARSER_USER_OBJECT *)user)->plugins_action->set_action(
                user, (!dimension || !*dimension) ? NULL : dimension, (!value || !*value) ? 0 : strtoll(value, NULL, 0));
    }

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
        } else
            rrddim_set_by_pointer(st, rd, strtoll(value, NULL, 0));
    }
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_begin(char **words, void *user)
{
    char *id = words[1];
    char *microseconds_txt = words[2];

    usec_t microseconds = 0;
    if (microseconds_txt && *microseconds_txt)
        microseconds = str2ull(microseconds_txt);

    if (((PARSER_USER_OBJECT *) user)->plugins_action->begin_action) {
        return ((PARSER_USER_OBJECT *) user)->plugins_action->begin_action(user, id, microseconds);
    }

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!id)) {
        error("requested a BEGIN without a chart id for host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

    st = rrdset_find(host, id);
    if (unlikely(!st)) {
        error("requested a BEGIN on chart '%s', which does not exist on host '%s'. Disabling it.", id, host->hostname);
        goto disable;
    }

    if (likely(st->counter_done)) {
        if (likely(microseconds)) {
            if (((PARSER_USER_OBJECT *) user)->trust_durations)
                rrdset_next_usec_unfiltered(st, microseconds);
            else
                rrdset_next_usec(st, microseconds);
        } else
            rrdset_next(st);
    }
    ((PARSER_USER_OBJECT *) user)->st = st;
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_end(char **words, void *user)
{
    UNUSED(words);

    if (((PARSER_USER_OBJECT *) user)->plugins_action->end_action) {
        return ((PARSER_USER_OBJECT *) user)->plugins_action->end_action(user);
    }

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!st)) {
        error("requested an END, without a BEGIN on host '%s'. Disabling it.", host->hostname);
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(D_PLUGINSD, "requested an END on chart %s", st->id);

    rrdset_done(st);
    ((PARSER_USER_OBJECT *) user)->st = NULL;

    ((PARSER_USER_OBJECT *) user)->count++;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_chart(char **words, void *user)
{
    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

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

//    if (((PARSER_USER_OBJECT *) user)->plugins_action->chart_action) {
//        return ((PARSER_USER_OBJECT *) user)->plugins_action->chart_action(user, name, title, units, family,
//                                                                          context, chart_type, priority, update_every,
//                                                                          options, plugin, module);
//    }

    // parse the id from type
    char *id = NULL;
    if (likely(type && (id = strchr(type, '.')))) {
        *id = '\0';
        id++;
    }

    // make sure we have the required variables
    if (unlikely(!type || !*type || !id || !*id)) {
        error("requested a CHART, without a type.id, on host '%s'. Disabling it.", host->hostname);
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    // parse the name, and make sure it does not include 'type.'
    if (unlikely(name && *name)) {
        // when data are coming from slaves
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

    st = rrdset_create(
        host, type, id, name, family, context, title, units, (plugin && *plugin) ? plugin : ((PARSER_USER_OBJECT *) user)->cd->filename, module,
        priority, update_every, chart_type);

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
    ((PARSER_USER_OBJECT *) user)->st = st;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_dimension(char **words, void *user)
{
    char *id = words[1];
    char *name = words[2];
    char *algorithm = words[3];
    char *multiplier_s = words[4];
    char *divisor_s = words[5];
    char *options = words[6];

    long multiplier = 1;
    if (multiplier_s && *multiplier_s)
        multiplier = strtol(multiplier_s, NULL, 0);
    if (unlikely(!multiplier))
        multiplier = 1;

    long divisor = 1;
    if (likely(divisor_s && *divisor_s))
        divisor = strtol(divisor_s, NULL, 0);
    if (unlikely(!divisor))
        divisor = 1;

    if (unlikely(!algorithm || !*algorithm))
        algorithm = "absolute";

    if (((PARSER_USER_OBJECT *) user)->plugins_action->dimension_action) {
        return ((PARSER_USER_OBJECT *) user)->plugins_action->dimension_action(
                user, (!id || !*id) ? NULL : id, name, algorithm,
            multiplier, divisor, (options && *options)?options:NULL, rrd_algorithm_id(algorithm));
    }

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(!id || !*id)) {
        error(
            "requested a DIMENSION, without an id, host '%s' and chart '%s'. Disabling it.", host->hostname,
            st ? st->id : "UNSET");
        goto disable;
    }

    if (unlikely(!st)) {
        error("requested a DIMENSION, without a CHART, on host '%s'. Disabling it.", host->hostname);
        goto disable;
    }

//    long multiplier = 1;
//    if (multiplier_s && *multiplier_s)
//        multiplier = strtol(multiplier_s, NULL, 0);
//    if (unlikely(!multiplier))
//        multiplier = 1;
//
//    long divisor = 1;
//    if (likely(divisor_s && *divisor_s))
//        divisor = strtol(divisor_s, NULL, 0);
//    if (unlikely(!divisor))
//        divisor = 1;

//    if (unlikely(!algorithm || !*algorithm))
//        algorithm = "absolute";

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
        debug(
            D_PLUGINSD,
            "creating dimension in chart %s, id='%s', name='%s', algorithm='%s', multiplier=%ld, divisor=%ld, hidden='%s'",
            st->id, id, name ? name : "", rrd_algorithm_name(rrd_algorithm_id(algorithm)), multiplier, divisor,
            options ? options : "");

    RRDDIM *rd = rrddim_add(st, id, name, multiplier, divisor, rrd_algorithm_id(algorithm));
    rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
    rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    if (options && *options) {
        if (strstr(options, "obsolete") != NULL)
            rrddim_is_obsolete(st, rd);
        else
            rrddim_isnot_obsolete(st, rd);
        if (strstr(options, "hidden") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
        if (strstr(options, "noreset") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
        if (strstr(options, "nooverflow") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    } else {
        rrddim_isnot_obsolete(st, rd);
    }
    ((PARSER_USER_OBJECT *) user)->st = st;
    return PARSER_RC_OK;

disable:
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_variable(char **words, void *user)
{
    char *name = words[1];
    char *value = words[2];

    RRDSET *st = ((PARSER_USER_OBJECT *) user)->st;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    int global = (st) ? 0 : 1;

    if (((PARSER_USER_OBJECT *) user)->plugins_action->variable_action) {
        if (name && *name) {
            global = 0;
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
        return ((PARSER_USER_OBJECT *) user)->plugins_action->variable_action(user, global, name, value);
    }

    if (unlikely(!name || !*name)) {
        error("requested a VARIABLE on host '%s', without a variable name. Disabling it.", host->hostname);
        ((PARSER_USER_OBJECT *) user)->enabled = 0;
        return PARSER_RC_ERROR;
    }

    if (name && *name) {
        global = 0;
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

    if (unlikely(!value || !*value))
        value = NULL;

    if (value) {
        char *endptr = NULL;
        calculated_number v = (calculated_number)str2ld(value, &endptr);

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

        if (global) {
            RRDVAR *rv = rrdvar_custom_host_variable_create(host, name);
            if (rv)
                rrdvar_custom_host_variable_set(host, rv, v);
            else
                error("cannot find/create HOST VARIABLE '%s' on host '%s'", name, host->hostname);
        } else if (st) {
            RRDSETVAR *rs = rrdsetvar_custom_chart_variable_create(st, name);
            if (rs)
                rrdsetvar_custom_chart_variable_set(rs, v);
            else
                error("cannot find/create CHART VARIABLE '%s' on host '%s', chart '%s'", name, host->hostname, st->id);
        } else
            error("cannot find/create CHART VARIABLE '%s' on host '%s' without a chart", name, host->hostname);
    } else
        error(
            "cannot set %s VARIABLE '%s' on host '%s' to an empty value", (global) ? "HOST" : "CHART", name,
            host->hostname);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_flush(char **words, void *user)
{
    UNUSED(words);

    if (((PARSER_USER_OBJECT *)user)->plugins_action->flush_action) {
        return ((PARSER_USER_OBJECT *)user)->plugins_action->flush_action(user);
    }

    debug(D_PLUGINSD, "requested a FLUSH");

    ((PARSER_USER_OBJECT *) user)->st = NULL;
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_disable(char **words, void *user)
{
    UNUSED(user);
    UNUSED(words);

    if (((PARSER_USER_OBJECT *)user)->plugins_action->disable_action) {
        return ((PARSER_USER_OBJECT *)user)->plugins_action->disable_action(user);
    }
    info("called DISABLE. Disabling it.");
    ((PARSER_USER_OBJECT *) user)->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC pluginsd_label(char **words, void *user)
{
    debug(D_PLUGINSD, "requested a LABEL CHANGE");
    char *store;
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

    ((PARSER_USER_OBJECT *) user)->new_labels = add_label_to_list(((PARSER_USER_OBJECT *) user)->new_labels, words[1], store, strtol(words[2], NULL, 10));
    if (store != words[3])
        freez(store);
    return PARSER_RC_OK;
}

PARSER_RC pluginsd_overwrite(char **words, void *user)
{
    UNUSED(words);

    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    debug(D_PLUGINSD, "requested a OVERWITE a variable");
    if (!host->labels) {
        host->labels = ((PARSER_USER_OBJECT *) user)->new_labels;
    } else {
        rrdhost_rdlock(host);
        replace_label_list(host, ((PARSER_USER_OBJECT *) user)->new_labels);
        rrdhost_unlock(host);
    }

    ((PARSER_USER_OBJECT *) user)->new_labels = NULL;
    return PARSER_RC_OK;
}


// New plugins.d parser

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

    PARSER_USER_OBJECT *user = callocz(1, sizeof(*user));
    ((PARSER_USER_OBJECT *) user)->enabled = cd->enabled;
    ((PARSER_USER_OBJECT *) user)->host = host;
    ((PARSER_USER_OBJECT *) user)->cd = cd;
    ((PARSER_USER_OBJECT *) user)->trust_durations = trust_durations;

    PARSER *parser = parser_init(host, user, fp, PARSER_INPUT_SPLIT | PARSER_INPUT_ORIGINAL);

    if (unlikely(!parser)) {
        error("Failed to initialize parser");
        cd->serial_failures++;
        return 0;
    }

    user->plugins_action = callocz(1, sizeof(PLUGINSD_ACTION));
    //user->plugins_action->begin_action = &pluginsd_begin_action;

    int rc = parser_add_keyword(parser, PLUGINSD_KEYWORD_FLUSH, pluginsd_flush);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_CONTEXT, pluginsd_context);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_GUID, pluginsd_guid);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_HOST, pluginsd_host);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_TOMBSTONE, pluginsd_tombstone);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_CHART, pluginsd_chart);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_DIMENSION, pluginsd_dimension);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_DISABLE, pluginsd_disable);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_VARIABLE, pluginsd_variable);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_LABEL, pluginsd_label);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_OVERWRITE, pluginsd_overwrite);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_END, pluginsd_end);
    rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_BEGIN, pluginsd_begin);
    rc += parser_add_keyword(parser, "SET", pluginsd_set);

    info("Registered %d keywords for the parser", rc);
    user->parser = parser;

    while (likely(!parser_next(parser))) {
        if (unlikely(netdata_exit || parser_action(parser,  NULL)))
            break;
    }
    info("PARSER ended");

    parser_destroy(parser);

    cd->enabled = ((PARSER_USER_OBJECT *) user)->enabled;
    size_t count = ((PARSER_USER_OBJECT *) user)->count;

    freez(user->plugins_action);
    freez(user);

    if (likely(count)) {
        cd->successful_collections += count;
        cd->serial_failures = 0;
    } else
        cd->serial_failures++;

    return count;
}

