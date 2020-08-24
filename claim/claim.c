// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"
#include "../registry/registry_internals.h"
#include "../aclk/aclk_common.h"

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
char *is_agent_claimed()
{
    char *result;
    netdata_mutex_lock(&localhost->claimed_id_lock);
    result = (localhost->claimed_id == NULL) ? NULL : strdupz(localhost->claimed_id);
    netdata_mutex_unlock(&localhost->claimed_id_lock);
    return result;
}

#define CLAIMING_COMMAND_LENGTH 16384
#define CLAIMING_PROXY_LENGTH CLAIMING_COMMAND_LENGTH/4

extern struct registry registry;

/* rrd_init() and post_conf_load() must have been called before this function */
void claim_agent(char *claiming_arguments)
{
    if (!netdata_cloud_setting) {
        error("Refusing to claim agent -> cloud functionality has been disabled");
        return;
    }

#ifndef DISABLE_CLOUD
    int exit_code;
    pid_t command_pid;
    char command_buffer[CLAIMING_COMMAND_LENGTH + 1];
    FILE *fp;

    // This is guaranteed to be set early in main via post_conf_load()
    char *cloud_base_url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", NULL);
    if (cloud_base_url == NULL)
        fatal("Do not move the cloud base url out of post_conf_load!!");
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

    info("Executing agent claiming command 'netdata-claim.sh'");
    fp = mypopen(command_buffer, &command_pid);
    if(!fp) {
        error("Cannot popen(\"%s\").", command_buffer);
        return;
    }
    info("Waiting for claiming command to finish.");
    while (fgets(command_buffer, CLAIMING_COMMAND_LENGTH, fp) != NULL) {;}
    exit_code = mypclose(fp, command_pid);
    info("Agent claiming command returned with code %d", exit_code);
    if (0 == exit_code) {
        load_claiming_state();
        return;
    }
    if (exit_code < 0) {
        error("Agent claiming command failed to complete its run.");
        return;
    }
    errno = 0;
    unsigned maximum_known_exit_code = sizeof(claiming_errors) / sizeof(claiming_errors[0]) - 1;

    if ((unsigned)exit_code > maximum_known_exit_code) {
        error("Agent failed to be claimed with an unknown error.");
        return;
    }
    error("Agent failed to be claimed with the following error message:");
    error("\"%s\"", claiming_errors[exit_code]);
#else
    UNUSED(claiming_arguments);
    UNUSED(claiming_errors);
#endif
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
    netdata_cloud_setting = 0;
#else
    uuid_t uuid;
    netdata_mutex_lock(&localhost->claimed_id_lock);
    if (localhost->claimed_id) {
        freez(localhost->claimed_id);
        localhost->claimed_id = NULL;
    }
    if (aclk_connected)
    {
        info("Agent was already connected to Cloud - forcing reconnection under new credentials");
        aclk_kill_link = 1;
    }
    aclk_disable_runtime = 0;

    // Propagate into aclk and registry. Be kind of atomic...
    appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", DEFAULT_CLOUD_BASE_URL);

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/cloud.d/claimed_id", netdata_configured_varlib_dir);

    long bytes_read;
    char *claimed_id = read_by_filename(filename, &bytes_read);
    if(claimed_id && uuid_parse(claimed_id, uuid)) {
        error("claimed_id \"%s\" doesn't look like valid UUID", claimed_id);
        freez(claimed_id);
        claimed_id = NULL;
    }
    localhost->claimed_id = claimed_id;
    netdata_mutex_unlock(&localhost->claimed_id_lock);
    if (!claimed_id) {
        info("Unable to load '%s', setting state to AGENT_UNCLAIMED", filename);
        return;
    }

    info("File '%s' was found. Setting state to AGENT_CLAIMED.", filename);
    netdata_cloud_setting = appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", 1);
#endif
}

struct config cloud_config = { .first_section = NULL,
                               .last_section = NULL,
                               .mutex = NETDATA_MUTEX_INITIALIZER,
                               .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                          .rwlock = AVL_LOCK_INITIALIZER } };

void load_cloud_conf(int silent)
{
    char *filename;
    errno = 0;

    int ret = 0;

    filename = strdupz_path_subpath(netdata_configured_varlib_dir, "cloud.d/cloud.conf");

    ret = appconfig_load(&cloud_config, filename, 1, NULL);
    if(!ret && !silent) {
        info("CONFIG: cannot load cloud config '%s'. Running with internal defaults.", filename);
    }
    freez(filename);
}
