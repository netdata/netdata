// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_calls.h"
#include "claim/claim.h"

static char *netdata_random_session_id_filename = NULL;
static nd_uuid_t netdata_random_session_id = { 0 };

bool netdata_random_session_id_generate(void) {
    static char guid[UUID_STR_LEN] = "";

    uuid_generate_random(netdata_random_session_id);
    uuid_unparse_lower(netdata_random_session_id, guid);

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata_random_session_id", netdata_configured_varlib_dir);

    bool ret = true;

    (void)unlink(filename);

    // save it
    int fd = open(filename, O_WRONLY|O_CREAT|O_TRUNC|O_CLOEXEC, 640);
    if(fd == -1) {
        netdata_log_error("Cannot create random session id file '%s'.", filename);
        ret = false;
    }
    else {
        if (write(fd, guid, UUID_STR_LEN - 1) != UUID_STR_LEN - 1) {
            netdata_log_error("Cannot write the random session id file '%s'.", filename);
            ret = false;
        } else {
            ssize_t bytes = write(fd, "\n", 1);
            UNUSED(bytes);
        }
        close(fd);
    }

    if(ret && (!netdata_random_session_id_filename || strcmp(netdata_random_session_id_filename, filename) != 0)) {
        freez(netdata_random_session_id_filename);
        netdata_random_session_id_filename = strdupz(filename);
    }

    return ret;
}

static const char *netdata_random_session_id_get_filename(void) {
    if(!netdata_random_session_id_filename)
        netdata_random_session_id_generate();

    return netdata_random_session_id_filename;
}

static bool netdata_random_session_id_matches(const char *guid) {
    if(uuid_is_null(netdata_random_session_id))
        return false;

    nd_uuid_t uuid;

    if(uuid_parse(guid, uuid))
        return false;

    if(uuid_compare(netdata_random_session_id, uuid) == 0)
        return true;

    return false;
}

static bool check_claim_param(const char *s) {
    if(!s || !*s) return true;

    do {
        if(isalnum((uint8_t)*s) || *s == '.' || *s == ',' || *s == '-' || *s == ':' || *s == '/' || *s == '_')
            ;
        else
            return false;

    } while(*++s);

    return true;
}

static bool agent_can_be_claimed(void) {
    CLOUD_STATUS status = cloud_status();
    switch(status) {
        case CLOUD_STATUS_AVAILABLE:
        case CLOUD_STATUS_OFFLINE:
        case CLOUD_STATUS_INDIRECT:
            return true;

        case CLOUD_STATUS_BANNED:
        case CLOUD_STATUS_ONLINE:
            return false;
    }

    return false;
}

typedef enum {
    CLAIM_RESP_INFO,
    CLAIM_RESP_ERROR,
    CLAIM_RESP_ACTION_OK,
    CLAIM_RESP_ACTION_FAILED,
} CLAIM_RESPONSE;

static void claim_add_user_info_command(BUFFER *wb) {
    const char *filename = netdata_random_session_id_get_filename();
    CLEAN_BUFFER *os_cmd = buffer_create(0, NULL);

    const char *os_filename;
    const char *os_prefix;
    const char *os_quote;
    const char *os_message;

#if defined(OS_WINDOWS)
    char win_path[MAX_PATH];
    cygwin_conv_path(CCP_POSIX_TO_WIN_A, filename, win_path, sizeof(win_path));
    os_filename = win_path;
    os_prefix = "more";
    os_message = "We need to verify this Windows server is yours. So, open a Command Prompt on this server to run the command. It will give you a UUID. Copy and paste this UUID to this box:";
#else
    os_filename = filename;
    os_prefix = "sudo cat";
    os_message = "We need to verify this server is yours. SSH to this server and run this command. It will give you a UUID. Copy and paste this UUID to this box:";
#endif

    // add quotes only when the filename has a space
    if(strchr(os_filename, ' '))
        os_quote = "\"";
    else
        os_quote = "";

    buffer_sprintf(os_cmd, "%s %s%s%s", os_prefix, os_quote, os_filename, os_quote);

    buffer_json_member_add_string(wb, "key_filename", os_filename);
    buffer_json_member_add_string(wb, "cmd", buffer_tostring(os_cmd));
    buffer_json_member_add_string(wb, "help", os_message);
}

static int claim_json_response(BUFFER *wb, CLAIM_RESPONSE response, const char *msg) {
    time_t now_s = now_realtime_sec();
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    if(response != CLAIM_RESP_INFO) {
        // this is not an info, so it needs a status report
        buffer_json_member_add_boolean(wb, "success", response == CLAIM_RESP_ACTION_OK ? true : false);
        buffer_json_member_add_string_or_empty(wb, "message", msg ? msg : "");
    }

    buffer_json_cloud_status(wb, now_s);

    if(response != CLAIM_RESP_ACTION_OK) {
        buffer_json_member_add_boolean(wb, "can_be_claimed", agent_can_be_claimed());
        claim_add_user_info_command(wb);
    }

    buffer_json_agents_v2(wb, NULL, now_s, false, false);
    buffer_json_finalize(wb);

    return (response == CLAIM_RESP_ERROR) ? HTTP_RESP_BAD_REQUEST : HTTP_RESP_OK;
}

static int claim_txt_response(BUFFER *wb, const char *msg) {
    buffer_reset(wb);
    buffer_strcat(wb, msg);
    return HTTP_RESP_BAD_REQUEST;
}

static int api_claim(uint8_t version, struct web_client *w, char *url) {
    char *key = NULL;
    char *token = NULL;
    char *rooms = NULL;
    char *base_url = NULL;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        if(!strcmp(name, "key"))
            key = value;
        else if(!strcmp(name, "token"))
            token = value;
        else if(!strcmp(name, "rooms"))
            rooms = value;
        else if(!strcmp(name, "url"))
            base_url = value;
    }

    BUFFER *wb = w->response.data;

    CLAIM_RESPONSE response = CLAIM_RESP_INFO;
    const char *msg = NULL;
    bool can_be_claimed = agent_can_be_claimed();

    if(can_be_claimed && key) {
        if(!netdata_random_session_id_matches(key)) {
            netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it
            if(version < 3) return claim_txt_response(wb, "invalid key");
            return claim_json_response(wb, CLAIM_RESP_ERROR, "invalid key");
        }

        if(!token || !base_url || !check_claim_param(token) || !check_claim_param(base_url) || (rooms && !check_claim_param(rooms))) {
            netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it
            if(version < 3) return claim_txt_response(wb, "invalid parameters");
            return claim_json_response(wb, CLAIM_RESP_ERROR, "invalid parameters");
        }

        netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it

        if(claim_agent(base_url, token, rooms, cloud_config_proxy_get(), cloud_config_insecure_get())) {
            msg = "ok";
            can_be_claimed = false;
            claim_reload_and_wait_online();
            response = CLAIM_RESP_ACTION_OK;
        }
        else {
            msg = claim_agent_failure_reason_get();
            response = CLAIM_RESP_ACTION_FAILED;
        }
    }

    return claim_json_response(wb, response, msg);
}

int api_v2_claim(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_claim(2, w, url);
}

int api_v3_claim(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    return api_claim(3, w, url);
}
