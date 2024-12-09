// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * /api/v3/settings
 *
 * QUERY STRING PARAMETERS:
 * - file=a file name (alphanumerics, dashes, underscores)
 *   When the user is not authenticated with a bearer token
 *   only the 'default' file is allowed.
 *   Authenticated users can create, store and update any
 *   settings file.
 *
 * HTTP METHODS
 * - GET to retrieve a file
 * - PUT to create or update a file
 *
 * PAYLOAD
 * - The payload MUST have the member 'version'.
 * - The payload MAY have anything else.
 * - The maximum payload size in JSON is 20MiB.
 * - When updating the payload, the caller must specify the
 *   version of the existing file. If this check fails,
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

// we need an r/w spinlock to ensure that reads and write do not happen
// concurrently for settings files
static RW_SPINLOCK settings_spinlock = RW_SPINLOCK_INITIALIZER;

static inline void settings_path(char out[FILENAME_MAX]) {
    filename_from_path_entry(out, netdata_configured_varlib_dir, "settings", NULL);
}

static inline void settings_filename(char out[FILENAME_MAX], const char *file, const char *extension) {
    char path[FILENAME_MAX];
    settings_path(path);
    filename_from_path_entry(out, path, file, extension);
}

static inline bool settings_ensure_path_exists(void) {
    char path[FILENAME_MAX];
    settings_path(path);
    return filename_is_dir(path, true);
}

static inline size_t settings_extract_json_version(const char *json) {
    if(!json || !*json) return 0;

    // Parse the JSON string into a JSON-C object
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(json);
    if (jobj == NULL)
        return 0;

    // Access the "version" field
    struct json_object *version_obj;
    if (json_object_object_get_ex(jobj, "version", &version_obj))
        // Extract the integer value of the version
        return (size_t)json_object_get_int(version_obj);

    return 0;
}

static inline void settings_initial_version(BUFFER *wb) {
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "version", 1);
    buffer_json_finalize(wb);
}

static inline void settings_get(BUFFER *wb, const char *file, bool have_lock) {
    char filename[FILENAME_MAX];
    settings_filename(filename, file, NULL);

    buffer_reset(wb);

    if(!have_lock)
        rw_spinlock_read_lock(&settings_spinlock);

    bool rc = read_txt_file_to_buffer(filename, wb, MAX_SETTINGS_SIZE_BYTES);

    if(!have_lock)
        rw_spinlock_read_unlock(&settings_spinlock);

    if(rc) {
        size_t version = settings_extract_json_version(buffer_tostring(wb));
        if (!version) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "file '%s' cannot be parsed to extract version", filename);
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

static inline size_t settings_get_version(const char *path, bool have_lock) {
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    settings_get(wb, path, have_lock);

    return settings_extract_json_version(buffer_tostring(wb));
}

static inline int settings_put(struct web_client *w, char *file) {
    rw_spinlock_write_lock(&settings_spinlock);

    if(!settings_ensure_path_exists()) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return rrd_call_function_error(
            w->response.data,
            "Settings path cannot be created or accessed.",
            HTTP_RESP_BAD_REQUEST);
    }

    size_t old_version = settings_get_version(file, true);

    // Parse the JSON string into a JSON-C object
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(w->payload));
    if (jobj == NULL) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return rrd_call_function_error(
            w->response.data,
            "Payload cannot be parsed as a JSON object",
            HTTP_RESP_BAD_REQUEST);
    }

    // Access the "version" field
    struct json_object *version_obj;
    if (!json_object_object_get_ex(jobj, "version", &version_obj)) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return rrd_call_function_error(
            w->response.data,
            "Field version is not found in payload",
            HTTP_RESP_BAD_REQUEST);
    }

    size_t new_version = (size_t)json_object_get_int(version_obj);

    if (old_version != new_version) {
        rw_spinlock_write_unlock(&settings_spinlock);
        return rrd_call_function_error(
            w->response.data,
            "Payload version does not match the version of the stored object",
            HTTP_RESP_CONFLICT);
    }

    new_version++;
    // Set the new version back into the JSON object
    json_object_object_add(jobj, "version", json_object_new_int((int)new_version));

    // Convert the updated JSON object back to a string
    const char *updated_json_str = json_object_to_json_string(jobj);

    char tmp_filename[FILENAME_MAX];
    settings_filename(tmp_filename, file, "new");

    // Save the updated JSON string to a file
    FILE *fp = fopen(tmp_filename, "w");
    if (fp == NULL) {
        rw_spinlock_write_unlock(&settings_spinlock);
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot open/create settings file '%s'", tmp_filename);
        return rrd_call_function_error(
            w->response.data,
            "Cannot create payload file '%s'",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }
    size_t len = strlen(updated_json_str);
    if(fwrite(updated_json_str, 1, len, fp) != len) {
        fclose(fp);
        unlink(tmp_filename);
        rw_spinlock_write_unlock(&settings_spinlock);
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot save settings to file '%s'", tmp_filename);
        return rrd_call_function_error(
            w->response.data,
            "Cannot save payload to file '%s'",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }
    fclose(fp);

    char filename[FILENAME_MAX];
    settings_filename(filename, file, NULL);

    bool renamed = rename(tmp_filename, filename) == 0;

    rw_spinlock_write_unlock(&settings_spinlock);

    if(!renamed) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "cannot rename file '%s' to '%s'", tmp_filename, filename);
        return rrd_call_function_error(
            w->response.data,
            "Failed to move the payload file to its final location",
            HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    return rrd_call_function_error(
        w->response.data,
        "OK",
        HTTP_RESP_OK);
}

static inline bool is_settings_file_valid(char *file) {
    char *s = file;

    if(!s || !*s)
        return false;

    while(*s) {
        if(!isalnum((uint8_t)*s) && *s != '-' && *s != '_')
            return false;
        s++;
    }

    return true;
}

int api_v3_settings(RRDHOST *host, struct web_client *w, char *url) {
    char *file = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "file"))
            file = value;
    }

    if(!is_settings_file_valid(file))
        return rrd_call_function_error(
            w->response.data,
            "Invalid settings file given.",
            HTTP_RESP_BAD_REQUEST);

    if(host != localhost)
        return rrd_call_function_error(
            w->response.data,
            "Settings API is only allowed for the agent node.",
            HTTP_RESP_BAD_REQUEST);

    if(web_client_flags_check_auth(w) != WEB_CLIENT_FLAG_AUTH_BEARER && strcmp(file, "default") != 0)
        return rrd_call_function_error(
            w->response.data,
            "Only the 'default' settings file is allowed for anonymous users",
            HTTP_RESP_BAD_REQUEST);

    switch(w->mode) {
        case HTTP_REQUEST_MODE_GET:
            settings_get(w->response.data, file, false);
            return HTTP_RESP_OK;

        case HTTP_REQUEST_MODE_PUT:
            if(!w->payload || !buffer_strlen(w->payload))
                return rrd_call_function_error(
                    w->response.data,
                    "Settings API PUT action requires a payload.",
                    HTTP_RESP_BAD_REQUEST);

            return settings_put(w, file);

        default:
            return rrd_call_function_error(w->response.data,
                                           "Invalid HTTP mode. HTTP modes GET and PUT are supported.",
                                           HTTP_RESP_BAD_REQUEST);
    }
}
