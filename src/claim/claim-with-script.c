// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"
#include "aclk/aclk_proxy.h"

#ifdef CLAIM_WITH_SCRIPT

#define CLAIMING_COMMAND_LENGTH 16384
#define CLAIMING_PROXY_LENGTH (CLAIMING_COMMAND_LENGTH/4)

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

/* rrd_init() and post_conf_load() must have been called before this function */
static CLAIM_AGENT_RESPONSE claim_call_script(const char *claiming_arguments, bool force, const char **msg __maybe_unused)
{
    if (!force || !netdata_cloud_enabled) {
        netdata_log_error("Refusing to claim agent -> cloud functionality has been disabled");
        return CLAIM_AGENT_CLOUD_DISABLED;
    }

#ifndef DISABLE_CLOUD
    char command_exec_buffer[CLAIMING_COMMAND_LENGTH + 1];
    char command_line_buffer[CLAIMING_COMMAND_LENGTH + 1];

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

    snprintfz(command_exec_buffer, CLAIMING_COMMAND_LENGTH,
              "exec \"%s%snetdata-claim.sh\"",
              netdata_exe_path ? netdata_exe_path : "",
              netdata_exe_path ? "/" : ""
              );

    snprintfz(command_line_buffer,
              CLAIMING_COMMAND_LENGTH,
              "%s %s -hostname=%s -id=%s -url=%s -noreload %s",
              command_exec_buffer,
              proxy_flag,
              netdata_configured_hostname,
              localhost->machine_guid,
              cloud_base_url,
              claiming_arguments);

    netdata_log_info("Executing agent claiming command: %s", command_exec_buffer);
    POPEN_INSTANCE *instance = spawn_popen_run(command_line_buffer);
    if(!instance) {
        netdata_log_error("Cannot popen(\"%s\").", command_exec_buffer);
        return CLAIM_AGENT_CANNOT_EXECUTE_CLAIM_SCRIPT;
    }

    netdata_log_info("Waiting for claiming command '%s' to finish.", command_exec_buffer);
    char read_buffer[100 + 1];
    while (fgets(read_buffer, 100, instance->child_stdout_fp) != NULL) ;

    int exit_code = spawn_popen_wait(instance);

    netdata_log_info("Agent claiming command '%s' returned with code %d", command_exec_buffer, exit_code);
    if (0 == exit_code) {
        load_claiming_state();
        return CLAIM_AGENT_OK;
    }
    if (exit_code < 0) {
        netdata_log_error("Agent claiming command '%s' failed to complete its run", command_exec_buffer);
        return CLAIM_AGENT_CLAIM_SCRIPT_FAILED;
    }
    errno_clear();
    unsigned maximum_known_exit_code = sizeof(claiming_errors) / sizeof(claiming_errors[0]) - 1;

    if ((unsigned)exit_code > maximum_known_exit_code) {
        netdata_log_error("Agent failed to be claimed with an unknown error. Cmd: '%s'", command_exec_buffer);
        return CLAIM_AGENT_CLAIM_SCRIPT_RETURNED_INVALID_CODE;
    }

    netdata_log_error("Agent failed to be claimed using the command '%s' with the following error message: %s",
                      command_exec_buffer, claiming_errors[exit_code]);

    if(msg) *msg = claiming_errors[exit_code];

#else
    UNUSED(claiming_arguments);
    UNUSED(claiming_errors);
#endif

    return CLAIM_AGENT_FAILED_WITH_MESSAGE;
}

CLAIM_AGENT_RESPONSE claim_agent(const char *id, const char *token, const char *rooms, const char **error) {
    CLEAN_BUFFER *t = buffer_create(1024, NULL);
    if(rooms)
        buffer_sprintf(t, "-id=%s -token=%s -rooms=%s", id, token, rooms);
    else
        buffer_sprintf(t, "-id=%s -token=%s", id, token);

    return claim_call_script(buffer_tostring(t), true, error);
}

#endif
