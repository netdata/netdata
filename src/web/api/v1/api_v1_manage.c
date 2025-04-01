// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

char *api_secret;

static char *get_mgmt_api_key(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata.api.key", netdata_configured_varlib_dir);
    const char *api_key_filename = inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "netdata management api key file", filename);
    static char guid[GUID_LEN + 1] = "";

    if(likely(guid[0]))
        return guid;

    // read it from disk
    int fd = open(api_key_filename, O_RDONLY | O_CLOEXEC);
    if(fd != -1) {
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
        close(fd);
    }

    // generate a new one?
    if(!guid[0]) {
        nd_uuid_t uuid;

        uuid_generate(uuid);
        uuid_unparse_lower(uuid, guid);
        guid[GUID_LEN] = '\0';

        // save it
        fd = open(api_key_filename, O_WRONLY|O_CREAT|O_TRUNC | O_CLOEXEC, 444);
        if(fd == -1) {
            netdata_log_error("Cannot create unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file.", api_key_filename);
            goto temp_key;
        }

        if(write(fd, guid, GUID_LEN) != GUID_LEN) {
            netdata_log_error("Cannot write the unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file with enough space left.", api_key_filename);
            close(fd);
            goto temp_key;
        }

        close(fd);
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
