// SPDX-License-Identifier: GPL-3.0-or-later
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#if defined(TREE_MISSING_CHILDREN) || defined(TREE_BAD_NAMESPACE)
FILE *__real_fopen(const char *, const char *);
FILE *__wrap_fopen(const char *path, const char *mode) {
#ifdef TREE_MISSING_CHILDREN
    if (!strcmp(path, "/proc/thread-self/children")) {
        errno = ENOENT;
        return NULL;
    }
#else
    if (!strcmp(path, "/proc/thread-self/status")) {
        static char status[] = "NSpid:\t1\t2\n";
        return fmemopen(status, sizeof(status) - 1, "r");
    }
#endif
    return __real_fopen(path, mode);
}
#ifdef TREE_WAIT_NOT_EMPTY
pid_t __wrap_waitpid(pid_t pid, int *status, int options) {
    (void)pid;
    (void)status;
    (void)options;
    return 0;
}
#endif
#elif defined(TREE_NNP_SET_FAIL) || defined(TREE_NNP_GET_FAIL)
#include <sys/prctl.h>
int __real_prctl(int option, ...);
int __wrap_prctl(int option, ...) {
#ifdef TREE_NNP_SET_FAIL
    if (option == PR_SET_NO_NEW_PRIVS) {
        errno = EPERM;
        return -1;
    }
#else
    if (option == PR_GET_NO_NEW_PRIVS)
        return 0;
#endif
    return __real_prctl(option, 1UL, 0UL, 0UL, 0UL);
}
#elif defined(TREE_PRIVILEGE_PROBE)
#include <sys/prctl.h>
int main(void) {
    int gained = setresuid(0, 0, 0);
    uid_t real, effective, saved;
    if (getresuid(&real, &effective, &saved) != 0)
        return 90;
    printf("%d %ld %ld %ld %d\n", gained, (long)real, (long)effective, (long)saved,
           prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0));
    return 0;
}
#elif defined(TREE_SETUP_FAIL)
int __wrap_prctl(int option, ...) {
    (void)option;
    errno = EPERM;
    return -1;
}
#elif defined(TREE_FORK_FAIL)
pid_t __wrap_fork(void) {
    errno = EAGAIN;
    return -1;
}
#else
#include <sched.h>
#include <sys/wait.h>

static int ready_fd;

static int descendant(void *unused) {
    (void)unused;
    if (setsid() < 0)
        _exit(91);
    signal(SIGTERM, SIG_IGN);
    signal(SIGINT, SIG_IGN);
    dprintf(STDOUT_FILENO, "pid %ld\n", (long)getpid());
    if (write(ready_fd, "r", 1) != 1)
        _exit(92);
    close(ready_fd);
    for (;;)
        pause();
    return 0;
}

int main(int argc, char **argv) {
    if (argc < 2)
        return 90;
    if (!strcmp(argv[1], "ignored-launcher")) {
        signal(SIGCHLD, SIG_IGN);
        signal(SIGTERM, SIG_IGN);
        signal(SIGPIPE, SIG_IGN);
        sigset_t mask;
        sigemptyset(&mask);
        sigaddset(&mask, SIGPIPE);
        sigprocmask(SIG_BLOCK, &mask, NULL);
        execv(argv[2], &argv[2]);
        return 93;
    }
    if (!strcmp(argv[1], "fds")) {
        for (int fd = 3; fd < 256; fd++) {
            if (fcntl(fd, F_GETFD) != -1 || errno != EBADF)
                return 94;
        }
        struct sigaction pipe;
        sigset_t mask;
        if (sigaction(SIGPIPE, NULL, &pipe) || sigprocmask(SIG_BLOCK, NULL, &mask) ||
            pipe.sa_handler != SIG_DFL || sigismember(&mask, SIGPIPE))
            return 95;
        return 0;
    }

    int ready[2];
    if (pipe(ready))
        return 96;
    ready_fd = ready[1];
    dprintf(STDOUT_FILENO, "pid %ld\n", (long)getpid());
    if (!strcmp(argv[1], "clone") || !strcmp(argv[1], "clone-parent")) {
        void *stack = malloc(1024 * 1024);
        int flags = !strcmp(argv[1], "clone-parent") ? CLONE_PARENT : 0;
        if (!stack || clone(descendant, (char *)stack + 1024 * 1024, flags, NULL) < 0)
            return 97;
    }
    else {
        pid_t child = fork();
        if (child < 0)
            return 98;
        if (!child) {
            close(ready[0]);
            if (!strcmp(argv[1], "doublefork")) {
                if (setsid() < 0)
                    _exit(99);
                pid_t grandchild = fork();
                if (grandchild < 0)
                    _exit(100);
                if (grandchild)
                    _exit(0);
            }
            descendant(NULL);
        }
    }
    close(ready[1]);
    char byte;
    if (read(ready[0], &byte, 1) != 1)
        return 101;
    close(ready[0]);
    dprintf(STDOUT_FILENO, "ready\n");
    if (argc > 2 && !strcmp(argv[2], "exit"))
        return 23;
    signal(SIGTERM, SIG_IGN);
    signal(SIGINT, SIG_IGN);
    // A noncooperative leader verifies cancellation does not depend on handlers.
    for (;;)
        ;
}
#endif
