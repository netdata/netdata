#ifndef NETDATA_POPEN_H
#define NETDATA_POPEN_H 1

/**
 * @file popen.h
 * @brief API to run an external command
 */

#define PIPE_READ 0  ///< Read end file descriptor index in pipe file descripter array.
#define PIPE_WRITE 1 ///< Write end file descriptor index in pipe file descripter array.

/**
 * Execute `command` and return a file descriptor to the output.
 *
 * @param command Command line to execute.
 * @param pidptr Pointer to store the pid of the executed program.
 * @return FILE to read output from 
 */
extern FILE *mypopen(const char *command, pid_t *pidptr);
/**
 * Exit command started with myopen()
 *
 * This returns
 * - The exit code of command if present
 * - -1 if command was killed
 * - -2 if command was core dumped
 * - -4 if comannd was trapped by signal
 * - -5 if child gave us SIGCHILD
 * - 0 otherwise
 *
 * @param fp returned by myopen()
 * @param pid set by myopen()
 * @return exit code described above.
 */
extern int mypclose(FILE *fp, pid_t pid);

#endif /* NETDATA_POPEN_H */
