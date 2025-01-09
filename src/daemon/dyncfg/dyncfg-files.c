// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

void dyncfg_file_delete(const char *id) {
    CLEAN_CHAR_P *escaped_id = dyncfg_escape_id_for_filename(id);
    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/%s.dyncfg", dyncfg_globals.dir, escaped_id);
    unlink(filename);
}

void dyncfg_file_save(const char *id, DYNCFG *df) {
    CLEAN_CHAR_P *escaped_id = dyncfg_escape_id_for_filename(id);
    char filename[FILENAME_MAX];
    snprintfz(filename, sizeof(filename), "%s/%s.dyncfg", dyncfg_globals.dir, escaped_id);

    FILE *fp = fopen(filename, "w");
    if(!fp) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot create file '%s'", filename);
        return;
    }

    df->dyncfg.modified_ut = now_realtime_usec();
    if(!df->dyncfg.created_ut)
        df->dyncfg.created_ut = df->dyncfg.modified_ut;

    fprintf(fp, "version=%zu\n", DYNCFG_VERSION);
    fprintf(fp, "id=%s\n", id);

    if(df->template)
        fprintf(fp, "template=%s\n", string2str(df->template));

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(df->host_uuid.uuid, uuid_str);
    fprintf(fp, "host=%s\n", uuid_str);

    fprintf(fp, "path=%s\n", string2str(df->path));
    fprintf(fp, "type=%s\n", dyncfg_id2type(df->type));

    fprintf(fp, "source_type=%s\n", dyncfg_id2source_type(df->dyncfg.source_type));
    fprintf(fp, "source=%s\n", string2str(df->dyncfg.source));

    fprintf(fp, "created=%"PRIu64"\n", df->dyncfg.created_ut);
    fprintf(fp, "modified=%"PRIu64"\n", df->dyncfg.modified_ut);
    fprintf(fp, "sync=%s\n", df->sync ? "true" : "false");
    fprintf(fp, "user_disabled=%s\n", df->dyncfg.user_disabled ? "true" : "false");
    fprintf(fp, "saves=%"PRIu32"\n", ++df->dyncfg.saves);

    fprintf(fp, "cmds=");
    dyncfg_cmds2fp(df->cmds, fp);
    fprintf(fp, "\n");

    if(df->dyncfg.payload && buffer_strlen(df->dyncfg.payload) > 0) {
        fprintf(fp, "content_type=%s\n", content_type_id2string(df->dyncfg.payload->content_type));
        fprintf(fp, "content_length=%zu\n", buffer_strlen(df->dyncfg.payload));
        fprintf(fp, "---\n");
        fwrite(buffer_tostring(df->dyncfg.payload), 1, buffer_strlen(df->dyncfg.payload), fp);
    }

    fclose(fp);
}

void dyncfg_file_load(const char *d_name) {
    char filename[PATH_MAX];
    snprintf(filename, sizeof(filename), "%s/%s", dyncfg_globals.dir, d_name);

    FILE *fp = fopen(filename, "r");
    if (!fp) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot open file '%s'", filename);
        return;
    }

    DYNCFG tmp = { 0 };

    char line[PLUGINSD_LINE_MAX];
    CLEAN_CHAR_P *id = NULL;

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
            freez(id);
            id = strdupz(value);
        } else if (strcmp(key, "template") == 0) {
            tmp.template = string_strdupz(value);
        } else if (strcmp(key, "host") == 0) {
            uuid_parse_flexi(value, tmp.host_uuid.uuid);
        } else if (strcmp(key, "path") == 0) {
            tmp.path = string_strdupz(value);
        } else if (strcmp(key, "type") == 0) {
            tmp.type = dyncfg_type2id(value);
        } else if (strcmp(key, "source_type") == 0) {
            tmp.dyncfg.source_type = dyncfg_source_type2id(value);
        } else if (strcmp(key, "source") == 0) {
            tmp.dyncfg.source = string_strdupz(value);
        } else if (strcmp(key, "created") == 0) {
            tmp.dyncfg.created_ut = strtoull(value, NULL, 10);
        } else if (strcmp(key, "modified") == 0) {
            tmp.dyncfg.modified_ut = strtoull(value, NULL, 10);
        } else if (strcmp(key, "sync") == 0) {
            tmp.sync = (strcmp(value, "true") == 0);
        } else if (strcmp(key, "user_disabled") == 0) {
            tmp.dyncfg.user_disabled = (strcmp(value, "true") == 0);
        } else if (strcmp(key, "saves") == 0) {
            tmp.dyncfg.saves = strtoull(value, NULL, 10);
        } else if (strcmp(key, "content_type") == 0) {
            content_type = content_type_string2id(value);
        } else if (strcmp(key, "content_length") == 0) {
            content_length = strtoull(value, NULL, 10);
        } else if (strcmp(key, "cmds") == 0) {
            tmp.cmds = dyncfg_cmds2id(value);
        }
    }

    if (read_payload) {
        // Determine the actual size of the remaining file content
        int rc = 0;
        long saved_position = ftell(fp); // Save current position

        long total_size = 0;
        size_t actual_size = 0;
        if (saved_position != -1) {
            rc = fseek(fp, 0, SEEK_END);
            if (!rc) {
                total_size = ftell(fp);                      // Total size of the file
                actual_size = total_size - saved_position;   // Calculate remaining content size
                rc = fseek(fp, saved_position, SEEK_SET);    // Reset file pointer to the beginning of the payload
            }
        }

        // Check if any of the system calls failed
        if (rc || saved_position == -1 || total_size == -1) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: error while accessing '%s' to calculate the %s.", filename,
                   saved_position == -1 ? "payload position" : "file size");
            fclose(fp);
            dyncfg_cleanup(&tmp);
            return;
        }
        // Use actual_size instead of content_length to handle the whole remaining file
        tmp.dyncfg.payload = buffer_create(actual_size, NULL);
        tmp.dyncfg.payload->content_type = content_type;

        buffer_need_bytes(tmp.dyncfg.payload, actual_size);
        tmp.dyncfg.payload->len = fread(tmp.dyncfg.payload->buffer, 1, actual_size, fp);

        if (content_length != tmp.dyncfg.payload->len) {
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "DYNCFG: content_length %zu does not match actual payload size %zu for file '%s'",
                   content_length, actual_size, filename);
        }
    }

    fclose(fp);

    if(!id) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "DYNCFG: configuration file '%s' does not include a unique id. Ignoring it.",
               filename);

        dyncfg_cleanup(&tmp);
        return;
    }

    tmp.dyncfg.status = DYNCFG_STATUS_ORPHAN;
    tmp.dyncfg.restart_required = false;

    dyncfg_set_current_from_dyncfg(&tmp);

    dictionary_set(dyncfg_globals.nodes, id, &tmp, sizeof(tmp));

    // check if we need to rename the file
    CLEAN_CHAR_P *fixed_id = dyncfg_escape_id_for_filename(id);
    char fixed_filename[PATH_MAX];
    snprintf(fixed_filename, sizeof(fixed_filename), "%s/%s.dyncfg", dyncfg_globals.dir, fixed_id);

    if(strcmp(filename, fixed_filename) != 0) {
        if(rename(filename, fixed_filename) != 0)
            nd_log(NDLS_DAEMON, NDLP_ERR,
                "DYNCFG: cannot rename file '%s' into '%s'. Saving a new configuraton may not overwrite the old one.",
                filename, fixed_filename);
    }
}

void dyncfg_load_all(void) {
    DIR *dir = opendir(dyncfg_globals.dir);
    if (!dir) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG: cannot open directory '%s'", dyncfg_globals.dir);
        return;
    }

    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if ((entry->d_type == DT_REG || entry->d_type == DT_LNK) && strendswith(entry->d_name, ".dyncfg"))
            dyncfg_file_load(entry->d_name);
    }

    closedir(dir);
}

// ----------------------------------------------------------------------------
// schemas loading

static bool dyncfg_read_file_to_buffer(const char *filename, BUFFER *dst) {
    int fd = open(filename, O_RDONLY | O_CLOEXEC, 0666);
    if(unlikely(fd == -1))
        return false;

    struct stat st = { 0 };
    if(fstat(fd, &st) != 0) {
        close(fd);
        return false;
    }

    buffer_flush(dst);
    buffer_need_bytes(dst, st.st_size + 1); // +1 for the terminating zero

    ssize_t r = read(fd, (char*)dst->buffer, st.st_size);
    if(unlikely(r == -1)) {
        close(fd);
        return false;
    }
    dst->len = r;
    dst->buffer[dst->len] = '\0';

    close(fd);
    return true;
}

static bool dyncfg_get_schema_from(const char *dir, const char *id, BUFFER *dst) {
    char filename[FILENAME_MAX + 1];

    CLEAN_CHAR_P *escaped_id = dyncfg_escape_id_for_filename(id);
    snprintfz(filename, sizeof(filename), "%s/schema.d/%s.json", dir, escaped_id);
    if(dyncfg_read_file_to_buffer(filename, dst))
        return true;

    snprintfz(filename, sizeof(filename), "%s/schema.d/%s.json", dir, id);
    if(dyncfg_read_file_to_buffer(filename, dst))
        return true;

    return false;
}

bool dyncfg_get_schema(const char *id, BUFFER *dst) {
    if(dyncfg_get_schema_from(netdata_configured_user_config_dir, id, dst))
        return true;

    if(dyncfg_get_schema_from(netdata_configured_stock_config_dir, id, dst))
        return true;

    return false;
}
