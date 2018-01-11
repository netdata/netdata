#include "common.h"

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

FILE *mypopen(const char *command, volatile pid_t *pidptr)
{
    int pipefd[2];

    if(pipe(pipefd) == -1) return NULL;

    int pid = fork();
    if(pid == -1) {
        close(pipefd[PIPE_READ]);
        close(pipefd[PIPE_WRITE]);
        return NULL;
    }
    if(pid != 0) {
        // the parent
        *pidptr = pid;
        close(pipefd[PIPE_WRITE]);
        FILE *fp = fdopen(pipefd[PIPE_READ], "r");
        /*mypopen_add(fp, pid);*/
        return(fp);
    }
    // the child

    // close all files
    int i;
    for(i = (int) (sysconf(_SC_OPEN_MAX) - 1); i > 0; i--)
        if(i != STDIN_FILENO && i != STDERR_FILENO && i != pipefd[PIPE_WRITE]) close(i);

    // move the pipe to stdout
    if(pipefd[PIPE_WRITE] != STDOUT_FILENO) {
        dup2(pipefd[PIPE_WRITE], STDOUT_FILENO);
        close(pipefd[PIPE_WRITE]);
    }

#ifdef DETACH_PLUGINS_FROM_NETDATA
    // this was an attempt to detach the child and use the suspend mode charts.d
    // unfortunatelly it does not work as expected.

    // fork again to become session leader
    pid = fork();
    if(pid == -1)
        error("pre-execution of command '%s' on pid %d: Cannot fork 2nd time.", command, getpid());

    if(pid != 0) {
        // the parent
        exit(0);
    }

    // set a new process group id for just this child
    if( setpgid(0, 0) != 0 )
        error("pre-execution of command '%s' on pid %d: Cannot set a new process group.", command, getpid());

    if( getpgid(0) != getpid() )
        error("pre-execution of command '%s' on pid %d: Cannot set a new process group. Process group set is incorrect. Expected %d, found %d", command, getpid(), getpid(), getpgid(0));

    if( setsid() != 0 )
        error("pre-execution of command '%s' on pid %d: Cannot set session id.", command, getpid());

    fprintf(stdout, "MYPID %d\n", getpid());
    fflush(NULL);
#endif

    // reset all signals
    signals_unblock();
    signals_reset();

    debug(D_CHILDS, "executing command: '%s' on pid %d.", command, getpid());
    execl("/bin/sh", "sh", "-c", command, NULL);
    exit(1);
}

FILE *mypopene(const char *command, volatile pid_t *pidptr, char **env) {
    int pipefd[2];

    if(pipe(pipefd) == -1)
        return NULL;

    int pid = fork();
    if(pid == -1) {
        close(pipefd[PIPE_READ]);
        close(pipefd[PIPE_WRITE]);
        return NULL;
    }
    if(pid != 0) {
        // the parent
        *pidptr = pid;
        close(pipefd[PIPE_WRITE]);
        FILE *fp = fdopen(pipefd[PIPE_READ], "r");
        return(fp);
    }
    // the child

    // close all files
    int i;
    for(i = (int) (sysconf(_SC_OPEN_MAX) - 1); i > 0; i--)
        if(i != STDIN_FILENO && i != STDERR_FILENO && i != pipefd[PIPE_WRITE]) close(i);

    // move the pipe to stdout
    if(pipefd[PIPE_WRITE] != STDOUT_FILENO) {
        dup2(pipefd[PIPE_WRITE], STDOUT_FILENO);
        close(pipefd[PIPE_WRITE]);
    }

    execle("/bin/sh", "sh", "-c", command, NULL, env);
    exit(1);
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
