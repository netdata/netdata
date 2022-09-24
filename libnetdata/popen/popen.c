// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// popen with tracking

static pthread_mutex_t netdata_popen_tracking_mutex;
static bool netdata_popen_tracking_enabled = false;

struct netdata_popen {
    pid_t pid;
    struct netdata_popen *next;
    struct netdata_popen *prev;
};

static struct netdata_popen *netdata_popen_root = NULL;

// myp_add_lock takes the lock if we're tracking.
static void netdata_popen_tracking_lock(void) {
    if(!netdata_popen_tracking_enabled)
        return;

    netdata_mutex_lock(&netdata_popen_tracking_mutex);
}

// myp_add_unlock release the lock if we're tracking.
static void netdata_popen_tracking_unlock(void) {
    if(!netdata_popen_tracking_enabled)
        return;

    netdata_mutex_unlock(&netdata_popen_tracking_mutex);
}

// myp_add_locked adds pid if we're tracking.
// myp_add_lock must have been called previously.
static void netdata_popen_tracking_add_pid_unsafe(pid_t pid) {
    if(!netdata_popen_tracking_enabled)
        return;

    struct netdata_popen *mp;

    mp = mallocz(sizeof(struct netdata_popen));
    mp->pid = pid;

    DOUBLE_LINKED_LIST_PREPEND_UNSAFE(netdata_popen_root, mp, prev, next);
}

// myp_del deletes pid if we're tracking.
static void netdata_popen_tracking_del_pid(pid_t pid) {
    if(!netdata_popen_tracking_enabled)
        return;

    struct netdata_popen *mp;

    netdata_mutex_lock(&netdata_popen_tracking_mutex);

    DOUBLE_LINKED_LIST_FOREACH_FORWARD(netdata_popen_root, mp, prev, next) {
        if(unlikely(mp->pid == pid))
            break;
    }

    if(mp) {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(netdata_popen_root, mp, prev, next);
        freez(mp);
    }
    else
        error("Cannot find pid %d.", pid);

    netdata_mutex_unlock(&netdata_popen_tracking_mutex);
}

// netdata_popen_tracking_init() should be called by apps which act as init
// (pid 1) so that processes created by mypopen and mypopene
// are tracked. This enables the reaper to ignore processes
// which will be handled internally, by calling myp_reap, to
// avoid issues with already reaped processes during wait calls.
//
// Callers should call myp_free() to clean up resources.
void netdata_popen_tracking_init(void) {
    info("process tracking enabled.");
    netdata_popen_tracking_enabled = true;

    if (netdata_mutex_init(&netdata_popen_tracking_mutex) != 0)
        fatal("netdata_popen_tracking_init() mutex init failed.");
}

// myp_free cleans up any resources allocated for process
// tracking.
void netdata_popen_tracking_cleanup(void) {
    if(!netdata_popen_tracking_enabled)
        return;

    netdata_mutex_lock(&netdata_popen_tracking_mutex);
    netdata_popen_tracking_enabled = false;

    while(netdata_popen_root) {
        struct netdata_popen *mp = netdata_popen_root;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(netdata_popen_root, mp, prev, next);
        freez(mp);
    }

    netdata_mutex_unlock(&netdata_popen_tracking_mutex);
}

// myp_reap returns 1 if pid should be reaped, 0 otherwise.
int netdata_popen_tracking_pid_shoud_be_reaped(pid_t pid) {
    if(!netdata_popen_tracking_enabled)
        return 0;

    netdata_mutex_lock(&netdata_popen_tracking_mutex);

    int ret = 1;
    struct netdata_popen *mp;
    DOUBLE_LINKED_LIST_FOREACH_FORWARD(netdata_popen_root, mp, prev, next) {
        if(unlikely(mp->pid == pid)) {
            ret = 0;
            break;
        }
    }

    netdata_mutex_unlock(&netdata_popen_tracking_mutex);
    return ret;
}

// ----------------------------------------------------------------------------
// helpers

static inline void convert_argv_to_string(char *dst, size_t size, const char *spawn_argv[]) {
    int i;
    for(i = 0; spawn_argv[i] ;i++) {
        if(i == 0) snprintfz(dst, size, "%s", spawn_argv[i]);
        else {
            size_t len = strlen(dst);
            snprintfz(&dst[len], size - len, " '%s'", spawn_argv[i]);
        }
    }
}

// ----------------------------------------------------------------------------
// the core of netdata popen

/*
 * Returns -1 on failure, 0 on success. When POPEN_FLAG_CREATE_PIPE is set, on success set the FILE *fp pointer.
 */
#define PIPE_READ 0
#define PIPE_WRITE 1

static int popene_internal(volatile pid_t *pidptr, char **env, uint8_t flags, FILE **fpp_child_input, FILE **fpp_child_output, const char *command, const char *spawn_argv[]) {
    // create a string to be logged about the command we are running
    char command_to_be_logged[2048];
    convert_argv_to_string(command_to_be_logged, sizeof(command_to_be_logged), spawn_argv);
    // info("custom_popene() running command: %s", command_to_be_logged);

    FILE *fp_child_input = NULL, *fp_child_output = NULL;
    int ret = 0; // success by default
    int error;
    int pipefd_stdin[2] = { -1, -1 };
    int pipefd_stdout[2] = { -1, -1 };
    pid_t pid;
    posix_spawnattr_t attr;
    posix_spawn_file_actions_t fa;

    if (flags & POPEN_FLAG_CREATE_PIPE) {
        if (pipe(pipefd_stdin) == -1) {
            error("POPEN: stdin pipe() failed");
            goto cleanup_pipes;
        }

        if (pipe(pipefd_stdout) == -1) {
            error("POPEN: stdout pipe() failed");
            goto cleanup_pipes;
        }

        if ((fp_child_input = fdopen(pipefd_stdin[PIPE_WRITE], "w")) == NULL) {
            error("POPEN: fdopen() stdin failed");
            goto cleanup_pipes;
        }

        if ((fp_child_output = fdopen(pipefd_stdout[PIPE_READ], "r")) == NULL) {
            error("POPEN: fdopen() stdout failed");
            goto cleanup_pipes;
        }
    }

    if(flags & POPEN_FLAG_CLOSE_FD) {
        // Mark all files to be closed by the exec() stage of posix_spawn()
        for(int i = (int)(sysconf(_SC_OPEN_MAX) - 1); i >= 0; i--) {
            if(i != STDERR_FILENO)
                (void)fcntl(i, F_SETFD, FD_CLOEXEC);
        }
    }

    if(posix_spawn_file_actions_init(&fa)) {
        error("posix_spawn_file_actions_init() failed.");
        goto cleanup_pipes;
    }

    if(flags & POPEN_FLAG_CREATE_PIPE) {
        // move the pipe to stdout in the child
        if(posix_spawn_file_actions_adddup2(&fa, pipefd_stdin[PIPE_READ], STDIN_FILENO)) {
            error("posix_spawn_file_actions_adddup2() on stdin failed");
            goto error_after_posix_spawn_file_actions_init;
        }

        if(posix_spawn_file_actions_adddup2(&fa, pipefd_stdout[PIPE_WRITE], STDOUT_FILENO)) {
            error("posix_spawn_file_actions_adddup2() on stdout failed");
            goto error_after_posix_spawn_file_actions_init;
        }
    }
    else {
        // set stdin to /dev/null
        if (posix_spawn_file_actions_addopen(&fa, STDIN_FILENO, "/dev/null", O_RDONLY, 0)) {
            error("posix_spawn_file_actions_addopen() on stdin failed");
            // this is not a fatal error
        }

        // set stdout to /dev/null
        if (posix_spawn_file_actions_addopen(&fa, STDOUT_FILENO, "/dev/null", O_WRONLY, 0)) {
            error("posix_spawn_file_actions_addopen() on stdout failed");
            // this is not a fatal error
        }
    }

    if(!(error = posix_spawnattr_init(&attr))) {
        // reset all signals in the child
        sigset_t mask;

        if (posix_spawnattr_setflags(&attr, POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF))
            error("posix_spawnattr_setflags() failed.");

        sigemptyset(&mask);

        if (posix_spawnattr_setsigmask(&attr, &mask))
            error("posix_spawnattr_setsigmask() failed.");
    }
    else
        error("posix_spawnattr_init() failed.");

    // Take the lock while we fork to ensure we don't race with SIGCHLD
    // delivery on a process which exits quickly.
    netdata_popen_tracking_lock();
    if (!posix_spawn(&pid, command, &fa, &attr, (char * const*)spawn_argv, env)) {
        *pidptr = pid;
        netdata_popen_tracking_add_pid_unsafe(pid);
        netdata_popen_tracking_unlock();
        debug(D_CHILDS, "Spawned command: \"%s\" on pid %d from parent pid %d.", command_to_be_logged, pid, getpid());
    }
    else {
        netdata_popen_tracking_unlock();
        error("Failed to spawn command: \"%s\" from parent pid %d.", command_to_be_logged, getpid());
        if(flags & POPEN_FLAG_CREATE_PIPE) {
            fclose(fp_child_output);
            fp_child_output = NULL;
        }
        ret = -1;
    }

    if(flags & POPEN_FLAG_CREATE_PIPE) {
        close(pipefd_stdin[PIPE_READ]);
        close(pipefd_stdout[PIPE_WRITE]);

        if (0 == ret) {
            // on success set FILE * pointer
            if (fpp_child_input) *fpp_child_input = fp_child_input;
            if (fpp_child_output) *fpp_child_output = fp_child_output;
        }
    }

    if(!error) {
        // posix_spawnattr_init() succeeded
        if (posix_spawnattr_destroy(&attr))
            error("posix_spawnattr_destroy() failed");
    }

    if (posix_spawn_file_actions_destroy(&fa))
        error("posix_spawn_file_actions_destroy() failed");

    return ret;

error_after_posix_spawn_file_actions_init:
    if (posix_spawn_file_actions_destroy(&fa))
        error("posix_spawn_file_actions_destroy");

cleanup_pipes:
    if (flags & POPEN_FLAG_CREATE_PIPE) {
        if (fp_child_output)
            fclose(fp_child_output);
        else if(pipefd_stdout[PIPE_READ] != -1)
            close(pipefd_stdout[PIPE_READ]);

        if(pipefd_stdout[PIPE_WRITE] != -1)
            close(pipefd_stdout[PIPE_WRITE]);

        if(fp_child_input)
            fclose(fp_child_input);
        else if(pipefd_stdin[PIPE_WRITE] != -1)
            close(pipefd_stdin[PIPE_WRITE]);

        if(pipefd_stdin[PIPE_READ] != -1)
            close(pipefd_stdin[PIPE_READ]);
    }
    return -1;
}

int netdata_popene_variadic_internal_dont_use_directly(volatile pid_t *pidptr, char **env, uint8_t flags, FILE **fpp_child_input, FILE **fpp_child_output, const char *command, ...) {
    // convert the variable list arguments into what posix_spawn() needs
    // all arguments are expected strings
    va_list args;
    int args_count;

    // count the number variable parameters
    // the variable parameters are expected NULL terminated
    {
        const char *s;

        va_start(args, command);
        args_count = 0;
        while ((s = va_arg(args, const char *))) args_count++;
        va_end(args);
    }

    // create a string pointer array as needed by posix_spawn()
    // variable array in the stack
    const char *spawn_argv[args_count + 1];
    {
        const char *s;
        va_start(args, command);
        int i;
        for (i = 0; i < args_count; i++) {
            s = va_arg(args, const char *);
            spawn_argv[i] = s;
        }
        spawn_argv[args_count] = NULL;
        va_end(args);
    }

    return popene_internal(pidptr, env, flags, fpp_child_input, fpp_child_output, command, spawn_argv);
}

// See man environ
extern char **environ;

FILE *netdata_popen(const char *command, volatile pid_t *pidptr, FILE **fpp_child_input) {
    FILE *fp_child_output = NULL;
    const char *spawn_argv[] = {
        "sh",
        "-c",
        command,
        NULL
    };
    (void)popene_internal(pidptr, environ, POPEN_FLAG_CREATE_PIPE | POPEN_FLAG_CLOSE_FD, fpp_child_input, &fp_child_output, "/bin/sh", spawn_argv);
    return fp_child_output;
}

FILE *netdata_popene(const char *command, volatile pid_t *pidptr, char **env, FILE **fpp_child_input) {
    FILE *fp_child_output = NULL;
    const char *spawn_argv[] = {
        "sh",
        "-c",
        command,
        NULL
    };
    (void)popene_internal(pidptr, env, POPEN_FLAG_CREATE_PIPE | POPEN_FLAG_CLOSE_FD, fpp_child_input, &fp_child_output, "/bin/sh", spawn_argv);
    return fp_child_output;
}

// returns 0 on success, -1 on failure
int netdata_spawn(const char *command, volatile pid_t *pidptr) {
    const char *spawn_argv[] = {
        "sh",
        "-c",
        command,
        NULL
    };
    return popene_internal(pidptr, environ, POPEN_FLAG_NONE, NULL, NULL, "/bin/sh", spawn_argv);
}

int netdata_pclose(FILE *fp_child_input, FILE *fp_child_output, pid_t pid) {
    int ret;
    siginfo_t info;

    debug(D_EXIT, "Request to netdata_pclose() on pid %d", pid);

    if (fp_child_input) {
        // close the pipe fd
        // this is required in musl
        // without it the childs do not exit
        close(fileno(fp_child_input));

        // close the pipe file pointer
        fclose(fp_child_input);
    }

    if (fp_child_output) {
        // close the pipe fd
        // this is required in musl
        // without it the childs do not exit
        close(fileno(fp_child_output));

        // close the pipe file pointer
        fclose(fp_child_output);
    }

    errno = 0;

    ret = waitid(P_PID, (id_t) pid, &info, WEXITED);
    netdata_popen_tracking_del_pid(pid);

    if (ret != -1) {
        switch (info.si_code) {
            case CLD_EXITED:
                if(info.si_status)
                    error("child pid %d exited with code %d.", info.si_pid, info.si_status);
                return(info.si_status);

            case CLD_KILLED:
                if(info.si_status == 15) {
                    info("child pid %d killed by signal %d.", info.si_pid, info.si_status);
                    return(0);
                }
                else {
                    error("child pid %d killed by signal %d.", info.si_pid, info.si_status);
                    return(-1);
                }

            case CLD_DUMPED:
                error("child pid %d core dumped by signal %d.", info.si_pid, info.si_status);
                return(-2);

            case CLD_STOPPED:
                error("child pid %d stopped by signal %d.", info.si_pid, info.si_status);
                return(0);

            case CLD_TRAPPED:
                error("child pid %d trapped by signal %d.", info.si_pid, info.si_status);
                return(-4);

            case CLD_CONTINUED:
                error("child pid %d continued by signal %d.", info.si_pid, info.si_status);
                return(0);

            default:
                error("child pid %d gave us a SIGCHLD with code %d and status %d.", info.si_pid, info.si_code, info.si_status);
                return(-5);
        }
    }
    else
        error("Cannot waitid() for pid %d", pid);
    
    return 0;
}

int netdata_spawn_waitpid(pid_t pid) {
    return netdata_pclose(NULL, NULL, pid);
}
