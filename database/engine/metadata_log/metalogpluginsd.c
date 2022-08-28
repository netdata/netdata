// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"
#include "metalogpluginsd.h"

extern struct config stream_config;

PARSER_RC metalog_pluginsd_host_action(
    void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os, char *timezone,
    char *tags)
{
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;

    RRDHOST *host = rrdhost_find_by_guid(machine_guid);
    if (host) {
        if (unlikely(host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)) {
            error("Archived host '%s' has memory mode '%s', but the archived one is '%s'. Ignoring archived state.",
                  rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode),
                  rrd_memory_mode_name(RRD_MEMORY_MODE_DBENGINE));
            ((PARSER_USER_OBJECT *) user)->host = NULL; /* Ignore objects if memory mode is not dbengine */
        }
        ((PARSER_USER_OBJECT *) user)->host = host;
        return PARSER_RC_OK;
    }

    if (strcmp(machine_guid, registry_get_this_machine_guid()) == 0) {
        ((PARSER_USER_OBJECT *) user)->host = host;
        return PARSER_RC_OK;
    }

    if (likely(!uuid_parse(machine_guid, state->host_uuid))) {
        int rc = sql_store_host(&state->host_uuid, hostname, registry_hostname, update_every, os, timezone, tags, 1);
        if (unlikely(rc)) {
            errno = 0;
            error("Failed to store host %s with UUID %s in the database", hostname, machine_guid);
        }
    }
    else {
        errno = 0;
        error("Host machine GUID %s is not valid", machine_guid);
    }

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family, char *context,
                                        char *title, char *units, char *plugin, char *module, int priority,
                                        int update_every, RRDSET_TYPE chart_type, char *options)
{
    UNUSED(options);

    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    RRDHOST *host = ((PARSER_USER_OBJECT *) user)->host;

    if (unlikely(uuid_is_null(state->host_uuid))) {
        debug(D_METADATALOG, "Ignoring chart belonging to missing or ignored host.");
        return PARSER_RC_OK;
    }
    uuid_copy(state->chart_uuid, state->uuid);
    uuid_clear(state->uuid); /* Consume UUID */
    (void) sql_store_chart(&state->chart_uuid, &state->host_uuid,
        type, id, name, family, context, title, units,
        plugin, module, priority, update_every,
        chart_type, RRD_MEMORY_MODE_DBENGINE, host ? host->rrd_history_entries : 1);
    ((PARSER_USER_OBJECT *)user)->st_exists = 1;

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                            long multiplier, long divisor, char *options, RRD_ALGORITHM algorithm_type)
{
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;
    UNUSED(user);
    UNUSED(options);
    UNUSED(algorithm);
    UNUSED(st);

    if (unlikely(uuid_is_null(state->chart_uuid))) {
        debug(D_METADATALOG, "Ignoring dimension belonging to missing or ignored chart.");
        info("Ignoring dimension belonging to missing or ignored chart.");
        return PARSER_RC_OK;
    }

    if (unlikely(uuid_is_null(state->uuid))) {
        debug(D_METADATALOG, "Ignoring dimension without unknown UUID");
        info("Ignoring dimension without unknown UUID");
        return PARSER_RC_OK;
    }

    (void) sql_store_dimension(&state->uuid, &state->chart_uuid, id, name, multiplier, divisor, algorithm_type);
    uuid_clear(state->uuid); /* Consume UUID */

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
    struct metalog_pluginsd_state *state = ((PARSER_USER_OBJECT *)user)->private;

    int rc = find_uuid_type(uuid);

    if (rc == 1) {
        uuid_copy(state->host_uuid, *uuid);
        ((PARSER_USER_OBJECT *)user)->st_exists = 0;
        ((PARSER_USER_OBJECT *)user)->host_exists = 1;
    } else if (rc == 2) {
        uuid_copy(state->chart_uuid, *uuid);
        ((PARSER_USER_OBJECT *)user)->st_exists = 1;
    } else
        uuid_copy(state->uuid, *uuid);

    return PARSER_RC_OK;
}

PARSER_RC metalog_pluginsd_tombstone_action(void *user, uuid_t *uuid)
{
    UNUSED(user);
    UNUSED(uuid);

    return PARSER_RC_OK;
}

void metalog_pluginsd_state_init(struct metalog_pluginsd_state *state, struct metalog_instance *ctx)
{
    state->ctx = ctx;
    state->skip_record = 0;
    uuid_clear(state->uuid);
    state->metalogfile = NULL;
}
