#ifndef NETDATA_MAIN_H
#define NETDATA_MAIN_H 1

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

struct netdata_static_thread {
    char *name;

    char *config_section;
    char *config_name;

    volatile sig_atomic_t enabled;

    netdata_thread_t *thread;

    void (*init_routine) (void);
    void *(*start_routine) (void *);
};

extern void cancel_main_threads(void);
extern int killpid(pid_t pid, int signal);
extern void netdata_cleanup_and_exit(int ret) NORETURN;

#endif /* NETDATA_MAIN_H */
