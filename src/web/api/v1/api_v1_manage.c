// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

char *api_secret;

static char *get_mgmt_api_key(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata.api.key", netdata_configured_varlib_dir);
    const char *api_key_filename = inicfg_get_filename(&netdata_config, CONFIG_SECTION_REGISTRY, "netdata management api key file", filename);
    static char guid[GUID_LEN + 1] = "";

    if(likely(guid[0]))
        return guid;

    // read it from disk
#ifdef O_NOFOLLOW
    int fd = open(api_key_filename, O_RDONLY | O_CLOEXEC | O_NOFOLLOW);
#else
    int fd = open(api_key_filename, O_RDONLY | O_CLOEXEC);
#endif
    if(fd != -1) {
        struct stat st;
        if(fstat(fd, &st) != 0 || !S_ISREG(st.st_mode))
            netdata_log_error("Management API key file '%s' is not a regular file, regenerating.", api_key_filename);
        else {
            char buf[GUID_LEN + 1];
            if(read(fd, buf, GUID_LEN) != GUID_LEN)
                netdata_log_error("Failed to read management API key from '%s'", api_key_filename);
            else {
                buf[GUID_LEN] = '\0';
                if(regenerate_guid(buf, guid) == -1) {
                    netdata_log_error("Failed to validate management API key '%s' from '%s'.",
                                      buf, api_key_filename);

                    guid[0] = '\0';
                }
            }
        }
        close(fd);
    }

    // generate a new one?
    if(!guid[0]) {
        nd_uuid_t uuid;

        uuid_generate(uuid);
        uuid_unparse_lower(uuid, guid);
        guid[GUID_LEN] = '\0';

        // save it
#ifdef O_NOFOLLOW
        struct stat st;
        // O_RDWR avoids blocking on FIFOs (O_WRONLY blocks until a reader arrives).
        // O_TRUNC is omitted so truncation only happens after fstat() confirms a regular file.
        fd = open(api_key_filename, O_RDWR|O_CREAT|O_CLOEXEC|O_NOFOLLOW, 0600);
        if(fd == -1) {
            netdata_log_error("Cannot create unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file.", api_key_filename);
            goto temp_key;
        }

        if(fstat(fd, &st) != 0 || !S_ISREG(st.st_mode)) {
            netdata_log_error("Management API key file '%s' is not a regular file.", api_key_filename);
            close(fd);
            goto temp_key;
        }

        if(ftruncate(fd, 0) != 0) {
            netdata_log_error("Cannot truncate management API key file '%s'.", api_key_filename);
            close(fd);
            goto temp_key;
        }

        if(write(fd, guid, GUID_LEN) != GUID_LEN) {
            netdata_log_error("Cannot write the unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file with enough space left.", api_key_filename);
            close(fd);
            goto temp_key;
        }

        close(fd);
#else
        // Without O_NOFOLLOW: write to a uniquely named temp file then rename atomically.
        // Use mkstemp() so the temporary filename is not predictable.
        // rename() replaces the destination entry without following symlinks there,
        // so a symlink planted at api_key_filename cannot redirect truncation to another file.
        char tmp_filename[FILENAME_MAX + 1];
        snprintfz(tmp_filename, FILENAME_MAX, "%s.tmp.XXXXXX", api_key_filename);

        fd = mkstemp(tmp_filename);
        if(fd == -1) {
            netdata_log_error("Cannot create temporary management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file.", tmp_filename);
            goto temp_key;
        }

        if(write(fd, guid, GUID_LEN) != GUID_LEN) {
            netdata_log_error("Cannot write the unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file with enough space left.", tmp_filename);
            close(fd);
            unlink(tmp_filename);
            goto temp_key;
        }
        close(fd);

        if(rename(tmp_filename, api_key_filename) != 0) {
            netdata_log_error("Cannot rename temporary API key file '%s' to '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file.", tmp_filename, api_key_filename);
            unlink(tmp_filename);
            goto temp_key;
        }
#endif
    }

    return guid;

temp_key:
    netdata_log_info("You can still continue to use the alarm management API using the authorization token %s during this Netdata session only.", guid);
    return guid;
}

void api_v1_management_init(void) {
    api_secret = get_mgmt_api_key();
}

#define HLT_MGM "manage/health"
int api_v1_manage(RRDHOST *host, struct web_client *w, char *url) {
    const char *haystack = buffer_tostring(w->url_path_decoded);
    char *needle;

    buffer_flush(w->response.data);

    if ((needle = strstr(haystack, HLT_MGM)) == NULL) {
        buffer_strcat(w->response.data, "Invalid management request. Curently only 'health' is supported.");
        return HTTP_RESP_NOT_FOUND;
    }
    needle += strlen(HLT_MGM);
    if (*needle != '\0') {
        buffer_strcat(w->response.data, "Invalid management request. Currently only 'health' is supported.");
        return HTTP_RESP_NOT_FOUND;
    }
    return web_client_api_request_v1_mgmt_health(host, w, url);
}
