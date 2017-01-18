#ifndef NETDATA_MAIN_H
#define NETDATA_MAIN_H 1

 /**
 * @file main.h
 * @brief This file holds the API of global netdata elements.
 */

/**
 * This struct contains information about command line options.
 */
struct option_def {
    /** The option character */
    const char val;
    /** The name of the long option. */
    const char *description;
    /** Short descripton what the option does */
    /** Name of the argument displayed in SYNOPSIS */
    const char *arg_name;
    /** Default value if not set */
    const char *default_value;
};

/**
 * List of command line options.
 * This can be used to compute manpage, help messages, ect.
 */
extern struct option_def options[];

/**
 * A static thread of netdata.
 */
struct netdata_static_thread {
    /**
     * Name of the thrad.
     *
     * i.e. used for logging
     */
    char *name;

    char *config_section; ///< config section to enable thread
    char *config_name;    ///< config name to enable thread

    volatile int enabled; ///< boolean

    pthread_t *thread; ///< the thread itself

    void (*init_routine) (void);     ///< function called before thread starts
    void *(*start_routine) (void *); ///< function the thread executes
};

/**
 * Kill all child processes.
 */
extern void kill_childs(void);
/**
 * Send `signal` to process `pid`.
 *
 * Send the signal `signal` to process with pid `pid`.
 * Signals are specified in signal.h.
 * 
 * @param signal 
 * @param pid
 * @return 0 on success.
 */
extern int killpid(pid_t pid, int signal);
/**
 * Stop netdata.
 *
 * Write database to disk. Signal running threads and processes to stop.
 *
 * @param ret Return code to exit with.
 */
extern void netdata_cleanup_and_exit(int ret) NORETURN;

#endif /* NETDATA_MAIN_H */
