// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

/*
struct mypopen {
    pid_t pid;
    FILE *fp;
    struct mypopen *next;
    struct mypopen *prev;
};

static struct mypopen *mypopen_root = NULL;

static void mypopen_add(FILE *fp, pid_t *pid) {
    struct mypopen *mp = malloc(sizeof(struct mypopen));
    if(!mp) {
        fatal("Cannot allocate %zu bytes", sizeof(struct mypopen))
        return;
    }

    mp->fp = fp;
    mp->pid = pid;
    mp->next = popen_root;
    mp->prev = NULL;
    if(mypopen_root) mypopen_root->prev = mp;
    mypopen_root = mp;
}

static void mypopen_del(FILE *fp) {
    struct mypopen *mp;

    for(mp = mypopen_root; mp; mp = mp->next)
        if(mp->fd == fp) break;

    if(!mp) error("Cannot find mypopen() file pointer in open childs.");
    else {
        if(mp->next) mp->next->prev = mp->prev;
        if(mp->prev) mp->prev->next = mp->next;
        if(mypopen_root == mp) mypopen_root = mp->next;
        free(mp);
    }
}
*/
#define PIPE_READ 0
#define PIPE_WRITE 1

static inline FILE *custom_popene(const char *command, volatile pid_t *pidptr, char **env) {
    FILE *fp;
    int pipefd[2], error;
    pid_t pid;
    char *const spawn_argv[] = {
            "sh",
            "-c",
            (char *)command,
            NULL
    };
    posix_spawnattr_t attr;
    posix_spawn_file_actions_t fa;

    if(pipe(pipefd) == -1)
        return NULL;
    if ((fp = fdopen(pipefd[PIPE_READ], "r")) == NULL) {
        goto error_after_pipe;
    }

    // Mark all files to be closed by the exec() stage of posix_spawn()
    int i;
    for(i = (int) (sysconf(_SC_OPEN_MAX) - 1); i >= 0; i--)
        if(i != STDIN_FILENO && i != STDERR_FILENO)
            (void)fcntl(i, F_SETFD, FD_CLOEXEC);

    if (!posix_spawn_file_actions_init(&fa)) {
        // move the pipe to stdout in the child
        if (posix_spawn_file_actions_adddup2(&fa, pipefd[PIPE_WRITE], STDOUT_FILENO)) {
            error("posix_spawn_file_actions_adddup2() failed");
            goto error_after_posix_spawn_file_actions_init;
        }
    } else {
        error("posix_spawn_file_actions_init() failed.");
        goto error_after_pipe;
    }
    if (!(error = posix_spawnattr_init(&attr))) {
        // reset all signals in the child
        sigset_t mask;

        if (posix_spawnattr_setflags(&attr, POSIX_SPAWN_SETSIGMASK | POSIX_SPAWN_SETSIGDEF))
            error("posix_spawnattr_setflags() failed.");
        sigemptyset(&mask);
        if (posix_spawnattr_setsigmask(&attr, &mask))
            error("posix_spawnattr_setsigmask() failed.");
    } else {
        error("posix_spawnattr_init() failed.");
    }
    if (!posix_spawn(&pid, "/bin/sh", &fa, &attr, spawn_argv, env)) {
        *pidptr = pid;
        debug(D_CHILDS, "Spawned command: '%s' on pid %d from parent pid %d.", command, pid, getpid());
    } else {
        error("Failed to spawn command: '%s' from parent pid %d.", command, getpid());
        fclose(fp);
        fp = NULL;
    }
    close(pipefd[PIPE_WRITE]);

    if (!error) {
        // posix_spawnattr_init() succeeded
        if (posix_spawnattr_destroy(&attr))
            error("posix_spawnattr_destroy");
    }
    if (posix_spawn_file_actions_destroy(&fa))
        error("posix_spawn_file_actions_destroy");

    return fp;

error_after_posix_spawn_file_actions_init:
    if (posix_spawn_file_actions_destroy(&fa))
        error("posix_spawn_file_actions_destroy");
error_after_pipe:
    if (fp)
        fclose(fp);
    else
        close(pipefd[PIPE_READ]);

    close(pipefd[PIPE_WRITE]);
    return NULL;
}

// See man environ
extern char **environ;

FILE *mypopen(const char *command, volatile pid_t *pidptr) {
    return custom_popene(command, pidptr, environ);
}

FILE *mypopene(const char *command, volatile pid_t *pidptr, char **env) {
    return custom_popene(command, pidptr, env);
}

int mypclose(FILE *fp, pid_t pid) {
    debug(D_EXIT, "Request to mypclose() on pid %d", pid);

    /*mypopen_del(fp);*/

    // close the pipe fd
    // this is required in musl
    // without it the childs do not exit
    close(fileno(fp));

    // close the pipe file pointer
    fclose(fp);

    errno = 0;

    siginfo_t info;
    if(waitid(P_PID, (id_t) pid, &info, WEXITED) != -1) {
        switch(info.si_code) {
            case CLD_EXITED:
                if(info.si_status)
                    error("child pid %d exited with code %d.", info.si_pid, info.si_status);
                return(info.si_status);

            case CLD_KILLED:
                error("child pid %d killed by signal %d.", info.si_pid, info.si_status);
                return(-1);

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
