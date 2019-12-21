// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"
#include "../registry/registry_internals.h"

char *claiming_pending_arguments = NULL;

static char *claiming_errors[] = {
        "Agent claimed successfully",                   // 0
        "Unknown argument",                             // 1
        "Problems with claiming working directory",     // 2
        "Missing dependencies",                         // 3
        "Failure to connect to endpoint",               // 4
        "Unknown HTTP error message",                   // 5
        "invalid agent id",                             // 6
        "invalid public key",                           // 7
        "token has expired",                            // 8
        "invalid token",                                // 9
        "duplicate agent id",                           // 10
        "claimed in another workspace",                 // 11
        "internal server error"                         // 12
};

#define AGENT_UNCLAIMED 0
#define AGENT_CLAIMED 1
static uint8_t claiming_status = AGENT_UNCLAIMED;

uint8_t is_agent_claimed(void)
{
    return (AGENT_CLAIMED == claiming_status);
}

#define CLAIMING_COMMAND_LENGTH 16384

extern struct registry registry;

/* rrd_init() must have been called before this function */
void claim_agent(char *claiming_arguments)
{
    info("The claiming feature is under development and still subject to change before the next release");
    return;

    int exit_code;
    pid_t command_pid;
    char command_buffer[CLAIMING_COMMAND_LENGTH + 1];
    FILE *fp;

    snprintfz(command_buffer,
              CLAIMING_COMMAND_LENGTH,
              "exec netdata-claim.sh -hostname=%s -id=%s -url=%s %s",
              netdata_configured_hostname,
              localhost->machine_guid,
              registry.cloud_base_url,
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
        claiming_status = AGENT_CLAIMED;
        info("Agent successfully claimed.");
        return;
    }
    if (exit_code < 0) {
        error("Agent claiming command failed to complete its run.");
        return;
    }
    errno = 0;
    unsigned maximum_known_exit_code = sizeof(claiming_errors) / sizeof(claiming_errors[0]);

    if ((unsigned)exit_code > maximum_known_exit_code) {
        error("Agent failed to be claimed with an unknown error.");
        return;
    }
    error("Agent failed to be claimed with the following error message:");
    error("\"%s\"", claiming_errors[exit_code]);
}

void load_claiming_state(void)
{
    info("The claiming feature is under development and still subject to change before the next release");
    return;

    char filename[FILENAME_MAX + 1];
    struct stat statbuf;

    snprintfz(filename, FILENAME_MAX, "%s/claim.d/is_claimed", netdata_configured_user_config_dir);
    // check if the file exists
    if (lstat(filename, &statbuf) != 0) {
        info("File '%s' was not found. Setting state to AGENT_UNCLAIMED.", filename);
        claiming_status = AGENT_UNCLAIMED;
   } else {
        info("File '%s' was found. Setting state to AGENT_CLAIMED.", filename);
        claiming_status = AGENT_CLAIMED;
    }
}
