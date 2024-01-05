// SPDX-License-Identifier: GPL-3.0-or-later

#define RRD_COLLECTOR_INTERNALS
#define RRD_FUNCTIONS_INTERNALS

#include "dyncfg.h"

#define DYNCFG_VERSION (size_t)1

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_TYPE type;
    const char *name;
} dyncfg_types[] = {
    { .type = DYNCFG_TYPE_SINGLE, .name = "single" },
    { .type = DYNCFG_TYPE_TEMPLATE, .name = "template" },
    { .type = DYNCFG_TYPE_JOB, .name = "job" },
};

DYNCFG_TYPE dyncfg_type2id(const char *type) {
    if(!type || !*type)
        return DYNCFG_TYPE_SINGLE;

    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_types[i].name, type) == 0)
            return dyncfg_types[i].type;
    }

    return DYNCFG_TYPE_SINGLE;
}

const char *dyncfg_id2type(DYNCFG_TYPE type) {
    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(type == dyncfg_types[i].type)
            return dyncfg_types[i].name;
    }

    return "single";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_SOURCE_TYPE source_type;
    const char *name;
} dyncfg_source_types[] = {
    { .source_type = DYNCFG_SOURCE_TYPE_STOCK, .name = "stock" },
    { .source_type = DYNCFG_SOURCE_TYPE_USER, .name = "user" },
    { .source_type = DYNCFG_SOURCE_TYPE_DYNCFG, .name = "dyncfg" },
    { .source_type = DYNCFG_SOURCE_TYPE_DISCOVERY, .name = "discovered" },
};

DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type) {
    if(!source_type || !*source_type)
        return DYNCFG_SOURCE_TYPE_STOCK;

    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_source_types[i].name, source_type) == 0)
            return dyncfg_source_types[i].source_type;
    }

    return DYNCFG_SOURCE_TYPE_STOCK;
}

const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type) {
    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(source_type == dyncfg_source_types[i].source_type)
            return dyncfg_source_types[i].name;
    }

    return "stock";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_STATUS status;
    const char *name;
} dyncfg_statuses[] = {
    { .status = DYNCFG_STATUS_OK, .name = "ok" },
    { .status = DYNCFG_STATUS_DISABLED, .name = "disabled" },
    { .status = DYNCFG_STATUS_ORPHAN, .name = "orphan" },
    { .status = DYNCFG_STATUS_REJECTED, .name = "rejected" },
};

DYNCFG_STATUS dyncfg_status2id(const char *status) {
    if(!status || !*status)
        return DYNCFG_STATUS_OK;

    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_statuses[i].name, status) == 0)
            return dyncfg_statuses[i].status;
    }

    return DYNCFG_STATUS_OK;
}

const char *dyncfg_id2status(DYNCFG_STATUS status) {
    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(status == dyncfg_statuses[i].status)
            return dyncfg_statuses[i].name;
    }

    return "ok";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_CMDS cmd;
    const char *name;
} cmd_map[] = {
    { .cmd = DYNCFG_CMD_GET, .name = "get" },
    { .cmd = DYNCFG_CMD_SCHEMA, .name = "schema" },
    { .cmd = DYNCFG_CMD_UPDATE, .name = "update" },
    { .cmd = DYNCFG_CMD_ADD, .name = "add" },
    { .cmd = DYNCFG_CMD_TEST, .name = "test" },
    { .cmd = DYNCFG_CMD_REMOVE, .name = "remove" },
    { .cmd = DYNCFG_CMD_ENABLE, .name = "enable" },
    { .cmd = DYNCFG_CMD_DISABLE, .name = "disable" },
    { .cmd = DYNCFG_CMD_RESTART, .name = "restart" }
};

DYNCFG_CMDS dyncfg_cmds2id(const char *cmds) {
    DYNCFG_CMDS result = DYNCFG_CMD_NONE;
    const char *p = cmds;
    size_t len, i;

    while (*p) {
        // Skip any leading spaces
        while (*p == ' ') p++;

        // Find the end of the current word
        const char *end = p;
        while (*end && *end != ' ') end++;
        len = end - p;

        // Compare with known commands
        for (i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
            if (strncmp(p, cmd_map[i].name, len) == 0 && cmd_map[i].name[len] == '\0') {
                result |= cmd_map[i].cmd;
                break;
            }
        }

        // Move to the next word
        p = end;
    }

    return result;
}

static void dyncfg_cmds2fp(DYNCFG_CMDS cmds, FILE *fp) {
    fprintf(fp, "cmds=");
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            fprintf(fp, "%s ", cmd_map[i].name);
    }
    fprintf(fp, "\n");
}

static void dyncfg_cmds2json_array(DYNCFG_CMDS cmds, const char *key, BUFFER *wb) {
    buffer_json_member_add_array(wb, key);
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            buffer_json_add_array_item_string(wb, cmd_map[i].name);
    }
    buffer_json_array_close(wb);
}

// ----------------------------------------------------------------------------

static bool dyncfg_is_valid_id(const char *id) {
    const char *s = id;

    while(*s) {
        if(isspace(*s)) return false;
        s++;
    }

    return true;
}

static char *dyncfg_escape_id(const char *id) {
    if (id == NULL) return NULL;

    // Allocate memory for the worst case, where every character is escaped.
    char *escaped = mallocz(strlen(id) * 3 + 1); // Each char can become '%XX', plus '\0'
    if (!escaped) return NULL;

    const char *src = id;
    char *dest = escaped;

    while (*src) {
        if (*src == '/' || isspace(*src) || !isprint(*src)) {
            sprintf(dest, "%%%02X", (unsigned char)*src);
            dest += 3;
        } else {
            *dest++ = *src;
        }
        src++;
    }

    *dest = '\0';
    return escaped;
}

// ----------------------------------------------------------------------------

typedef struct dyncfg {
    RRDHOST *host;
    STRING *path;
    DYNCFG_STATUS status;
    DYNCFG_TYPE type;
    DYNCFG_CMDS cmds;
    DYNCFG_SOURCE_TYPE source_type;
    STRING *source;
    usec_t created_ut;
    usec_t modified_ut;
    uint32_t saves;
    bool sync;
    bool user_disabled;
    bool restart_required;

    BUFFER *payload;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;
} DYNCFG;

struct {
    const char *dir;
    DICTIONARY *nodes;
} dyncfg_globals = { 0 };

static void dyncfg_cleanup(DYNCFG *v) {
    buffer_free(v->payload);
    v->payload = NULL;

    string_freez(v->path);
    v->path = NULL;

    string_freez(v->source);
    v->source = NULL;
}

static void dyncfg_normalize(DYNCFG *v) {
    usec_t now_ut = now_realtime_usec();

    if(!v->created_ut)
        v->created_ut = now_ut;

    if(!v->modified_ut)
        v->modified_ut = now_ut;
}

static void dyncfg_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_cleanup(v);
}

static void dyncfg_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    DYNCFG *v = value;
    dyncfg_normalize(v);
}

static bool dyncfg_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    DYNCFG *v = old_value;
    DYNCFG *nv = new_value;

    size_t changes = 0;

    dyncfg_normalize(nv);

    if(v->host != nv->host) {
        SWAP(v->host, nv->host);
        changes++;
    }

    if(v->path != nv->path) {
        SWAP(v->path, nv->path);
        changes++;
    }

    if(v->status != nv->status) {
        SWAP(v->status, nv->status);
        changes++;
    }

    if(v->type != nv->type) {
        SWAP(v->type, nv->type);
        changes++;
    }

    if(v->source_type != nv->source_type) {
        SWAP(v->source_type, nv->source_type);
        changes++;
    }

    if(v->cmds != nv->cmds) {
        SWAP(v->cmds, nv->cmds);
        changes++;
    }

    if(v->source != nv->source) {
        SWAP(v->source, nv->source);
        changes++;
    }

    if(nv->created_ut < v->created_ut) {
        SWAP(v->created_ut, nv->created_ut);
        changes++;
    }

    if(nv->modified_ut > v->modified_ut) {
        SWAP(v->modified_ut, nv->modified_ut);
        changes++;
    }

    if(v->sync != nv->sync) {
        SWAP(v->sync, nv->sync);
        changes++;
    }

    if(nv->payload) {
        SWAP(v->payload, nv->payload);
        changes++;
    }

    if(nv->execute_cb && (v->execute_cb != nv->execute_cb || v->execute_cb_data != nv->execute_cb_data)) {
        v->execute_cb = nv->execute_cb;
        v->execute_cb_data = nv->execute_cb_data;
        changes++;
    }

    dyncfg_cleanup(nv);

    return changes > 0;
}

void dyncfg_save(const char *id, DYNCFG *df) {
    CLEAN_CHAR_P *escaped_id = dyncfg_escape_id(id);
    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/%s.dyncfg", dyncfg_globals.dir, escaped_id);

    FILE *fp = fopen(filename, "w");
    if(!fp) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot create file '%s'", filename);
        return;
    }

    fprintf(fp, "version=%zu\n", DYNCFG_VERSION);
    fprintf(fp, "id=%s\n", id);

    if(df->host)
        fprintf(fp, "host=%s\n", rrdhost_hostname(df->host));

    fprintf(fp, "path=%s\n", string2str(df->path));
    fprintf(fp, "type=%s\n", dyncfg_id2type(df->type));
    fprintf(fp, "source_type=%s\n", dyncfg_id2source_type(df->source_type));
    fprintf(fp, "source=%s\n", string2str(df->source));
    fprintf(fp, "created=%"PRIu64"\n", df->created_ut);
    fprintf(fp, "modified=%"PRIu64"\n", df->modified_ut);
    fprintf(fp, "sync=%s\n", df->sync ? "true" : "false");
    fprintf(fp, "user_disabled=%s\n", df->user_disabled ? "true" : "false");
    fprintf(fp, "saves=%"PRIu32"\n", ++df->saves);
    dyncfg_cmds2fp(df->cmds, fp);

    if(df->payload && buffer_strlen(df->payload) > 0) {
        fprintf(fp, "content_type=%s\n", content_type_id2string(df->payload->content_type));
        fprintf(fp, "content_length=%zu\n", buffer_strlen(df->payload));
        fprintf(fp, "---\n");
        fwrite(buffer_tostring(df->payload), 1, buffer_strlen(df->payload), fp);
    }

    fclose(fp);
}

static void dyncfg_load(const char *filename) {
    FILE *fp = fopen(filename, "r");
    if (!fp) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot open file '%s'", filename);
        return;
    }

    DYNCFG tmp = {
        .host = NULL,
        .status = DYNCFG_STATUS_ORPHAN,
    };

    char line[PLUGINSD_LINE_MAX];
    CLEAN_CHAR_P *id = NULL, *hostname = NULL;

    HTTP_CONTENT_TYPE content_type = CT_NONE;
    size_t content_length = 0;
    bool read_payload = false;

    while (fgets(line, sizeof(line), fp)) {
        if(strcmp(line, "---\n") == 0) {
            read_payload = true;
            break;
        }

        char *value = strchr(line, '=');
        if(!value) continue;

        *value++ = '\0';

        value = trim(value);
        if(!value) continue;

        char *key = trim(line);
        if(!key) continue;

        // Parse key-value pairs
        if (strcmp(key, "version") == 0) {
            size_t version = strtoull(value, NULL, 10);

            if(version > DYNCFG_VERSION)
                nd_log(NDLS_DAEMON, NDLP_NOTICE,
                       "DYNCFG: configuration file '%s' has version %zu, which is newer than our version %zu",
                       filename, version, DYNCFG_VERSION);

        } else if (strcmp(key, "id") == 0) {
            id = strdupz(value);
        } else if (strcmp(key, "host") == 0) {
            hostname = strdupz(value);
        } else if (strcmp(key, "path") == 0) {
            tmp.path = string_strdupz(value);
        } else if (strcmp(key, "type") == 0) {
            tmp.type = dyncfg_type2id(value);
        } else if (strcmp(key, "source_type") == 0) {
            tmp.source_type = dyncfg_source_type2id(value);
        } else if (strcmp(key, "source") == 0) {
            tmp.source = string_strdupz(value);
        } else if (strcmp(key, "created") == 0) {
            tmp.created_ut = strtoull(value, NULL, 10);
        } else if (strcmp(key, "modified") == 0) {
            tmp.modified_ut = strtoull(value, NULL, 10);
        } else if (strcmp(key, "sync") == 0) {
            tmp.sync = (strcmp(value, "true") == 0);
        } else if (strcmp(key, "user_disabled") == 0) {
            tmp.user_disabled = (strcmp(value, "true") == 0);
        } else if (strcmp(key, "saves") == 0) {
            tmp.saves = strtoull(value, NULL, 10);
        } else if (strcmp(key, "content_type") == 0) {
            content_type = content_type_string2id(value);
        } else if (strcmp(key, "content_length") == 0) {
            content_length = strtoull(value, NULL, 10);
        } else if (strcmp(key, "cmds") == 0) {
            tmp.cmds = dyncfg_cmds2id(value);
        }
    }
    fclose(fp);

    if(read_payload && content_length) {
        tmp.payload = buffer_create(content_length, NULL);
        tmp.payload->content_type = content_type;

        buffer_need_bytes(tmp.payload, content_length);
        tmp.payload->len = fread(&tmp.payload->buffer, 1, content_length, fp);
    }

    if(!id) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: configuration file '%s' does not include a unique id. Ignoring it.",
               filename);

        dyncfg_cleanup(&tmp);
        return;
    }

    // FIXME: RRDHOST may not be available when files are loaded

    dictionary_set(dyncfg_globals.nodes, id, &tmp, sizeof(tmp));
}

void dyncfg_scan() {
    DIR *dir = opendir(dyncfg_globals.dir);
    if (!dir) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot open directory '%s'", dyncfg_globals.dir);
        return;
    }

    struct dirent *entry;
    char filepath[PATH_MAX];
    while ((entry = readdir(dir)) != NULL) {
        if ((entry->d_type == DT_REG || entry->d_type == DT_LNK) && strendswith(entry->d_name, ".dyncfg")) {
            snprintf(filepath, sizeof(filepath), "%s/%s", dyncfg_globals.dir, entry->d_name);
            dyncfg_load(filepath);
        }
    }

    closedir(dir);
}

void dyncfg_init(bool load_saved) {
    if(!dyncfg_globals.nodes) {
        dyncfg_globals.nodes = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(DYNCFG));
        dictionary_register_insert_callback(dyncfg_globals.nodes, dyncfg_insert_cb, NULL);
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
            dyncfg_scan();
    }
}

static const DICTIONARY_ITEM *dyncfg_add_internal(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {
    DYNCFG tmp = {
        .host = host,
        .path = string_strdupz(path),
        .status = status,
        .type = type,
        .cmds = cmds,
        .source_type = source_type,
        .source = string_strdupz(source),
        .created_ut = created_ut,
        .modified_ut = modified_ut,
        .sync = sync,
        .user_disabled = false,
        .restart_required = false,
        .payload = NULL,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
    };

    return dictionary_set_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1, &tmp, sizeof(tmp), NULL);
}

// ----------------------------------------------------------------------------
// echo is the first config command we send to the plugin

struct dyncfg_echo {
    const DICTIONARY_ITEM *item;
    DYNCFG *df;
    BUFFER *wb;
};

void dyncfg_echo_cb(BUFFER *wb __maybe_unused, int code, void *result_cb_data) {
    struct dyncfg_echo *e = result_cb_data;

    if(code != HTTP_RESP_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Failed to send the first config cmd to '%s', with error code %d",
               e->item ? dictionary_acquired_item_name(e->item) : "(null)", code);

        e->df->status = DYNCFG_STATUS_REJECTED;
    }
    else
        e->df->status = DYNCFG_STATUS_OK;

    buffer_free(e->wb);
    dictionary_acquired_item_release(dyncfg_globals.nodes, e->item);

    e->wb = NULL;
    e->df = NULL;
    e->item = NULL;
    freez(e);
}

static void dyncfg_send_echo_status(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[strlen(id) + 100];
    snprintfz(buf, sizeof(buf), PLUGINSD_FUNCTION_CONFIG " %s %s", id, df->user_disabled ? "disable" : "enable");

    rrd_function_run(df->host, e->wb, 10,
                     HTTP_ACCESS_ADMINS, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     NULL);
}

static void dyncfg_send_echo_payload(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, const char *cmd) {
    if(!df->payload)
        return;

    struct dyncfg_echo *e = callocz(1, sizeof(struct dyncfg_echo));
    e->item = dictionary_acquired_item_dup(dyncfg_globals.nodes, item);
    e->wb = buffer_create(0, NULL);
    e->df = df;

    char buf[strlen(id) + 100];
    snprintfz(buf, sizeof(buf), PLUGINSD_FUNCTION_CONFIG " %s %s", id, cmd);

    rrd_function_run(df->host, e->wb, 10,
                     HTTP_ACCESS_ADMINS, buf, false, NULL,
                     dyncfg_echo_cb, e,
                     NULL, NULL,
                     NULL, NULL,
                     NULL);
}

static void dyncfg_send_echo_update(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id) {
    dyncfg_send_echo_payload(item, df, id, "update");
}

static void dyncfg_send_echo_add(const DICTIONARY_ITEM *item, DYNCFG *df, const char *id, const char *job_name) {
    char buf[strlen(job_name) + 20];
    snprintfz(buf, sizeof(buf), "add %s", job_name);
    dyncfg_send_echo_payload(item, df, id, buf);
}

// ----------------------------------------------------------------------------
// we intercept the config function calls of the plugin

struct dyncfg_call {
    BUFFER *payload;
    char *function;
    char *id;
    char *add_name;
    DYNCFG_CMDS cmd;
    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
};

void dyncfg_function_result_cb(BUFFER *wb, int code, void *result_cb_data) {
    struct dyncfg_call *dc = result_cb_data;

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, dc->id, -1);
    if(item) {
        DYNCFG *df = dictionary_acquired_item_value(item);

        if(code == HTTP_RESP_NOT_FOUND) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not found error to call '%s', marking it as rejected.", dc->function);
            df->status = DYNCFG_STATUS_REJECTED;
        }
        else if(code == HTTP_RESP_NOT_IMPLEMENTED) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: plugin returned not supported error to call '%s', disabling this action.", dc->function);
            df->cmds &= ~dc->cmd;
        }
        else if(code == HTTP_RESP_ACCEPTED) {
            nd_log(NDLS_DAEMON, NDLP_INFO, "DYNCFG: plugin returned 202 to call '%s', restart is required.", dc->function);
            df->cmds &= ~dc->cmd;
        }

        if(code == HTTP_RESP_OK || code == HTTP_RESP_ACCEPTED) {
            if(dc->cmd == DYNCFG_CMD_ADD) {
                char id[strlen(dc->id) + 1 + strlen(dc->add_name) + 1];
                snprintfz(id, sizeof(id), "%s:%s", dc->id, dc->add_name);

                const DICTIONARY_ITEM *new_item =
                    dyncfg_add_internal(df->host, id, string2str(df->path),
                                        DYNCFG_STATUS_OK, DYNCFG_TYPE_JOB, DYNCFG_SOURCE_TYPE_DYNCFG,
                                        "dyncfg", df->cmds & ~DYNCFG_CMD_ADD, 0, 0,
                                        df->sync, NULL, NULL);

                DYNCFG *new_df = dictionary_acquired_item_value(new_item);
                SWAP(new_df->payload, dc->payload);

                dyncfg_save(id, new_df);
                dictionary_acquired_item_release(dyncfg_globals.nodes, new_item);
            }
            else if(dc->cmd == DYNCFG_CMD_UPDATE) {
                SWAP(df->payload, dc->payload);
                dyncfg_save(dc->id, df);
            }
            else if(dc->cmd == DYNCFG_CMD_ENABLE) {
                bool old = df->user_disabled;
                df->user_disabled = false;

                if(old != df->user_disabled)
                    dyncfg_save(dc->id, df);
            }
            else if(dc->cmd == DYNCFG_CMD_DISABLE) {
                bool old = df->user_disabled;
                df->user_disabled = true;

                if(old != df->user_disabled)
                    dyncfg_save(dc->id, df);
            }
        }

        dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    }

    if(dc->result_cb)
        dc->result_cb(wb, code, dc->result_cb_data);

    buffer_free(dc->payload);
    freez(dc->function);
    freez(dc->id);
    freez(dc);
}

static int dyncfg_function_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
                                 usec_t *stop_monotonic_ut, const char *function,
                                 void *execute_cb_data __maybe_unused,
                                 rrd_function_result_callback_t result_cb, void *result_cb_data,
                                 rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                 rrd_function_is_cancelled_cb_t is_cancelled_cb,
                                 void *is_cancelled_cb_data,
                                 rrd_function_register_canceller_cb_t register_canceller_cb,
                                 void *register_canceller_cb_data,
                                 rrd_function_register_progresser_cb_t register_progresser_cb,
                                 void *register_progresser_cb_data) {

    // IMPORTANT: this function MUST call the result_cb even on failures

    DYNCFG_CMDS c = DYNCFG_CMD_NONE;
    const DICTIONARY_ITEM *item = NULL;
    const char *add_name = NULL;
    size_t add_name_len = 0;
    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) == 0) {
        const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
        while(isspace(*id)) id++;
        const char *space = id;
        while(*space && !isspace(*space)) space++;
        size_t id_len = space - id;

        const char *cmd = space;
        while(isspace(*cmd)) cmd++;
        space = cmd;
        while(*space && !isspace(*space)) space++;
        size_t cmd_len = space - cmd;

        char cmd_copy[cmd_len + 1];
        strncpyz(cmd_copy, cmd, cmd_len);
        c = dyncfg_cmds2id(cmd_copy);

        if(c == DYNCFG_CMD_ADD) {
            add_name = space;
            while(isspace(*add_name)) add_name++;
            space = add_name;
            while(*space && !isspace(*space)) space++;
            add_name_len = space - add_name;
        }

        item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, (ssize_t)id_len);
    }

    if(!item) {
        rrd_call_function_error(result_body_wb, "not found", HTTP_RESP_NOT_FOUND);

        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_NOT_FOUND, result_cb_data);

        return HTTP_RESP_NOT_FOUND;
    }

    if(c == DYNCFG_CMD_ADD && (!add_name || !*add_name || !add_name_len)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: add command does not specify a name: %s", function);
        dictionary_acquired_item_release(dyncfg_globals.nodes, item);

        rrd_call_function_error(result_body_wb, "bad request, name is missing", HTTP_RESP_BAD_REQUEST);

        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_BAD_REQUEST, result_cb_data);

        return HTTP_RESP_BAD_REQUEST;
    }

    struct dyncfg_call *dc = callocz(1, sizeof(*dc));
    dc->function = strdupz(function);
    dc->id = strdupz(dictionary_acquired_item_name(item));
    dc->add_name = (c == DYNCFG_CMD_ADD) ? strndupz(add_name, add_name_len) : NULL;
    dc->cmd = c;
    dc->result_cb = result_cb;
    dc->result_cb_data = result_cb_data;
    dc->payload = buffer_dup(payload);

    DYNCFG *df = dictionary_acquired_item_value(item);
    int rc = df->execute_cb(transaction, result_body_wb, payload,
                            stop_monotonic_ut, function, df->execute_cb_data,
                            dyncfg_function_result_cb, dc,
                            progress_cb, progress_cb_data,
                            is_cancelled_cb, is_cancelled_cb_data,
                            register_canceller_cb, register_canceller_cb_data,
                            register_progresser_cb, register_progresser_cb_data);

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
    return rc;
}

// ----------------------------------------------------------------------------

static void dyncfg_update_plugin(const char *id) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dyncfg_globals.nodes, id, -1);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: asked to update plugin for configuration '%s', but it is not found.", id);
        return;
    }

    DYNCFG *df = dictionary_acquired_item_value(item);

    if(df->type == DYNCFG_TYPE_SINGLE || df->type == DYNCFG_TYPE_JOB) {
        if (df->cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
            dyncfg_send_echo_update(item, df, id);
    }
    else if(df->type == DYNCFG_TYPE_TEMPLATE) {
        size_t len = strlen(id);
        DYNCFG *tf;
        dfe_start_reentrant(dyncfg_globals.nodes, tf) {
            const char *t_id = tf_dfe.name;
            if(tf->type == DYNCFG_TYPE_JOB && strncmp(t_id, id, len) == 0 && t_id[len] == ':' && t_id[len + 1]) {
                if (tf->cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
                    dyncfg_send_echo_add(tf_dfe.item, tf, id, &t_id[len + 1]);
            }
        }
        dfe_done(tf);
    }

    dictionary_acquired_item_release(dyncfg_globals.nodes, item);
}

bool dyncfg_add(RRDHOST *host, const char *id, const char *path, DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds, usec_t created_ut, usec_t modified_ut, bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return false;
    }

    // all configurations support schema
    cmds |= DYNCFG_CMD_SCHEMA;

    // if there is either enable or disable, both are supported
    if(cmds & (DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE))
        cmds |= DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE;

    if(type == DYNCFG_TYPE_TEMPLATE) {
        // templates must always support "add"
        cmds |= DYNCFG_CMD_ADD;

        // templates do not have data
        cmds &= ~(DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST);
    }
    else if(type == DYNCFG_TYPE_JOB) {
        // jobs cannot support "add"
        cmds &= ~DYNCFG_CMD_ADD;
    }
    else if(type == DYNCFG_TYPE_SINGLE) {
        // single cannot support "add" or "remove"
        cmds &= ~(DYNCFG_CMD_ADD | DYNCFG_CMD_REMOVE);
    }

    const DICTIONARY_ITEM *item = dyncfg_add_internal(host, id, path, status, type, source_type, source, cmds, created_ut, modified_ut, sync, execute_cb, execute_cb_data);
    DYNCFG *df = dictionary_acquired_item_value(item);

    char name[strlen(id) + 20];
    snprintfz(name, sizeof(name), PLUGINSD_FUNCTION_CONFIG " %s", id);

    rrd_collector_started();
    rrd_function_add(host, NULL, name, 120, 1000,
                     "Dynamic configuration", "config", HTTP_ACCESS_MEMBERS,
                     sync, dyncfg_function_execute_cb, NULL);

    dyncfg_send_echo_status(item, df, id);
    dyncfg_update_plugin(id);
    dictionary_acquired_item_release(dyncfg_globals.nodes, item);

    return true;
}

void dyncfg_add_streaming(BUFFER *wb) {
    // when sending config functions to parents, we send only 1 function called 'config';
    // the parent will send the command to the child and the child will validate it;
    // this way the parent does not need to receive removals of config functions;

    buffer_sprintf(wb
                   , PLUGINSD_KEYWORD_FUNCTION " GLOBAL " PLUGINSD_FUNCTION_CONFIG " %d \"%s\" \"%s\" \"%s\" %d\n"
                   , 120
                   , "Dynamic configuration"
                   , "config"
                   , http_id2access(HTTP_ACCESS_MEMBERS)
                   , 1000
    );
}

bool dyncfg_available_for_rrdhost(RRDHOST *host) {
    if(host == localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
        return true;

    if(!host->functions)
        return false;

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions, PLUGINSD_FUNCTION_CONFIG);
    if(item) {
        struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);
        if(rrd_collector_running(rdcf->collector))
            ret = true;

        dictionary_acquired_item_release(host->functions, item);
    }

    return ret;
}

// ----------------------------------------------------------------------------

static int dyncfg_tree_compar(const void *a, const void *b) {
    const DICTIONARY_ITEM *item1 = *(const DICTIONARY_ITEM **)a;
    const DICTIONARY_ITEM *item2 = *(const DICTIONARY_ITEM **)b;

    DYNCFG *df1 = dictionary_acquired_item_value(item1);
    DYNCFG *df2 = dictionary_acquired_item_value(item2);

    int rc = string_cmp(df1->path, df2->path);
    if(rc == 0)
        rc = strcmp(dictionary_acquired_item_name(item1), dictionary_acquired_item_name(item2));

    return rc;
}

static void dyncfg_to_json(DYNCFG *df, const char *id, BUFFER *wb) {
    buffer_json_member_add_object(wb, id);
    {
        buffer_json_member_add_string(wb, "type", dyncfg_id2type(df->type));
        buffer_json_member_add_string(wb, "status", dyncfg_id2status(df->status));
        dyncfg_cmds2json_array(df->cmds, "cmds", wb);
        buffer_json_member_add_string(wb, "source_type", dyncfg_id2source_type(df->source_type));
        buffer_json_member_add_string(wb, "source", string2str(df->source));
        buffer_json_member_add_boolean(wb, "sync", df->sync);
        buffer_json_member_add_boolean(wb, "user_disabled", df->user_disabled);
        buffer_json_member_add_boolean(wb, "restart_required", df->restart_required);
        if(df->payload && buffer_strlen(df->payload)) {
            buffer_json_member_add_object(wb, "payload");
            buffer_json_member_add_boolean(wb, "content_type", content_type_id2string(df->payload->content_type));
            buffer_json_member_add_uint64(wb, "content_length", df->payload->len);
            buffer_json_object_close(wb);
        }
        buffer_json_member_add_uint64(wb, "saves", df->saves);
        buffer_json_member_add_uint64(wb, "created_ut", df->created_ut);
        buffer_json_member_add_uint64(wb, "modified_ut", df->modified_ut);
    }
    buffer_json_object_close(wb);
}

static void dyncfg_tree_for_host(RRDHOST *host, BUFFER *wb, const char *parent) {
    size_t entries = dictionary_entries(dyncfg_globals.nodes);
    size_t used = 0;
    const DICTIONARY_ITEM *items[entries];

    size_t parent_len = strlen(parent);
    DYNCFG *df;
    dfe_start_read(dyncfg_globals.nodes, df) {
        if(df->host != host || strncmp(string2str(df->path), parent, parent_len) != 0)
            continue;

        items[used++] = dictionary_acquired_item_dup(dyncfg_globals.nodes, df_dfe.item);
    }
    dfe_done(df);

    qsort(items, used, sizeof(const DICTIONARY_ITEM *), dyncfg_tree_compar);

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    STRING *last_path = NULL;
    for(size_t i = 0; i < used ;i++) {
        df = dictionary_acquired_item_value(items[i]);
        if(df->path != last_path) {
            last_path = df->path;

            if(i)
                buffer_json_object_close(wb);

            buffer_json_member_add_object(wb, string2str(last_path));
        }

        dyncfg_to_json(df, dictionary_acquired_item_name(items[i]), wb);
    }

    if(used)
        buffer_json_object_close(wb);

    buffer_json_finalize(wb);

    for(size_t i = 0; i < used ;i++)
        dictionary_acquired_item_release(dyncfg_globals.nodes, items[i]);
}

static int dyncfg_config_execute_cb(uuid_t *transaction __maybe_unused, BUFFER *result_body_wb, BUFFER *payload __maybe_unused,
                                    usec_t *stop_monotonic_ut __maybe_unused, const char *function,
                                    void *execute_cb_data,
                                    rrd_function_result_callback_t result_cb, void *result_cb_data,
                                    rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                                    rrd_function_is_cancelled_cb_t is_cancelled_cb __maybe_unused,
                                    void *is_cancelled_cb_data __maybe_unused,
                                    rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                                    void *register_canceller_cb_data __maybe_unused,
                                    rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                                    void *register_progresser_cb_data __maybe_unused) {
    RRDHOST *host = execute_cb_data;

    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: received function that is not config: %s", function);
        rrd_call_function_error(result_body_wb, "wrong function", 400);
        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_NOT_FOUND, result_cb_data);
        return HTTP_RESP_NOT_FOUND;
    }

    // extract the id
    const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
    while(*id && isspace(*id)) id++;
    const char *space = id;
    while(*space && !isspace(*space)) space++;
    size_t id_len = space - id;

    char id_copy[id_len + 1];
    memcpy(id_copy, id, id_len);
    id_copy[id_len] = '\0';

    // extract the path
    const char *path = space;
    while(*path && isspace(*path)) path++;
    space = path;
    while(*space && !isspace(*space)) space++;
    size_t path_len = space - path;

    if(*path || !path_len) {
        path_len = 1;
        path = "/";
    }

    char path_copy[path_len + 1];
    memcpy(path_copy, path, path_len);
    path_copy[path_len] = '\0';

    if(strcmp(id_copy, "tree") == 0) {
        dyncfg_tree_for_host(host, result_body_wb, path_copy);
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: unsupported config command '%s' in: %s", id_copy, function);
        rrd_call_function_error(result_body_wb, "wrong config function", 400);
        if(result_cb)
            result_cb(result_body_wb, HTTP_RESP_NOT_FOUND, result_cb_data);
        return HTTP_RESP_NOT_FOUND;
    }

    if(result_cb)
        result_cb(result_body_wb, HTTP_RESP_OK, result_cb_data);

    return HTTP_RESP_OK;
}

void dyncfg_host_init(RRDHOST *host) {
    rrd_function_add(host, NULL, PLUGINSD_FUNCTION_CONFIG, 120,
                     1000, "Dynamic configuration", "config", HTTP_ACCESS_MEMBERS,
                     true, dyncfg_config_execute_cb, host);
}

// ----------------------------------------------------------------------------
// unit test

#define LINE_FILE_STR TOSTRING(__LINE__) "@" __FILE__

struct dyncfg_unittest {
    bool enabled;
    int errors;
} dyncfg_unittest_data = { 0 };

static int dyncfg_unittest_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
                                      usec_t *stop_monotonic_ut, const char *function,
                                      void *execute_cb_data,
                                      rrd_function_result_callback_t result_cb, void *result_cb_data,
                                      rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                      rrd_function_is_cancelled_cb_t is_cancelled_cb,
                                      void *is_cancelled_cb_data,
                                      rrd_function_register_canceller_cb_t register_canceller_cb,
                                      void *register_canceller_cb_data,
                                      rrd_function_register_progresser_cb_t register_progresser_cb,
                                      void *register_progresser_cb_data) {
    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: received function that is not config: %s", function);
        rrd_call_function_error(result_body_wb, "wrong function", 400);
        if(result_cb)
            result_cb(result_body_wb, 400, result_cb_data);
        return 400;
    }

    // extract the id
    const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
    while(*id && isspace(*id)) id++;
    const char *space = id;
    while(*space && !isspace(*space)) space++;
    size_t id_len = space - id;

    char id_copy[id_len + 1];
    memcpy(id_copy, id, id_len);
    id_copy[id_len] = '\0';

    // extract the cmd
    const char *cmd = space;
    while(*cmd && isspace(*cmd)) cmd++;
    space = cmd;
    while(*space && !isspace(*space)) space++;
    size_t cmd_len = space - cmd;

    char cmd_copy[cmd_len + 1];
    memcpy(cmd_copy, cmd, cmd_len);
    cmd_copy[cmd_len] = '\0';
    DYNCFG_CMDS c = dyncfg_cmds2id(cmd_copy);

    if(c == DYNCFG_CMD_ENABLE)
        dyncfg_unittest_data.enabled = true;
    else if(c == DYNCFG_CMD_DISABLE)
        dyncfg_unittest_data.enabled = false;

    if(result_cb)
        result_cb(result_body_wb, HTTP_RESP_OK, result_cb_data);

    return HTTP_RESP_OK;
}

static int dyncfg_unittest_run(const char *cmd, BUFFER *wb) {
    buffer_flush(wb);

    int rc = rrd_function_run(localhost, wb, 10, HTTP_ACCESS_ADMINS, cmd,
                              true, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              NULL);
    if(rc != HTTP_RESP_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to run: %s", cmd);
        dyncfg_unittest_data.errors++;
    }

    return rc;
}

int dyncfg_unittest(void) {
    rrd_functions_inflight_init();
    dyncfg_init(false);

    dyncfg_unittest_data.enabled = false;
    dyncfg_add(localhost, "unittest:sync:single", "/unittests",
               DYNCFG_STATUS_OK, DYNCFG_TYPE_SINGLE,
               DYNCFG_SOURCE_TYPE_DYNCFG, LINE_FILE_STR,
               DYNCFG_CMD_GET|DYNCFG_CMD_SCHEMA|DYNCFG_CMD_UPDATE|DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE,
               0, 0, true,
               dyncfg_unittest_execute_cb, &dyncfg_unittest_data);

    dyncfg_add(localhost, "unittest:sync:jobs", "/unittests",
               DYNCFG_STATUS_OK, DYNCFG_TYPE_TEMPLATE,
               DYNCFG_SOURCE_TYPE_DYNCFG, LINE_FILE_STR,
               DYNCFG_CMD_SCHEMA|DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE|DYNCFG_CMD_ADD|DYNCFG_CMD_RESTART,
               0, 0, true,
               dyncfg_unittest_execute_cb, &dyncfg_unittest_data);

    dyncfg_add(localhost, "unittest:sync:jobs:stock", "/unittests",
               DYNCFG_STATUS_OK, DYNCFG_TYPE_JOB,
               DYNCFG_SOURCE_TYPE_STOCK, LINE_FILE_STR,
               DYNCFG_CMD_GET|DYNCFG_CMD_SCHEMA|DYNCFG_CMD_UPDATE|DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE|DYNCFG_CMD_RESTART|DYNCFG_CMD_TEST,
               0, 0, true,
               dyncfg_unittest_execute_cb, &dyncfg_unittest_data);

    dyncfg_add(localhost, "unittest:sync:jobs:user", "/unittests",
               DYNCFG_STATUS_OK, DYNCFG_TYPE_JOB,
               DYNCFG_SOURCE_TYPE_USER, LINE_FILE_STR,
               DYNCFG_CMD_GET|DYNCFG_CMD_SCHEMA|DYNCFG_CMD_UPDATE|DYNCFG_CMD_ENABLE|DYNCFG_CMD_DISABLE|DYNCFG_CMD_RESTART|DYNCFG_CMD_TEST,
               0, 0, true,
               dyncfg_unittest_execute_cb, &dyncfg_unittest_data);

    int rc;
    BUFFER *wb = buffer_create(0, NULL);

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single enable", wb);
    if(rc == HTTP_RESP_OK && !dyncfg_unittest_data.enabled) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is not set");
        dyncfg_unittest_data.errors++;
    }

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single disable", wb);
    if(rc == HTTP_RESP_OK && dyncfg_unittest_data.enabled) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is set");
        dyncfg_unittest_data.errors++;
    }

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " tree", wb);
    if(rc == HTTP_RESP_OK)
        fprintf(stderr, "%s\n", buffer_tostring(wb));

    return dyncfg_unittest_data.errors;
}
