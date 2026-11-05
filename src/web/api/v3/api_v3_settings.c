// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * /api/v3/settings
 *
 * There is one settings object per agent: 'default'. It is shared by
 * every caller admitted to this API (anonymous or bearer). The dashboard
 * keeps agent-wide preferences in it (e.g. the preferred nodes of the
 * agent's local space).
 *
 * QUERY STRING PARAMETERS:
 * - file=default (optional; an empty value counts as omitted;
 *   any other value is rejected)
 *
 * HTTP METHODS
 * - GET to retrieve the settings object
 * - PUT to create or update the settings object
 *
 * PAYLOAD
 * - The payload MUST have the member 'version', a positive integer.
 * - The payload MAY have anything else.
 * - The maximum payload size in JSON is 20MiB.
 * - When updating the payload, the caller must specify the
 *   version of the stored object. If this check fails,
 *   Netdata will return 409 (conflict).
 *   When the caller receives 409, it means there are updates
 *   in the payload outside its control and the object MUST
 *   be loaded again to find its current version to update it.
 *   After loading it, the caller must reapply the changes and
 *   PUT it again.
 * - Netdata will increase the version on every PUT action.
 *   So, the payload MUST specify the version found on disk
 *   but, Netdata will increment the version before saving it.
 */

#include "api_v3_calls.h"

#define MAX_SETTINGS_SIZE_BYTES (20 * 1024 * 1024)
#define SETTINGS_FILE "default"

// we need an r/w spinlock to ensure that reads and write do not happen
// concurrently for settings files
static RW_SPINLOCK settings_spinlock = RW_SPINLOCK_INITIALIZER;

static inline void settings_path(char out[FILENAME_MAX]) {
    filename_from_path_entry(out, netdata_configured_varlib_dir, "settings", NULL);
}

static inline void settings_filename(char out[FILENAME_MAX], const char *extension) {
    char path[FILENAME_MAX];
    settings_path(path);
    filename_from_path_entry(out, path, SETTINGS_FILE, extension);
}

static inline bool settings_ensure_path_exists(void) {
    char path[FILENAME_MAX];
    settings_path(path);
    return filename_is_dir(path, true);
}

// a valid version is a JSON integer from 1 to INT64_MAX - 1; settings_put() refuses to increment
// INT64_MAX - 1, so a stored version always stays valid
// returns 0 when the version is missing or invalid
static inline int64_t settings_json_object_version(struct json_object *jobj) {
    struct json_object *version_obj;
    if(!json_object_object_get_ex(jobj, "version", &version_obj) || !json_object_is_type(version_obj, json_type_int))
        return 0;

    int64_t version = json_object_get_int64(version_obj);
    return (version > 0 && version < INT64_MAX) ? version : 0;
}

static inline int64_t settings_extract_json_version(const char *json) {
    if(!json || !*json) return 0;

    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json);
    if (jobj == NULL)
        return 0;

    return settings_json_object_version(jobj);
}

static inline void settings_initial_version(BUFFER *wb) {
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "version", 1);
    buffer_json_finalize(wb);
}

static inline void settings_get(BUFFER *wb, bool have_lock) {
    char filename[FILENAME_MAX];
    settings_filename(filename, NULL);

    buffer_reset(wb);

    if(!have_lock)
        rw_spinlock_read_lock(&settings_spinlock);

    bool rc = read_txt_file_to_buffer(filename, wb, MAX_SETTINGS_SIZE_BYTES);

    if(!have_lock)
        rw_spinlock_read_unlock(&settings_spinlock);

    if(rc) {
        int64_t version = settings_extract_json_version(buffer_tostring(wb));
        if (!version) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "settings file '%s' has no valid version; serving the initial object", filename);
            settings_initial_version(wb);
        }
        else {
            wb->content_type = CT_APPLICATION_JSON;
            buffer_no_cacheable(wb);
        }
    }
    else
        settings_initial_version(wb);
}

static inline int64_t settings_get_version(bool have_lock) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    settings_get(wb, have_lock);

    return settings_extract_json_version(buffer_tostring(wb));
}

static FILE *settings_open_tmp_file(const char *filename) {
    struct stat before;
    bool reuse = lstat(filename, &before) == 0;
    if(reuse && !S_ISREG(before.st_mode)) {
        errno = EINVAL;
        return NULL;
    }

    if(!reuse && errno != ENOENT)
        return NULL;

    int flags = O_WRONLY | O_CLOEXEC | O_NONBLOCK;
#ifdef O_NOFOLLOW
    flags |= O_NOFOLLOW;
#endif
    if(!reuse)
        flags |= O_CREAT | O_EXCL;

    int fd = open(filename, flags, 0666);
    if(fd == -1)
        return NULL;

    struct stat after;
    if(fstat(fd, &after) != 0) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
        return NULL;
    }

    if(!S_ISREG(after.st_mode) ||
       (reuse && (before.st_dev != after.st_dev || before.st_ino != after.st_ino))) {
        close(fd);
        errno = EINVAL;
        return NULL;
    }

    if(reuse && ftruncate(fd, 0) != 0) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
        return NULL;
    }

    FILE *fp = fdopen(fd, "w");
    if(!fp) {
        int saved_errno = errno;
        close(fd);
        errno = saved_errno;
    }

    return fp;
}

static bool settings_write_tmp_file(FILE *fp, const char *data, size_t len) {
    size_t written = fwrite(data, 1, len, fp);
    int saved_errno = errno;
    bool failed = ferror(fp) || written != len;

    if(fclose(fp) != 0) {
        if(!failed)
            saved_errno = errno;

        failed = true;
    }

    if(failed)
        errno = saved_errno;

    return !failed;
}

static inline int settings_put(struct web_client *w) {
    rw_spinlock_write_lock(&settings_spinlock);

    if(!settings_ensure_path_exists()) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Settings path cannot be created or accessed.",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    int64_t old_version = settings_get_version(true);

    // Parse the JSON string into a JSON-C object
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(w->payload));
    if (jobj == NULL) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Payload cannot be parsed as a JSON object",
            HTTP_RESP_BAD_REQUEST);
    }

    // Access the "version" field
    struct json_object *version_obj;
    if (!json_object_object_get_ex(jobj, "version", &version_obj)) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Field version is not found in payload",
            HTTP_RESP_BAD_REQUEST);
    }

    int64_t new_version = settings_json_object_version(jobj);
    if (!new_version) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Field version must be a positive integer",
            HTTP_RESP_BAD_REQUEST);
    }

    if (old_version != new_version) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Payload version does not match the version of the stored object",
            HTTP_RESP_CONFLICT);
    }

    // the stored version must stay valid after the increment
    if (new_version >= INT64_MAX - 1) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Field version cannot be incremented any further",
            HTTP_RESP_BAD_REQUEST);
    }

    new_version++;
    // Set the new version back into the JSON object
    json_object_object_add(jobj, "version", json_object_new_int64(new_version));

    // Convert the updated JSON object back to a string
    const char *updated_json_str = json_object_to_json_string(jobj);
    if (!updated_json_str) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return nrpc_call_error(
            w->response.data,
            "Cannot serialize the payload",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    char tmp_filename[FILENAME_MAX];
    settings_filename(tmp_filename, "new");

    // Save the updated JSON string to a file
    FILE *fp = settings_open_tmp_file(tmp_filename);
    if (fp == NULL) {
        rw_spinlock_write_unlock(&settings_spinlock);
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot open/create settings file '%s'", tmp_filename);
        return nrpc_call_error(
            w->response.data,
            "Cannot create payload file",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }
    if(!settings_write_tmp_file(fp, updated_json_str, strlen(updated_json_str))) {
        int saved_errno = errno;
        unlink(tmp_filename);
        rw_spinlock_write_unlock(&settings_spinlock);
        errno = saved_errno;
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot save settings to file '%s'", tmp_filename);
        return nrpc_call_error(
            w->response.data,
            "Cannot save payload to file",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    char filename[FILENAME_MAX];
    settings_filename(filename, NULL);

    bool renamed = rename(tmp_filename, filename) == 0;
    if(!renamed)
        unlink(tmp_filename);

    rw_spinlock_write_unlock(&settings_spinlock);

    if(!renamed) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot rename file '%s' to '%s'", tmp_filename, filename);
        return nrpc_call_error(
            w->response.data,
            "Failed to move the payload file to its final location",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    return nrpc_call_error(
        w->response.data,
        "OK",
        HTTP_RESP_OK);
}

int api_v3_settings(RRDHOST *host, struct web_client *w, char *url) {
    bool file_is_default = true;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "file") && strcmp(value, SETTINGS_FILE) != 0)
            file_is_default = false;
    }

    if(!file_is_default)
        return nrpc_call_error(
            w->response.data,
            "Only the '" SETTINGS_FILE "' settings file is supported.",
            HTTP_RESP_BAD_REQUEST);

    if(host != localhost)
        return nrpc_call_error(
            w->response.data,
            "Settings API is only allowed for the agent node.",
            HTTP_RESP_BAD_REQUEST);

    switch(w->mode) {
        case HTTP_REQUEST_MODE_GET:
            settings_get(w->response.data, false);
            return HTTP_RESP_OK;

        case HTTP_REQUEST_MODE_PUT:
            if(!w->payload || !buffer_strlen(w->payload))
                return nrpc_call_error(
                    w->response.data,
                    "Settings API PUT action requires a payload.",
                    HTTP_RESP_BAD_REQUEST);

            if(buffer_strlen(w->payload) > MAX_SETTINGS_SIZE_BYTES)
                return nrpc_call_error(
                    w->response.data,
                    "Settings API PUT payload exceeds the maximum allowed size.",
                    HTTP_RESP_CONTENT_TOO_LONG);

            return settings_put(w);

        default:
            return nrpc_call_error(w->response.data,
                                           "Invalid HTTP mode. HTTP modes GET and PUT are supported.",
                                           HTTP_RESP_BAD_REQUEST);
    }
}
