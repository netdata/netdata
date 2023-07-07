// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"
#include "registry/registry_internals.h"
#include "aclk/aclk.h"
#include "aclk/aclk_proxy.h"

char *claiming_pending_arguments = NULL;

static char *claiming_errors[] = {
        "Agent claimed successfully",                   // 0
        "Unknown argument",                             // 1
        "Problems with claiming working directory",     // 2
        "Missing dependencies",                         // 3
        "Failure to connect to endpoint",               // 4
        "The CLI didn't work",                          // 5
        "Wrong user",                                   // 6
        "Unknown HTTP error message",                   // 7
        "invalid node id",                              // 8
        "invalid node name",                            // 9
        "invalid room id",                              // 10
        "invalid public key",                           // 11
        "token expired/token not found/invalid token",  // 12
        "already claimed",                              // 13
        "processing claiming",                          // 14
        "Internal Server Error",                        // 15
        "Gateway Timeout",                              // 16
        "Service Unavailable",                          // 17
        "Agent Unique Id Not Readable"                  // 18
};

/* Retrieve the claim id for the agent.
 * Caller owns the string.
*/
char *get_agent_claimid()
{
    char *result;
    rrdhost_aclk_state_lock(localhost);
    result = (localhost->aclk_state.claimed_id == NULL) ? NULL : strdupz(localhost->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(localhost);
    return result;
}

#define CLAIMING_COMMAND_LENGTH 16384
#define CLAIMING_PROXY_LENGTH (CLAIMING_COMMAND_LENGTH/4)

extern struct registry registry;

/* rrd_init() and post_conf_load() must have been called before this function */
CLAIM_AGENT_RESPONSE claim_agent(const char *claiming_arguments, bool force, const char **msg)
{
    if (!force || !netdata_cloud_enabled) {
        netdata_log_error("Refusing to claim agent -> cloud functionality has been disabled");
        return CLAIM_AGENT_CLOUD_DISABLED;
    }

#ifndef DISABLE_CLOUD
    int exit_code;
    pid_t command_pid;
    char command_buffer[CLAIMING_COMMAND_LENGTH + 1];
    FILE *fp_child_output, *fp_child_input;

    // This is guaranteed to be set early in main via post_conf_load()
    char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
    if (cloud_base_url == NULL) {
        internal_fatal(true, "Do not move the cloud base url out of post_conf_load!!");
        return CLAIM_AGENT_NO_CLOUD_URL;
    }

    const char *proxy_str;
    ACLK_PROXY_TYPE proxy_type;
    char proxy_flag[CLAIMING_PROXY_LENGTH] = "-noproxy";

    proxy_str = aclk_get_proxy(&proxy_type);

    if (proxy_type == PROXY_TYPE_SOCKS5 || proxy_type == PROXY_TYPE_HTTP)
        snprintf(proxy_flag, CLAIMING_PROXY_LENGTH, "-proxy=\"%s\"", proxy_str);

    snprintfz(command_buffer,
              CLAIMING_COMMAND_LENGTH,
              "exec netdata-claim.sh %s -hostname=%s -id=%s -url=%s -noreload %s",
              proxy_flag,
              netdata_configured_hostname,
              localhost->machine_guid,
              cloud_base_url,
              claiming_arguments);

    netdata_log_info("Executing agent claiming command 'netdata-claim.sh'");
    fp_child_output = netdata_popen(command_buffer, &command_pid, &fp_child_input);
    if(!fp_child_output) {
        netdata_log_error("Cannot popen(\"%s\").", command_buffer);
        return CLAIM_AGENT_CANNOT_EXECUTE_CLAIM_SCRIPT;
    }
    netdata_log_info("Waiting for claiming command to finish.");
    while (fgets(command_buffer, CLAIMING_COMMAND_LENGTH, fp_child_output) != NULL) {;}
    exit_code = netdata_pclose(fp_child_input, fp_child_output, command_pid);
    netdata_log_info("Agent claiming command returned with code %d", exit_code);
    if (0 == exit_code) {
        load_claiming_state();
        return CLAIM_AGENT_OK;
    }
    if (exit_code < 0) {
        netdata_log_error("Agent claiming command failed to complete its run.");
        return CLAIM_AGENT_CLAIM_SCRIPT_FAILED;
    }
    errno = 0;
    unsigned maximum_known_exit_code = sizeof(claiming_errors) / sizeof(claiming_errors[0]) - 1;

    if ((unsigned)exit_code > maximum_known_exit_code) {
        netdata_log_error("Agent failed to be claimed with an unknown error.");
        return CLAIM_AGENT_CLAIM_SCRIPT_RETURNED_INVALID_CODE;
    }

    netdata_log_error("Agent failed to be claimed with the following error message:");
    netdata_log_error("\"%s\"", claiming_errors[exit_code]);

    if(msg) *msg = claiming_errors[exit_code];

#else
    UNUSED(claiming_arguments);
    UNUSED(claiming_errors);
#endif

    return CLAIM_AGENT_FAILED_WITH_MESSAGE;
}

#ifdef ENABLE_ACLK
extern int aclk_connected, aclk_kill_link, aclk_disable_runtime;
#endif

/* Change the claimed state of the agent.
 *
 * This only happens when the user has explicitly requested it:
 *   - via the cli tool by reloading the claiming state
 *   - after spawning the claim because of a command-line argument
 * If this happens with the ACLK active under an old claim then we MUST KILL THE LINK
 */
void load_claiming_state(void)
{
    // --------------------------------------------------------------------
    // Check if the cloud is enabled
#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
    netdata_cloud_enabled = false;
#else
    uuid_t uuid;

    // Propagate into aclk and registry. Be kind of atomic...
    appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", DEFAULT_CLOUD_BASE_URL);

    rrdhost_aclk_state_lock(localhost);
    if (localhost->aclk_state.claimed_id) {
        if (aclk_connected)
            localhost->aclk_state.prev_claimed_id = strdupz(localhost->aclk_state.claimed_id);
        freez(localhost->aclk_state.claimed_id);
        localhost->aclk_state.claimed_id = NULL;
    }
    if (aclk_connected)
    {
        netdata_log_info("Agent was already connected to Cloud - forcing reconnection under new credentials");
        aclk_kill_link = 1;
    }
    aclk_disable_runtime = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/cloud.d/claimed_id", netdata_configured_varlib_dir);

    long bytes_read;
    char *claimed_id = read_by_filename(filename, &bytes_read);
    if(claimed_id && uuid_parse(claimed_id, uuid)) {
        netdata_log_error("claimed_id \"%s\" doesn't look like valid UUID", claimed_id);
        freez(claimed_id);
        claimed_id = NULL;
    }

    if(claimed_id) {
        localhost->aclk_state.claimed_id = mallocz(UUID_STR_LEN);
        uuid_unparse_lower(uuid, localhost->aclk_state.claimed_id);
    }

    invalidate_node_instances(&localhost->host_uuid, claimed_id ? &uuid : NULL);
    metaqueue_store_claim_id(&localhost->host_uuid, claimed_id ? &uuid : NULL);
    rrdhost_aclk_state_unlock(localhost);

    if (!claimed_id) {
        netdata_log_info("Unable to load '%s', setting state to AGENT_UNCLAIMED", filename);
        return;
    }

    freez(claimed_id);

    netdata_log_info("File '%s' was found. Setting state to AGENT_CLAIMED.", filename);
    netdata_cloud_enabled = appconfig_get_boolean_ondemand(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", netdata_cloud_enabled);
#endif
}

struct config cloud_config = { .first_section = NULL,
                               .last_section = NULL,
                               .mutex = NETDATA_MUTEX_INITIALIZER,
                               .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                          .rwlock = AVL_LOCK_INITIALIZER } };

void load_cloud_conf(int silent)
{
    char *nd_disable_cloud = getenv("NETDATA_DISABLE_CLOUD");
    if (nd_disable_cloud && !strncmp(nd_disable_cloud, "1", 1))
        netdata_cloud_enabled = CONFIG_BOOLEAN_NO;

    char *filename;
    errno = 0;

    int ret = 0;

    filename = strdupz_path_subpath(netdata_configured_varlib_dir, "cloud.d/cloud.conf");

    ret = appconfig_load(&cloud_config, filename, 1, NULL);
    if(!ret && !silent)
        netdata_log_info("CONFIG: cannot load cloud config '%s'. Running with internal defaults.", filename);

    freez(filename);

    // --------------------------------------------------------------------
    // Check if the cloud is enabled

#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
    netdata_cloud_enabled = CONFIG_BOOLEAN_NO;
#else
    netdata_cloud_enabled = appconfig_get_boolean_ondemand(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", netdata_cloud_enabled);
#endif

    // This must be set before any point in the code that accesses it. Do not move it from this function.
    appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", DEFAULT_CLOUD_BASE_URL);
}

static char *netdata_random_session_id_filename = NULL;
static uuid_t netdata_random_session_id = { 0 };

bool netdata_random_session_id_generate(void) {
    static char guid[UUID_STR_LEN] = "";

    uuid_generate_random(netdata_random_session_id);
    uuid_unparse_lower(netdata_random_session_id, guid);

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata_random_session_id", netdata_configured_varlib_dir);

    bool ret = true;

    (void)unlink(filename);

    // save it
    int fd = open(filename, O_WRONLY|O_CREAT|O_TRUNC, 640);
    if(fd == -1) {
        netdata_log_error("Cannot create random session id file '%s'.", filename);
        ret = false;
    }
    else {
        if (write(fd, guid, UUID_STR_LEN - 1) != UUID_STR_LEN - 1) {
            netdata_log_error("Cannot write the random session id file '%s'.", filename);
            ret = false;
        } else
            (void) write(fd, "\n", 1);
        close(fd);
    }

    if(ret && (!netdata_random_session_id_filename || strcmp(netdata_random_session_id_filename, filename) != 0)) {
        freez(netdata_random_session_id_filename);
        netdata_random_session_id_filename = strdupz(filename);
    }

    return ret;
}

const char *netdata_random_session_id_get_filename(void) {
    if(!netdata_random_session_id_filename)
        netdata_random_session_id_generate();

    return netdata_random_session_id_filename;
}

bool netdata_random_session_id_matches(const char *guid) {
    if(uuid_is_null(netdata_random_session_id))
        return false;

    uuid_t uuid;

    if(uuid_parse(guid, uuid))
        return false;

    if(uuid_compare(netdata_random_session_id, uuid) == 0)
        return true;

    return false;
}

static bool check_claim_param(const char *s) {
    if(!s || !*s) return true;

    do {
        if(isalnum(*s) || *s == '.' || *s == ',' || *s == '-' || *s == ':' || *s == '/' || *s == '_')
            ;
        else
            return false;

    } while(*++s);

    return true;
}

void claim_reload_all(void) {
    error_log_limit_unlimited();
    load_claiming_state();
    registry_update_cloud_base_url();
    rrdpush_send_claimed_id(localhost);
    error_log_limit_reset();
}

int api_v2_claim(struct web_client *w, char *url) {
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
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);

    time_t now_s = now_realtime_sec();
    CLOUD_STATUS status = buffer_json_cloud_status(wb, now_s);

    bool can_be_claimed = false;
    switch(status) {
        case CLOUD_STATUS_AVAILABLE:
        case CLOUD_STATUS_DISABLED:
        case CLOUD_STATUS_OFFLINE:
            can_be_claimed = true;
            break;

        case CLOUD_STATUS_UNAVAILABLE:
        case CLOUD_STATUS_BANNED:
        case CLOUD_STATUS_ONLINE:
            can_be_claimed = false;
            break;
    }

    buffer_json_member_add_boolean(wb, "can_be_claimed", can_be_claimed);

    if(can_be_claimed && key) {
        if(!netdata_random_session_id_matches(key)) {
            buffer_reset(wb);
            buffer_strcat(wb, "invalid key");
            netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it
            return HTTP_RESP_FORBIDDEN;
        }

        if(!token || !base_url || !check_claim_param(token) || !check_claim_param(base_url) || (rooms && !check_claim_param(rooms))) {
            buffer_reset(wb);
            buffer_strcat(wb, "invalid parameters");
            netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it
            return HTTP_RESP_BAD_REQUEST;
        }

        netdata_random_session_id_generate(); // generate a new key, to avoid an attack to find it

        netdata_cloud_enabled = CONFIG_BOOLEAN_AUTO;
        appconfig_set_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", CONFIG_BOOLEAN_AUTO);
        appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", base_url);

        uuid_t claimed_id;
        uuid_generate_random(claimed_id);
        char claimed_id_str[UUID_STR_LEN];
        uuid_unparse_lower(claimed_id, claimed_id_str);

        BUFFER *t = buffer_create(1024, NULL);
        if(rooms)
            buffer_sprintf(t, "-id=%s -token=%s -rooms=%s", claimed_id_str, token, rooms);
        else
            buffer_sprintf(t, "-id=%s -token=%s", claimed_id_str, token);

        bool success = false;
        const char *msg = NULL;
        CLAIM_AGENT_RESPONSE rc = claim_agent(buffer_tostring(t), true, &msg);
        switch(rc) {
            case CLAIM_AGENT_OK:
                msg = "ok";
                success = true;
                can_be_claimed = false;
                claim_reload_all();
                {
                    int ms = 0;
                    do {
                        status = cloud_status();
                        if (status == CLOUD_STATUS_ONLINE)
                            break;

                        sleep_usec(100 * USEC_PER_MS);
                        ms += 100;
                    } while (ms < 5000);
                }
                break;

            case CLAIM_AGENT_NO_CLOUD_URL:
                msg = "No Netdata Cloud URL.";
                break;

            case CLAIM_AGENT_CLAIM_SCRIPT_FAILED:
                msg = "Claiming script failed.";
                break;

            case CLAIM_AGENT_CLOUD_DISABLED:
                msg = "Netdata Cloud is disabled on this agent.";
                break;

            case CLAIM_AGENT_CANNOT_EXECUTE_CLAIM_SCRIPT:
                msg = "Failed to execute claiming script.";
                break;

            case CLAIM_AGENT_CLAIM_SCRIPT_RETURNED_INVALID_CODE:
                msg = "Claiming script returned invalid code.";
                break;

            default:
            case CLAIM_AGENT_FAILED_WITH_MESSAGE:
                if(!msg)
                    msg = "Unknown error";
                break;
        }

        // our status may have changed
        // refresh the status in our output
        buffer_flush(wb);
        buffer_json_initialize(wb, "\"", "\"", 0, true, false);
        now_s = now_realtime_sec();
        buffer_json_cloud_status(wb, now_s);

        // and this is the status of the claiming command we run
        buffer_json_member_add_boolean(wb, "success", success);
        buffer_json_member_add_string(wb, "message", msg);
    }

    if(can_be_claimed)
        buffer_json_member_add_string(wb, "key_filename", netdata_random_session_id_get_filename());

    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}
