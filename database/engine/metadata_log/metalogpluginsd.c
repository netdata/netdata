// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"
#include "metalogpluginsd.h"

extern struct config stream_config;

PARSER_RC metalog_pluginsd_host_action(
    void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os, char *timezone,
    char *tags)
{
    int history = 5;
    RRD_MEMORY_MODE mode = RRD_MEMORY_MODE_DBENGINE;
    int rrdpush_enabled = default_rrdpush_enabled;
    char *rrdpush_destination = default_rrdpush_destination;
    char *rrdpush_api_key = default_rrdpush_api_key;
    char *rrdpush_send_charts_matching = default_rrdpush_send_charts_matching;

    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;

    RRDHOST *host = rrdhost_find_by_guid(machine_guid, 0);
    if (host) {
        if (unlikely(host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)) {
            error("Archived host '%s' has memory mode '%s', but the archived one is '%s'. Ignoring archived state.",
                  host->hostname, rrd_memory_mode_name(host->rrd_memory_mode),
                  rrd_memory_mode_name(RRD_MEMORY_MODE_DBENGINE));
            ((PARSER_USER_OBJECT *) user)->host = NULL; /* Ignore objects if memory mode is not dbengine */
            return PARSER_RC_OK;
        }
        goto write_replay;
    }

    if (strcmp(machine_guid, registry_get_this_machine_guid()) == 0) {
        struct metalog_record record;
        struct metadata_logfile *metalogfile = state->metalogfile;

        uuid_parse(machine_guid, record.uuid);
        mlf_record_insert(metalogfile, &record);
        if (localhost->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            ((PARSER_USER_OBJECT *) user)->host = localhost;
        else
            ((PARSER_USER_OBJECT *) user)->host = NULL;
        return PARSER_RC_OK;
    }

    // Fetch configuration options from streaming config
    update_every = (int)appconfig_get_number(&stream_config, machine_guid, "update every", update_every);
    if(update_every < 0) update_every = 1;

    //rrdpush_enabled = appconfig_get_boolean(&stream_config, rpt->key, "default proxy enabled", rrdpush_enabled);
    rrdpush_enabled = appconfig_get_boolean(&stream_config, machine_guid, "proxy enabled", rrdpush_enabled);

    //rrdpush_destination = appconfig_get(&stream_config, rpt->key, "default proxy destination", rrdpush_destination);
    rrdpush_destination = appconfig_get(&stream_config, machine_guid, "proxy destination", rrdpush_destination);

    //rrdpush_api_key = appconfig_get(&stream_config, rpt->key, "default proxy api key", rrdpush_api_key);
    rrdpush_api_key = appconfig_get(&stream_config, machine_guid, "proxy api key", rrdpush_api_key);

    //rrdpush_send_charts_matching = appconfig_get(&stream_config, rpt->key, "default proxy send charts matching", rrdpush_send_charts_matching);
    rrdpush_send_charts_matching = appconfig_get(&stream_config, machine_guid, "proxy send charts matching", rrdpush_send_charts_matching);


    host = rrdhost_create(
        hostname
        , registry_hostname
        , machine_guid
        , os
        , timezone
        , tags
        , NULL
        , NULL
        , update_every
        , history   // entries
        , mode
        , 0    // health enabled
        , rrdpush_enabled   // Push enabled
        , rrdpush_destination  //destination
        , rrdpush_api_key  // api key
        , rrdpush_send_charts_matching  // charts matching
        , callocz(1, sizeof(struct rrdhost_system_info))
        , 0     // localhost
        , 1     // archived
    );

write_replay:
    if (host) { /* It's a valid object */
        struct metalog_record record;
        struct metadata_logfile *metalogfile = state->metalogfile;

        uuid_copy(record.uuid, host->host_uuid);
        mlf_record_insert(metalogfile, &record);
    }
    ((PARSER_USER_OBJECT *) user)->host = host;
    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context,
                                        char *title, char *units, char *plugin, char *module, int priority,
                                        int update_every, RRDSET_TYPE chart_type, char *options)
{
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    RRDSET *st = NULL;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;
    uuid_t *chart_uuid;

    if (unlikely(!host)) {
        debug(D_METADATALOG, "Ignoring chart belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }
    chart_uuid = uuid_is_null(state->uuid) ? NULL : &state->uuid;
    st = rrdset_create_custom(
        host, type, id, name, family, context, title, units,
        plugin, module, priority, update_every,
        chart_type, RRD_MEMORY_MODE_DBENGINE, (host)->rrd_history_entries, 1, chart_uuid);

    rrdset_isnot_obsolete(st); /* archived charts cannot be obsolete */
    if (options && *options) {
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
        rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
        rrdset_flag_clear(st, RRDSET_FLAG_STORE_FIRST);
    }
    ((PARSER_USER_OBJECT *)user)->st = st;

    if (chart_uuid) { /* It's a valid object */
        struct metalog_record record;
        struct metadata_logfile *metalogfile = state->metalogfile;

        uuid_copy(record.uuid, state->uuid);
        mlf_record_insert(metalogfile, &record);
        uuid_clear(state->uuid); /* Consume UUID */
    }
    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                            long multiplier, long divisor, char *options, RRD_ALGORITHM algorithm_type)
{
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    UNUSED(user);
    UNUSED(algorithm);
    uuid_t *dim_uuid;

    if (unlikely(!st)) {
        debug(D_METADATALOG, "Ignoring dimension belonging to missing or ignored chart.");
        return PARSER_RC_OK;
    }
    dim_uuid = uuid_is_null(state->uuid) ? NULL : &state->uuid;

    RRDDIM *rd = rrddim_add_custom(st, id, name, multiplier, divisor, algorithm_type, RRD_MEMORY_MODE_DBENGINE, 1,
                                   dim_uuid);
    rrddim_flag_clear(rd, RRDDIM_FLAG_HIDDEN);
    rrddim_flag_clear(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    rrddim_isnot_obsolete(st, rd); /* archived dimensions cannot be obsolete */
    if (options && *options) {
        if (strstr(options, "hidden") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_HIDDEN);
        if (strstr(options, "noreset") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
        if (strstr(options, "nooverflow") != NULL)
            rrddim_flag_set(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS);
    }
    if (dim_uuid) { /* It's a valid object */
        struct metalog_record record;
        struct metadata_logfile *metalogfile = state->metalogfile;

        uuid_copy(record.uuid, state->uuid);
        mlf_record_insert(metalogfile, &record);
        uuid_clear(state->uuid); /* Consume UUID */
    }
    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_guid_action(void *user, uuid_t *uuid)
{
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;

    uuid_copy(state->uuid, *uuid);

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_context_action(void *user, uuid_t *uuid)
{
    GUID_TYPE ret;
    //struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    //struct metalog_instance *ctx = state->ctx;
    char object[49], chart_object[33], id_str[1024];
    uuid_t *chart_guid, *chart_char_guid;
    RRDHOST *host;

    ret = find_object_by_guid(uuid, object, 49);
    switch (ret) {
        case GUID_TYPE_NOTFOUND:
            error_with_guid(uuid, "Failed to find valid context");
            break;
        case GUID_TYPE_CHAR:
            error_with_guid(uuid, "Ignoring unexpected type GUID_TYPE_CHAR");
            break;
        case GUID_TYPE_CHART:
        case GUID_TYPE_DIMENSION:
            host = metalog_get_host_from_uuid(NULL, (uuid_t *) &object);
            if (unlikely(!host))
                break;
            switch (ret) {
                case GUID_TYPE_CHART:
                    chart_char_guid = (uuid_t *)(object + 16);

                    ret = find_object_by_guid(chart_char_guid, id_str, RRD_ID_LENGTH_MAX + 1);
                    if (unlikely(GUID_TYPE_CHAR != ret))
                        error_with_guid(uuid, "Failed to find valid chart name");
                    else
                        ((PARSER_USER_OBJECT *)user)->st = rrdset_find(host, id_str);
                    break;
                case GUID_TYPE_DIMENSION:
                    chart_guid = (uuid_t *)(object + 16);

                    ret = find_object_by_guid(chart_guid, chart_object, 33);
                    if (unlikely(GUID_TYPE_CHART != ret)) {
                        error_with_guid(uuid, "Failed to find valid chart");
                        break;
                    }
                    chart_char_guid = (uuid_t *)(object + 16);

                    ret = find_object_by_guid(chart_char_guid, id_str, RRD_ID_LENGTH_MAX + 1);
                    if (unlikely(GUID_TYPE_CHAR != ret))
                        error_with_guid(uuid, "Failed to find valid chart name");
                    else
                        ((PARSER_USER_OBJECT *)user)->st = rrdset_find(host, id_str);
                    break;
                default:
                    break;
            }
            break;
        case GUID_TYPE_HOST:
            ((PARSER_USER_OBJECT *)user)->host = metalog_get_host_from_uuid(NULL, (uuid_t *) &object);
            break;
        case GUID_TYPE_NOSPACE:
            error_with_guid(uuid, "Not enough space for object retrieval");
            break;
        default:
            error("Unknown return code %u from find_object_by_guid", ret);
            break;
    }

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_tombstone_action(void *user, uuid_t *uuid)
{
    GUID_TYPE ret;
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    struct metalog_instance *ctx = state->ctx;
    RRDHOST *host = NULL;
    RRDSET *st;
    RRDDIM *rd;

    ret = find_object_by_guid(uuid, NULL, 0);
    switch (ret) {
        case GUID_TYPE_CHAR:
            fatal_assert(0);
            break;
        case GUID_TYPE_CHART:
            st = metalog_get_chart_from_uuid(ctx, uuid);
            if (st) {
                host = st->rrdhost;
                rrdhost_wrlock(host);
                rrdset_free(st);
                rrdhost_unlock(host);
            } else {
                debug(D_METADATALOG, "Ignoring nonexistent chart metadata record.");
            }
            break;
        case GUID_TYPE_DIMENSION:
            rd = metalog_get_dimension_from_uuid(ctx, uuid);
            if (rd) {
                st = rd->rrdset;
                rrdset_wrlock(st);
                rrddim_free_custom(st, rd, 0);
                rrdset_unlock(st);
            }
            else {
                debug(D_METADATALOG, "Ignoring nonexistent dimension metadata record.");
            }
            break;
        case GUID_TYPE_HOST:
            /* Ignore for now */
            break;
        default:
            break;
    }

    return PARSER_RC_OK;
}

void metalog_pluginsd_state_init(struct metalog_pluginsd_state *state, struct metalog_instance *ctx)
{
    state->ctx = ctx;
    state->skip_record = 0;
    uuid_clear(state->uuid);
    state->metalogfile = NULL;
}