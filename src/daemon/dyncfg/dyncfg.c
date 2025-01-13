// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

struct dyncfg_globals dyncfg_globals = { 0 };

RRDHOST *dyncfg_rrdhost_by_uuid(ND_UUID *uuid) {
    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(uuid->uuid, uuid_str);

    RRDHOST *host = rrdhost_find_by_guid(uuid_str);
    if(!host)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot find host with UUID '%s'", uuid_str);

    return host;
}

RRDHOST *dyncfg_rrdhost(DYNCFG *df) {
    return dyncfg_rrdhost_by_uuid(&df->host_uuid);
}

void dyncfg_cleanup(DYNCFG *v) {
    string_freez(v->dyncfg.source);
    v->dyncfg.source = NULL;

    buffer_free(v->dyncfg.payload);
    v->dyncfg.payload = NULL;

    string_freez(v->path);
    v->path = NULL;

    string_freez(v->current.source);
    v->current.source = NULL;

    string_freez(v->function);
    v->function = NULL;

    string_freez(v->template);
    v->template = NULL;
}

static void dyncfg_normalize(DYNCFG *df) {
    usec_t now_ut = now_realtime_usec();

    if(!df->current.created_ut)
        df->current.created_ut = now_ut;

    if(!df->current.modified_ut)
        df->current.modified_ut = now_ut;
}

static void dyncfg_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *df = value;
    dyncfg_cleanup(df);
}

static void dyncfg_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    DYNCFG *df = value;
    dyncfg_normalize(df);

    const char *id = dictionary_acquired_item_name(item);
    char buf[strlen(id) + 20];
    snprintfz(buf, sizeof(buf), PLUGINSD_FUNCTION_CONFIG " %s", id);
    df->function = string_strdupz(buf);

    if(df->type == DYNCFG_TYPE_JOB && !df->template) {
        const char *last_colon = strrchr(id, ':');
        if(last_colon)
            df->template = string_strndupz(id, last_colon - id);
        else
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "DYNCFG: id '%s' is a job, but does not contain a colon to find the template", id);
    }
}

static void dyncfg_react_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *df = value; (void)df;
    ;
}

static bool dyncfg_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    bool *overwrite_cb_ptr = data;
    bool overwrite_cb = (overwrite_cb_ptr && *overwrite_cb_ptr);

    DYNCFG *v = old_value;
    DYNCFG *nv = new_value;

    size_t changes = 0;

    dyncfg_normalize(nv);

    if(!UUIDeq(v->host_uuid, nv->host_uuid)) {
        SWAP(v->host_uuid, nv->host_uuid);
        changes++;
    }

    if(v->path != nv->path) {
        SWAP(v->path, nv->path);
        changes++;
    }

    if(v->cmds != nv->cmds) {
        SWAP(v->cmds, nv->cmds);
        changes++;
    }

    if(v->type != nv->type) {
        SWAP(v->type, nv->type);
        changes++;
    }

    if(v->view_access != nv->view_access) {
        SWAP(v->view_access, nv->view_access);
        changes++;
    }

    if(v->edit_access != nv->edit_access) {
        SWAP(v->edit_access, nv->edit_access);
        changes++;
    }

    if(v->current.status != nv->current.status) {
        SWAP(v->current.status, nv->current.status);
        changes++;
    }

    if (v->current.source_type != nv->current.source_type) {
        SWAP(v->current.source_type, nv->current.source_type);
        changes++;
    }

    if (v->current.source != nv->current.source) {
        SWAP(v->current.source, nv->current.source);
        changes++;
    }

    if(nv->current.created_ut < v->current.created_ut) {
        SWAP(v->current.created_ut, nv->current.created_ut);
        changes++;
    }

    if(nv->current.modified_ut > v->current.modified_ut) {
        SWAP(v->current.modified_ut, nv->current.modified_ut);
        changes++;
    }

    if(!v->execute_cb || (overwrite_cb && nv->execute_cb && (v->execute_cb != nv->execute_cb || v->execute_cb_data != nv->execute_cb_data))) {
        v->sync = nv->sync,
        v->execute_cb = nv->execute_cb;
        v->execute_cb_data = nv->execute_cb_data;
        changes++;
    }

    dyncfg_cleanup(nv);

    return changes > 0;
}

// ----------------------------------------------------------------------------

void dyncfg_init_low_level(bool load_saved) {
    if(!dyncfg_globals.nodes) {
        dyncfg_globals.nodes = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, &dictionary_stats_category_dyncfg, sizeof(DYNCFG));
        dictionary_register_insert_callback(dyncfg_globals.nodes, dyncfg_insert_cb, NULL);
        dictionary_register_react_callback(dyncfg_globals.nodes, dyncfg_react_cb, NULL);
        dictionary_register_conflict_callback(dyncfg_globals.nodes, dyncfg_conflict_cb, NULL);
        dictionary_register_delete_callback(dyncfg_globals.nodes, dyncfg_delete_cb, NULL);

        char path[PATH_MAX];
        snprintfz(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, "config");

        if(mkdir(path, 0755) == -1) {
            if(errno != EEXIST)
                nd_log(NDLS_DAEMON, NDLP_CRIT, "DYNCFG: failed to create dynamic configuration directory '%s'", path);
        }

        dyncfg_globals.dir = strdupz(path);

        if(load_saved)
            dyncfg_load_all();
    }
}

// ----------------------------------------------------------------------------

const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path,
                                           DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type,
                                           const char *source, DYNCFG_CMDS cmds,
                                           usec_t created_ut, usec_t modified_ut,
                                           bool sync, HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                                           rrd_function_execute_cb_t execute_cb, void *execute_cb_data,
                                           bool overwrite_cb) {
    DYNCFG tmp = {
        .host_uuid = host->host_id,
        .path = string_strdupz(path),
        .cmds = cmds,
        .type = type,
        .view_access = view_access,
        .edit_access = edit_access,
        .current = {
            .status = status,
            .source_type = source_type,
            .source = string_strdupz(source),
            .created_ut = created_ut,
            .modified_ut = modified_ut,
        },
        .sync = sync,
        .dyncfg = { 0 },
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
    };

    return dictionary_set_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1, &tmp, sizeof(tmp), &overwrite_cb);
}

static void dyncfg_send_updates(const char *id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: asked to update plugin for configuration '%s', but it is not found.", id);
        return;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);

    if(df->type == DYNCFG_TYPE_SINGLE || df->type == DYNCFG_TYPE_JOB) {
        if (df->cmds & DYNCFG_CMD_UPDATE && df->dyncfg.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && df->dyncfg.payload && buffer_strlen(df->dyncfg.payload))
            dyncfg_echo_update(item, df, id);
    }
    else if(df->type == DYNCFG_TYPE_TEMPLATE && (df->cmds & DYNCFG_CMD_ADD)) {
        STRING *template = string_strdupz(id);

        size_t len = strlen(id);
        DYNCFG *df_job;
        dfe_start_reentrant(dyncfg_globals.nodes, df_job) {
            const char *id_template = df_job_dfe.name;
            if(df_job->type == DYNCFG_TYPE_JOB &&                   // it is a job
                df_job->current.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && // it is dynamically configured
                df_job->template == template &&                     // it has the same template name
                strncmp(id_template, id, len) == 0 &&               // the template name matches (redundant)
                id_template[len] == ':' &&                          // immediately after the template there is ':'
                id_template[len + 1]) {                             // and there is something else after the ':'
                dyncfg_echo_add(item, df_job_dfe.item, df, df_job, id, &id_template[len + 1]);
            }
        }
        dfe_done(df_job);

        string_freez(template);
    }

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
}

bool dyncfg_is_user_disabled(const char *id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id);
    if(!item)
        return false;

    DYNCFG *df = dictionary_acquired_item_value(item);
    bool ret = df->dyncfg.user_disabled;
    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return ret;
}

bool dyncfg_job_has_registered_template(const char *id) {
    char buf[strlen(id) + 1];
    memcpy(buf, id, sizeof(buf));
    char *colon = strrchr(buf, ':');
    if(!colon)
        return false;

    *colon = '\0';
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, buf);
    if(!item)
        return false;

    DYNCFG *df = dictionary_acquired_item_value(item);
    bool ret = df->type == DYNCFG_TYPE_TEMPLATE;

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return ret;
}

bool dyncfg_add_low_level(RRDHOST *host, const char *id, const char *path,
                          DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source,
                          DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync,
                          HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                          rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {

    if(view_access == HTTP_ACCESS_NONE)
        view_access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG;

    if(edit_access == HTTP_ACCESS_NONE)
        edit_access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_AGENT_CONFIG | HTTP_ACCESS_COMMERCIAL_SPACE;

    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return false;
    }

    if(type == DYNCFG_TYPE_JOB && !dyncfg_job_has_registered_template(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: job id '%s' does not have a registered template. Ignoring dynamic configuration for it.", id);
        return false;
    }

    DYNCFG_CMDS old_cmds = cmds;

    // all configurations support schema
    cmds |= DYNCFG_CMD_SCHEMA;

    // if there is either enable or disable, both are supported
    if(cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
        cmds |= DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE;

    // add
    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates must always support "add"
        cmds |= DYNCFG_CMD_ADD;
    }
    else {
        // only templates can have "add"
        cmds &= ~DYNCFG_CMD_ADD;
    }

    // remove
    if(source_type != DYNCFG_SOURCE_TYPE_DYNCFG || type != DYNCFG_TYPE_JOB) {
        // remove is only available for dyncfg jobs
        cmds &= ~DYNCFG_CMD_REMOVE;
    }

    // data
    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates do not have data
        cmds &= ~(DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE);
    }

    if(cmds != old_cmds) {
        CLEAN_BUFFER *t = buffer_create(1024, NULL);
        buffer_sprintf(t, "DYNCFG: id '%s' was declared with cmds: ", id);
        dyncfg_cmds2buffer(old_cmds, t);
        buffer_strcat(t, ", but they have sanitized to: ");
        dyncfg_cmds2buffer(cmds, t);
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "%s", buffer_tostring(t));
    }

    const DICTIONARY_ITEM *item = dyncfg_add_internal(host, id, path, status, type, source_type, source, cmds,
                                                      created_ut, modified_ut, sync, view_access, edit_access,
                                                      execute_cb, execute_cb_data, true);
    DYNCFG *df = dictionary_acquired_item_value(item);

//    if(df->source_type == DYNCFG_SOURCE_TYPE_DYNCFG && !df->saves)
//        nd_log(NDLS_DAEMON, NDLP_WARNING, "DYNCFG: configuration '%s' is created with source type dyncfg, but we don't have a saved configuration for it", id);

    rrd_collector_started();
    rrd_function_add(
        host,
        NULL,
        string2str(df->function),
        120,
        1000,
        DYNCFG_FUNCTIONS_VERSION,
        "Dynamic configuration",
        "config",
        (view_access & edit_access),
        sync,
        dyncfg_function_intercept_cb,
        NULL);

    if(df->type != DYNCFG_TYPE_TEMPLATE && (df->cmds & (DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE))) {
        DYNCFG_CMDS status_to_send_to_plugin =
            (df->dyncfg.user_disabled || df->current.status == DYNCFG_STATUS_DISABLED) ? DYNCFG_CMD_DISABLE : DYNCFG_CMD_ENABLE;

        if (status_to_send_to_plugin == DYNCFG_CMD_ENABLE && dyncfg_is_user_disabled(string2str(df->template)))
            status_to_send_to_plugin = DYNCFG_CMD_DISABLE;

        dyncfg_echo(item, df, id, status_to_send_to_plugin);
    }

    if(!(df->current.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && df->type == DYNCFG_TYPE_JOB))
        dyncfg_send_updates(id);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);

    return true;
}

void dyncfg_del_low_level(RRDHOST *host, const char *id) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return;
    }

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);
        rrd_function_del(host, NULL, string2str(df->function));

        bool garbage_collect = false;
        if(df->dyncfg.saves == 0) {
            dictionary_del(dyncfg_globals.nodes, id);
            garbage_collect = true;
        }

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);

        if(garbage_collect)
            dictionary_garbage_collect(dyncfg_globals.nodes);
    }
}

void dyncfg_status_low_level(RRDHOST *host __maybe_unused, const char *id, DYNCFG_STATUS status) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return;
    }

    if(status == DYNCFG_STATUS_NONE) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: status provided to id '%s' is invalid. Ignoring it.", id);
        return;
    }

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dyncfg_globals.nodes, id);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);
        df->current.status = status;
        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }
}

// ----------------------------------------------------------------------------

void dyncfg_add_streaming(BUFFER *wb) {
    // when sending config functions to parents, we send only 1 function called 'config';
    // the parent will send the command to the child, and the child will validate it;
    // this way the parent does not need to receive removals of config functions;

    buffer_sprintf(wb
                   , PLUGINSD_KEYWORD_FUNCTION " GLOBAL " PLUGINSD_FUNCTION_CONFIG " %d \"%s\" \"%s\" "HTTP_ACCESS_FORMAT" %d\n"
                   , 120
                   , "Dynamic configuration"
                   , "config"
                   , (unsigned)HTTP_ACCESS_ANONYMOUS_DATA
                   , 1000
    );
}

bool dyncfg_available_for_rrdhost(RRDHOST *host) {
    if(host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
        return true;

    return rrd_function_available(host, PLUGINSD_FUNCTION_CONFIG);
}

// ----------------------------------------------------------------------------

