// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMANDS_H
#define NETDATA_COMMANDS_H 1

#ifdef _WIN32
# define PIPENAME "\\\\?\\pipe\\netdata-cli"
#else
# define PIPENAME "/tmp/netdata-ipc"
#endif

#define MAX_COMMAND_LENGTH 4096
#define MAX_EXIT_STATUS_LENGTH 23 /* Can't ever be bigger than "X-18446744073709551616" */

typedef enum cmd {
    CMD_HELP = 0,
    CMD_RELOAD_HEALTH,
    CMD_SAVE_DATABASE,
    CMD_REOPEN_LOGS,
    CMD_EXIT,
    CMD_FATAL,
    CMD_TOTAL_COMMANDS
} cmd_t;

typedef enum cmd_status {
    CMD_STATUS_SUCCESS = 0,
    CMD_STATUS_FAILURE,
    CMD_STATUS_BUSY
} cmd_status_t;

#define CMD_PREFIX_INFO 'O'         /* Following string should go to cli stdout */
#define CMD_PREFIX_ERROR 'E'        /* Following string should go to cli stderr */
#define CMD_PREFIX_EXIT_CODE 'X'    /* Following string is cli integer exit code */

typedef enum cmd_type {
    /*
     * No other command is allowed to run at the same time (except for CMD_TYPE_HIGH_PRIORITY).
     */
    CMD_TYPE_EXCLUSIVE = 0,
    /*
     * Other commands are allowed to run concurrently (except for CMD_TYPE_EXCLUSIVE) but calls to this command are
     * serialized.
     */
    CMD_TYPE_ORTHOGONAL,
    /*
     * Other commands are allowed to run concurrently (except for CMD_TYPE_EXCLUSIVE) as are calls to this command.
     */
    CMD_TYPE_CONCURRENT,
    /*
     * Those commands are always allowed to run.
     */
    CMD_TYPE_HIGH_PRIORITY
} cmd_type_t;

/**
 * Executes a command and returns the status.
 *
 * @param args a string that may contain additional parameters to be parsed
 * @param message allocate and return a message if need be (up to MAX_COMMAND_LENGTH bytes)
 * @return CMD_FAILURE or CMD_SUCCESS
 */
typedef cmd_status_t (command_action_t) (char *args, char **message);

typedef struct command_info {
    char *cmd_str;              // the command string
    command_action_t *func;     // the function that executes the command
    cmd_type_t type;            // Concurrency control information for the command
} command_info_t;

typedef void (command_lock_t) (unsigned index);

cmd_status_t execute_command(cmd_t idx, char *args, char **message);
extern void commands_init(void);
extern void commands_exit(void);

#endif //NETDATA_COMMANDS_H
