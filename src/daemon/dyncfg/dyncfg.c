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

    // no add-path caller supplies timestamps any more (they are always zero
    // there); these guards still serve the file-load insert path
    if(!df->current.created_ut)
        df->current.created_ut = now_ut;

    if(!df->current.modified_ut)
        df->current.modified_ut = now_ut;
}

static void dyncfg_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *df = value;

    // the ONLY release site for the node's transport pin: dyncfg_cleanup()
    // also serves conflict losers and file-load error paths (which never
    // held a pin), and dyncfg_shutdown_low_level()'s dictionary_destroy fires
    // this callback per node - an explicit release there would double-release
    if(df->transport) {
        nrpc_transport_entry_release(df->transport);
        df->transport = NULL;
    }

    dyncfg_cleanup(df);
}

static void dyncfg_insert_cb(const DICTIONARY_ITEM *item, void *value, void *data __maybe_unused) {
    DYNCFG *df = value;
    dyncfg_normalize(df);

    spinlock_init(&df->transport_spinlock);

    // the tmp carried the plugin transport RAW (unpinned); the pin belongs to
    // the NODE and is taken here, inside the insert callback - never on the
    // tmp - so conflict losers hold nothing to leak. entry-acquire returns its
    // argument (NULL-safe, fatals on use-after-release); storing the returned
    // pointer keeps the sites that fill a dedicated transport field shaped the
    // same way (dyncfg_conflict_cb, the in-flight cancel/progress hook pins)
    df->transport = nrpc_transport_entry_acquire(df->transport);

    const char *id = dictionary_acquired_item_name(item);
    size_t buf_size = strlen(id) + 20;
    CLEAN_CHAR_P *buf = mallocz(buf_size);
    snprintfz(buf, buf_size, PLUGINSD_FUNCTION_CONFIG " %s", id);
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
        char u1[UUID_STR_LEN], u2[UUID_STR_LEN];
        nd_uuid_unparse_lower(v->host_uuid.uuid, u1);
        nd_uuid_unparse_lower(nv->host_uuid.uuid, u2);

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DYNCFG: configuration '%s' changed host id from '%s' to '%s'",
               dictionary_acquired_item_name(item), u1, u2);

        SWAP(v->host_uuid, nv->host_uuid);
        changes++;
    }

    if(v->path != nv->path) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DYNCFG: configuration '%s' changed path from '%s' to '%s'",
               dictionary_acquired_item_name(item), string2str(v->path), string2str(nv->path));

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

    // the predicate reads v's execute pair outside the node spinlock: safe
    // because conflict callbacks are serialized by the dictionary index write
    // lock (the only writers of these fields), and the concurrent
    // snapshot-readers only read
    if(!v->handler || (overwrite_cb && nv->handler && (v->handler != nv->handler || v->handler_data != nv->handler_data))) {
        // the transfer branch - including the !v->handler rescue arm, where
        // the displaced pin is provably NULL. Under the node's leaf spinlock so
        // a concurrent reader (intercept invocation, template fan-out) either
        // snapshots the old consistent triple or the new one; the DISPLACED
        // pin is released before the new one is installed. nv's transport is
        // the RAW (unpinned) pointer from the tmp, so losers hold nothing.
        spinlock_lock(&v->transport_spinlock);

        if(v->transport)
            nrpc_transport_entry_release(v->transport);
        v->transport = nv->transport ? nrpc_transport_entry_acquire(nv->transport) : NULL;

        v->sync = nv->sync,
        v->handler = nv->handler;
        v->handler_data = nv->handler_data;

        spinlock_unlock(&v->transport_spinlock);
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

void dyncfg_shutdown_low_level(void) {
    if(dyncfg_globals.nodes) {
        dictionary_destroy(dyncfg_globals.nodes);
        dyncfg_globals.nodes = NULL;
    }
    
    freez((void *)dyncfg_globals.dir);
    dyncfg_globals.dir = NULL;
}

// ----------------------------------------------------------------------------

const DICTIONARY_ITEM *dyncfg_add_internal(const struct dyncfg_add_spec *spec, bool overwrite_cb) {
    internal_fatal(!spec->host, "DYNCFG: node addition without a host");

    DYNCFG tmp = {
        .host_uuid = spec->host->host_id,
        .path = string_strdupz(spec->path),
        .cmds = spec->cmds,
        .type = spec->type,
        .view_access = spec->view_access,
        .edit_access = spec->edit_access,
        .current = {
            .status = spec->status,
            .source_type = spec->source_type,
            .source = string_strdupz(spec->source),
            // created_ut/modified_ut stay zero; dyncfg_normalize() stamps
            // them to now on the insert and conflict paths
        },
        .sync = spec->sync,
        .dyncfg = { 0 },
        .handler = spec->handler,
        .handler_data = spec->handler_data,
        .transport = spec->transport,   // RAW - the insert/transfer callbacks pin it
    };

    const DICTIONARY_ITEM *item =
        dictionary_set_and_acquire_item_advanced(dyncfg_globals.nodes, spec->id, -1, &tmp, sizeof(tmp), &overwrite_cb);
    if(unlikely(!item)) {
        // dictionary destroyed - neither the insert nor the conflict callback
        // ran, so tmp's STRINGs are still ours and the RAW transport was never
        // pinned; unwind cleanly (same shape as nrpc_method_register)
        string_freez(tmp.path);
        string_freez(tmp.current.source);
    }
    return item;
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
    CLEAN_CHAR_P *buf = strdupz(id);
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

DYNCFG_CMDS dyncfg_sanitize_cmds(DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, DYNCFG_CMDS cmds) {
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
    if(source_type == DYNCFG_SOURCE_TYPE_DYNCFG && type == DYNCFG_TYPE_JOB) {
        // dyncfg jobs must always be removable
        cmds |= DYNCFG_CMD_REMOVE;
    }
    else {
        // remove is only available for dyncfg jobs
        cmds &= ~DYNCFG_CMD_REMOVE;
    }

    // data
    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates do not have data
        cmds &= ~(DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE);
    }

    return cmds;
}

bool dyncfg_add_low_level(const struct dyncfg_add_spec *spec) {
    // local copy: the HTTP_ACCESS_NONE-to-default mapping and the cmds
    // sanitization below are THIS wrapper's normalization - the intercept
    // path calls dyncfg_add_internal() directly and bypasses both
    struct dyncfg_add_spec s = *spec;

    if(s.view_access == HTTP_ACCESS_NONE)
        s.view_access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG;

    if(s.edit_access == HTTP_ACCESS_NONE)
        s.edit_access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_AGENT_CONFIG | HTTP_ACCESS_COMMERCIAL_SPACE;

    if(!dyncfg_is_valid_id(s.id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", s.id);
        return false;
    }

    if(s.type == DYNCFG_TYPE_JOB && !dyncfg_job_has_registered_template(s.id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: job id '%s' does not have a registered template. Ignoring dynamic configuration for it.", s.id);
        return false;
    }

    DYNCFG_CMDS old_cmds = s.cmds;
    s.cmds = dyncfg_sanitize_cmds(s.type, s.source_type, s.cmds);

    if(s.cmds != old_cmds) {
        CLEAN_BUFFER *t = buffer_create(1024, NULL);
        buffer_sprintf(t, "DYNCFG: id '%s' was declared with cmds: ", s.id);
        dyncfg_cmds2buffer(old_cmds, t);
        buffer_strcat(t, ", but they have sanitized to: ");
        dyncfg_cmds2buffer(s.cmds, t);
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "%s", buffer_tostring(t));
    }

    const DICTIONARY_ITEM *item = dyncfg_add_internal(&s, true);
    if(unlikely(!item)) {
        // the nodes dictionary is destroyed - no callback ran, so the RAW
        // `transport` was never pinned, and dyncfg_add_internal() already
        // unwound its tmp STRINGs; nothing is held here
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DYNCFG: cannot add configuration '%s' - the dyncfg registry is not available", s.id);
        return false;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);

//    if(df->source_type == DYNCFG_SOURCE_TYPE_DYNCFG && !df->saves)
//        nd_log(NDLS_DAEMON, NDLP_WARNING, "DYNCFG: configuration '%s' is created with source type dyncfg, but we don't have a saved configuration for it", id);

    nrpc_serving_started();
    nrpc_method_register(&(struct nrpc_method_desc) {
        .owner = rrdhost_nrpc_owner(s.host),
        .name = string2str(df->function),
        .help = "Dynamic configuration",
        .tags = "config",
        .timeout_s = 120,
        .priority = 1000,
        .version = DYNCFG_FUNCTIONS_VERSION,
        .access = (s.view_access & s.edit_access),
        .sync = s.sync,
        .source = NRPC_SOURCE_DAEMON,
        .handler = dyncfg_function_intercept_cb,
    });

    if(df->type != DYNCFG_TYPE_TEMPLATE && (df->cmds & (DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE))) {
        DYNCFG_CMDS status_to_send_to_plugin =
            (df->dyncfg.user_disabled || df->current.status == DYNCFG_STATUS_DISABLED) ? DYNCFG_CMD_DISABLE : DYNCFG_CMD_ENABLE;

        if (status_to_send_to_plugin == DYNCFG_CMD_ENABLE && dyncfg_is_user_disabled(string2str(df->template)))
            status_to_send_to_plugin = DYNCFG_CMD_DISABLE;

        dyncfg_echo(item, df, s.id, status_to_send_to_plugin);
    }

    if(!(df->current.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && df->type == DYNCFG_TYPE_JOB))
        dyncfg_send_updates(s.id);

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
        nrpc_method_unregister(rrdhost_nrpc_owner(host), string2str(df->function), NRPC_SOURCE_DAEMON);

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
    if(host == localhost)
        return true;

    return nrpc_method_available(rrdhost_nrpc_owner(host), PLUGINSD_FUNCTION_CONFIG);
}

// ----------------------------------------------------------------------------
