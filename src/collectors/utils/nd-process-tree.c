// SPDX-License-Identifier: GPL-3.0-or-later
#include "config.h"
#include "nd-process-tree.h"

#include <stdio.h>

#ifdef __linux__
#include <errno.h>
#include <dirent.h>
#include <fcntl.h>
#include <limits.h>
#include <poll.h>
#include <signal.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/signalfd.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

// The supervisor is single-threaded and is the only reaper. Direct children
// (including adopted orphans) cannot have their PID reused until it reaps them.
// Enumeration candidates are never used after waitpid(); no group signals are used.
static int kill_children(void) {
    FILE *children = fopen("/proc/thread-self/children", "re");
    if (!children)
        return -1;

    long value;
    int result = 0, scanned;
    while ((scanned = fscanf(children, "%ld", &value)) == 1) {
        if (value <= 0 || value > INT_MAX) {
            result = -1;
            break;
        }
        if (kill((pid_t)value, SIGKILL) != 0 && errno != ESRCH)
            result = -1;
    }
    if (scanned != EOF || ferror(children))
        result = -1;
    fclose(children);
    return result;
}

static bool compatible_procfs(void) {
    // procfs must expose IDs in our PID namespace. A procfs mounted in an outer
    // namespace may otherwise expose a valid PID belonging to a different child.
    FILE *status = fopen("/proc/thread-self/status", "re");
    if (!status)
        return false;
    char *line = NULL;
    size_t size = 0;
    bool compatible = false;
    while (getline(&line, &size, status) >= 0) {
        if (strncmp(line, "NSpid:", 6) == 0) {
            long pid;
            char extra;
            compatible = sscanf(line + 6, "%ld %c", &pid, &extra) == 1 && pid == getpid();
            break;
        }
    }
    free(line);
    fclose(status);
    if (!compatible)
        return false;

    // CONFIG_PROC_CHILDREN is required. Enumeration is never a drain proof.
    FILE *children = fopen("/proc/thread-self/children", "re");
    if (!children)
        return false;
    int c = fgetc(children);
    compatible = c == EOF && !ferror(children);
    fclose(children);
    return compatible;
}

static bool protocol_pipes(int control_fd, int status_fd) {
    struct stat control, status;
    if (control_fd < 3 || status_fd < 3 || control_fd == status_fd ||
        fstat(control_fd, &control) || fstat(status_fd, &status) ||
        !S_ISFIFO(control.st_mode) || !S_ISFIFO(status.st_mode) ||
        (control.st_dev == status.st_dev && control.st_ino == status.st_ino))
        return false;
    int control_flags = fcntl(control_fd, F_GETFL);
    int status_flags = fcntl(status_fd, F_GETFL);
    if (control_flags < 0 || status_flags < 0 ||
        (control_flags & O_ACCMODE) != O_RDONLY || (status_flags & O_ACCMODE) != O_WRONLY)
        return false;

    // Every protocol copy must stay with its owner. An inherited opposite end
    // can hide parent EOF, and an alias can let the payload retain the protocol.
    // Inspect only our descriptors, before any children or concurrent mutation.
    DIR *fds = opendir("/proc/thread-self/fd");
    if (!fds)
        return false;
    bool private = true;
    struct dirent *entry;
    errno = 0;
    while ((entry = readdir(fds))) {
        char *end;
        long fd = strtol(entry->d_name, &end, 10);
        if (*end || fd < 0 || fd > INT_MAX || fd == control_fd || fd == status_fd)
            continue;
        struct stat other;
        if (fstat((int)fd, &other) != 0 ||
            (other.st_dev == control.st_dev && other.st_ino == control.st_ino) ||
            (other.st_dev == status.st_dev && other.st_ino == status.st_ino)) {
            private = false;
            break;
        }
        errno = 0;
    }
    if (errno)
        private = false;
    closedir(fds);
    return private;
}

static bool write_frame(int status_fd, const char *frame, size_t length) {
    ssize_t written;
    do {
        written = write(status_fd, frame, length);
    } while (written < 0 && errno == EINTR);
    return written == (ssize_t)length;
}

static int setup_unavailable(int control_fd, int status_fd) {
    // Only used with validated private pipes and before a successful fork.
    // Capability refusal is distinct from losing a supervisor after launch.
    // Even here, absence of children must come from wait, not procfs discovery.
    struct sigaction ignore_pipe = { .sa_handler = SIG_IGN };
    sigemptyset(&ignore_pipe.sa_mask);
    if (sigaction(SIGPIPE, &ignore_pipe, NULL) == 0) {
        pid_t result;
        do {
            result = waitpid(-1, NULL, WNOHANG | __WALL);
        } while (result < 0 && errno == EINTR);
        if (result < 0 && errno == ECHILD) {
            static const char unavailable[] = "NDTREE1 unavailable\n";
            (void)write_frame(status_fd, unavailable, sizeof(unavailable) - 1);
        }
    }
    close(control_fd);
    close(status_fd);
    return 126;
}

int nd_process_tree_run(int control_fd, int status_fd, char **command) {
    if (!protocol_pipes(control_fd, status_fd)) {
        fprintf(stderr, "nd-run: tree supervision requires two distinct private protocol pipes\n");
        return 126;
    }
    if (!compatible_procfs()) {
        fprintf(stderr, "nd-run: tree supervision requires compatible procfs with children and NSpid support\n");
        return setup_unavailable(control_fd, status_fd);
    }

    // Block before fork: a fast child exit cannot be lost between wait and poll.
    // Explicit SIG_DFL prevents inherited SIG_IGN/SA_NOCLDWAIT auto-reaping.
    const int signals[] = { SIGCHLD, SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGPIPE };
    struct sigaction original[sizeof(signals) / sizeof(signals[0])];
    struct sigaction action = { .sa_handler = SIG_DFL };
    sigemptyset(&action.sa_mask);
    sigset_t watched, original_mask;
    sigemptyset(&watched);
    for (size_t i = 0; i < sizeof(signals) / sizeof(signals[0]); i++) {
        if (sigaction(signals[i], NULL, &original[i]) != 0)
            goto setup_failed;
        sigaddset(&watched, signals[i]);
    }
    if (sigprocmask(SIG_BLOCK, &watched, &original_mask) != 0 ||
        sigaction(SIGCHLD, &action, NULL) != 0)
        goto setup_failed;
    action.sa_handler = SIG_IGN;
    if (sigaction(SIGPIPE, &action, NULL) != 0)
        goto setup_failed;
    // Ignored termination signals do not reach signalfd; normalize those too.
    action.sa_handler = SIG_DFL;
    for (size_t i = 1; i < sizeof(signals) / sizeof(signals[0]) - 1; i++) {
        if (sigaction(signals[i], &action, NULL) != 0)
            goto setup_failed;
    }
    int signal_fd = signalfd(-1, &watched, SFD_CLOEXEC | SFD_NONBLOCK);
    if (signal_fd < 0)
        goto setup_failed;
    if (prctl(PR_SET_CHILD_SUBREAPER, 1) != 0) {
        close(signal_fd);
        goto setup_failed;
    }

    // Descendants must not gain identities the dropped supervisor cannot signal.
    // This flag is inherited across fork/exec and cannot be cleared by payloads.
    if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0) {
        close(signal_fd);
        goto setup_failed;
    }
    int no_new_privs = prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0);
    if (no_new_privs != 1) {
        if (no_new_privs >= 0)
            errno = EPERM;
        close(signal_fd);
        goto setup_failed;
    }

    pid_t child = fork();
    if (child < 0) {
        close(signal_fd);
        goto setup_failed;
    }
    if (child == 0) {
        close(control_fd);
        close(status_fd);
        close(signal_fd);
        for (size_t i = 0; i < sizeof(signals) / sizeof(signals[0]); i++) {
            if (sigaction(signals[i], &original[i], NULL) != 0)
                _exit(126);
        }
        if (sigprocmask(SIG_SETMASK, &original_mask, NULL) != 0)
            _exit(126);
        execvp(command[0], command);
        int error = errno;
        perror("nd-run: tree execvp");
        _exit(error == ENOENT ? 127 : 126);
    }

    bool draining = false, command_done = false, reported_failure = false;
    int command_status = 0;
    for (;;) {
        // Bound each batch so orphan churn cannot starve cancellation handling.
        bool exhausted = false, full_batch = true;
        for (int i = 0; i < 256; i++) {
            int status;
            pid_t reaped = waitpid(-1, &status, WNOHANG | __WALL);
            if (reaped > 0) {
                if (reaped == child && (WIFEXITED(status) || WIFSIGNALED(status))) {
                    command_status = status;
                    child = -1; // Discard the identity before another child can reuse it.
                    command_done = true;
                    draining = true;
                }
                continue;
            }
            if (reaped < 0 && errno == EINTR) {
                i--;
                continue;
            }
            if (reaped < 0 && errno == ECHILD)
                exhausted = true;
            else if (reaped < 0) {
                draining = true;
                if (!reported_failure) {
                    perror("nd-run: tree waitpid; retaining supervision");
                    reported_failure = true;
                }
            }
            full_batch = false;
            break;
        }
        if (exhausted) {
            close(control_fd);
            close(signal_fd);
            if (!command_done) {
                fprintf(stderr, "nd-run: tree command outcome unavailable\n");
                close(status_fd);
                return 126;
            }
            char frame[64];
            int length = snprintf(frame, sizeof(frame), "NDTREE1 %d\n", command_status);
            bool written = write_frame(status_fd, frame, (size_t)length);
            close(status_fd);
            if (!written)
                return 126;
            return WIFEXITED(command_status) ? WEXITSTATUS(command_status) : 128 + WTERMSIG(command_status);
        }

        if (draining && kill_children() != 0 && !reported_failure) {
            fprintf(stderr, "nd-run: tree child termination failed; retaining supervision\n");
            reported_failure = true;
        }

        struct pollfd events[] = {
            { .fd = draining ? -1 : control_fd, .events = POLLIN },
            { .fd = signal_fd, .events = POLLIN },
            { .fd = draining ? -1 : status_fd, .events = 0 },
        };
        // The bounded backstop also handles clone children with unusual exit
        // notification signals. Time or an empty children file NEVER means done.
        int ready = poll(events, sizeof(events) / sizeof(events[0]), full_batch ? 0 : 1000);
        if (ready < 0 && errno != EINTR)
            draining = true;
        if (events[0].revents || events[2].revents)
            draining = true;
        if (events[1].revents & POLLIN) {
            struct signalfd_siginfo info;
            for (int i = 0; i < 64 && read(signal_fd, &info, sizeof(info)) == sizeof(info); i++) {
                if (info.ssi_signo != SIGCHLD)
                    draining = true;
            }
        }
    }

setup_failed:
    perror("nd-run: tree supervision setup");
    return setup_unavailable(control_fd, status_fd);
}

#else
int nd_process_tree_run(int control_fd, int status_fd, char **command) {
    (void)control_fd;
    (void)status_fd;
    (void)command;
    fprintf(stderr, "nd-run: tree supervision is supported only on Linux\n");
    return 126;
}
#endif
