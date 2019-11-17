// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMANDS_H
#define NETDATA_COMMANDS_H 1

#ifdef _WIN32
# define PIPENAME "\\\\?\\pipe\\netdata-cli"
#else
# define PIPENAME "/tmp/netdata-ipc"
#endif

#define MAX_COMMAND_LENGTH 4096

typedef enum cmd {
    CMD_HELP = 0,
    CMD_RELOAD_HEALTH,
    CMD_SAVE_DATABASE,
    CMD_LOG_ROTATE,
    CMD_EXIT,
    CMD_FATAL,
    CMD_TOTAL_COMMANDS
} cmd_t;

typedef enum cmd_status {
    CMD_STATUS_SUCCESS = 0,
    CMD_STATUS_FAILURE,
    CMD_STATUS_BUSY
} cmd_status_t;

#define CMD_STATUS_SUCCESS_STR "SUCCESS"
#define CMD_STATUS_FAILURE_STR "FAILURE"
#define CMD_STATUS_BUSY_STR "BUSY"

typedef enum cmd_type {
    CMD_TYPE_EXCLUSIVE = 0, // No other command can run at the same time
    CMD_TYPE_ORTHOGONAL,    // Other commands are allowed to run concurrently but calls to this command are serialized
    CMD_TYPE_IDEMPOTENT,    // Any call to any command is allowed at the same time as its execution
    CMD_TYPE_HIGH_PRIORITY  // It can be called even when there is an exclusive command running
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
extern int commands_init(void);
extern int commands_exit(void);

#endif //NETDATA_COMMANDS_H
