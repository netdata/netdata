// SPDX-License-Identifier: GPL-3.0-or-later

#define DYNCFG_INTERNALS
#include "dyncfg.h"

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

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(df->host_uuid, uuid_str);
    fprintf(fp, "host=%s\n", uuid_str);

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

void dyncfg_load(const char *filename) {
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
            uuid_parse_flexi(value, tmp.host_uuid);
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

    if(read_payload && content_length) {
        tmp.payload = buffer_create(content_length, NULL);
        tmp.payload->content_type = content_type;

        buffer_need_bytes(tmp.payload, content_length);
        tmp.payload->len = fread(tmp.payload->buffer, 1, content_length, fp);
    }

    fclose(fp);

    if(!id) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: configuration file '%s' does not include a unique id. Ignoring it.",
               filename);

        dyncfg_cleanup(&tmp);
        return;
    }

    dictionary_set(dyncfg_globals.nodes, id, &tmp, sizeof(tmp));
}

void dyncfg_load_all(void) {
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
